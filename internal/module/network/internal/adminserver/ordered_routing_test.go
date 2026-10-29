package adminserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/network/internal/routingstatus"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
)

func TestMissingFieldPresenceRejectsWithoutMutation(t *testing.T) {
	store := newSerializedAdminNodeStore()
	logic := newUpdateServerNodeConfigLogic(context.Background(), Deps{Store: store})
	if err := logic.UpdateServerNodeConfig(&dto.UpdateServerNodeConfigRequest{ServerID: 1}); err == nil || store.override != nil {
		t.Fatalf("nil presence created an override: %#v %v", store.override, err)
	}
	raw := `{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct"}`
	store.override = &node.ServerConfigOverride{Id: 1, ServerId: 1, Routing: &raw, RoutingRevision: 2}
	if err := logic.UpdateServerNodeConfig(&dto.UpdateServerNodeConfigRequest{ServerID: 1, ServerNodeConfigOverride: dto.ServerNodeConfigOverride{InheritRouting: true, RoutingRevision: 2}}); err == nil {
		t.Fatal("nil presence accepted")
	}
	if store.override.Routing == nil || *store.override.Routing != raw || store.override.RoutingRevision != 2 {
		t.Fatalf("nil presence changed override: %#v", store.override)
	}
}

func TestZeroLocalRevisionIsValidWhenExplicit(t *testing.T) {
	store := newSerializedAdminNodeStore()
	var req dto.UpdateServerNodeConfigRequest
	if err := json.Unmarshal([]byte(`{"server_id":1,"routing_revision":0,"routing":{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct"}}`), &req); err != nil {
		t.Fatal(err)
	}
	if err := newUpdateServerNodeConfigLogic(context.Background(), Deps{Store: store}).UpdateServerNodeConfig(&req); err != nil {
		t.Fatal(err)
	}
	if store.override == nil || store.override.RoutingRevision != 1 {
		t.Fatalf("zero local revision not advanced: %#v", store.override)
	}
	if store.cacheClearCalls != 0 {
		t.Fatal("override write invalidated unrelated server-user cache")
	}

}

func TestOfflineDesiredRoutingDoesNotInventNodeStatus(t *testing.T) {
	for _, scenario := range []string{"no_redis", "no_report", "redis_unavailable", "pending_report", "applied_report"} {
		t.Run(scenario, func(t *testing.T) {
			store := newSerializedAdminNodeStore()
			global := config.NodeConfig{RoutingRevision: 5, Routing: &config.NodeRouting{Version: 1, Outbounds: []config.NodeOutbound{}, Rules: []config.NodeRoute{}, DefaultOutboundID: "direct"}}
			deps := Deps{Store: store, ReadGlobalNodeConfig: func(context.Context) (config.NodeConfig, error) { return global, nil }}
			if scenario != "no_redis" {
				mini := miniredis.RunT(t)
				deps.Redis = redis.NewClient(&redis.Options{Addr: mini.Addr(), MaxRetries: -1})
				t.Cleanup(func() { _ = deps.Redis.Close() })
				if scenario == "redis_unavailable" {
					_ = deps.Redis.Close()
				}
				if scenario == "pending_report" || scenario == "applied_report" {
					identity := &dto.RoutingCandidateIdentity{Mode: "ordered-v1-inherited", Source: "global", SourceRevision: 5}
					report := &dto.ServerPushStatusRequest{RoutingVersion: 1, RoutingSequence: 1, InstanceID: "11111111-1111-4111-8111-111111111111"}
					if scenario == "pending_report" {
						report.RoutingPending = identity
					} else {
						report.RoutingApplied = identity
					}
					if err := routingstatus.Record(context.Background(), deps.Redis, 1, report); err != nil {
						t.Fatal(err)
					}
				}
			}
			update := func(body string) error {
				var req dto.UpdateServerNodeConfigRequest
				if err := json.Unmarshal([]byte(body), &req); err != nil {
					t.Fatal(err)
				}
				return newUpdateServerNodeConfigLogic(context.Background(), deps).UpdateServerNodeConfig(&req)
			}
			checkStatus := func(wantLocal bool, revision int64) {
				t.Helper()
				got, err := newGetServerNodeConfigLogic(context.Background(), deps).GetServerNodeConfig(&dto.GetServerNodeConfigRequest{ServerID: 1})
				if err != nil {
					t.Fatal(err)
				}
				if got.Override.InheritRouting == wantLocal || got.Override.RoutingRevision != revision {
					t.Fatalf("desired override %#v", got.Override)
				}
				if wantLocal && (got.Effective.RoutingMode != "local-v1" || got.Effective.Routing.SourceRevision != revision) {
					t.Fatalf("desired local %#v", got.Effective)
				}
				if !wantLocal && (got.Effective.RoutingMode != "global-v1" || got.Effective.Routing.LocalTransitionRevision != revision) {
					t.Fatalf("desired inheritance %#v", got.Effective)
				}
				if scenario == "pending_report" {
					if !got.RoutingStatus.Known || got.RoutingStatus.Report == nil || got.RoutingStatus.Report.Applied != nil || got.RoutingStatus.Report.Pending == nil || got.RoutingStatus.Report.Pending.SourceRevision != 5 {
						t.Fatalf("pending status changed: %#v", got.RoutingStatus)
					}
				} else if scenario == "applied_report" {
					// A previous applied identity is evidence of the previous node
					// state, not an acknowledgement of this new desired revision.
					if !got.RoutingStatus.Known || got.RoutingStatus.Report == nil || got.RoutingStatus.Report.Pending != nil || got.RoutingStatus.Report.Applied == nil || got.RoutingStatus.Report.Applied.SourceRevision != 5 || got.RoutingStatus.Report.Applied.LocalTransitionRevision != 0 {
						t.Fatalf("previous applied identity changed: %#v", got.RoutingStatus)
					}
				} else if got.RoutingStatus.Known || got.RoutingStatus.Report != nil {
					t.Fatalf("invented status: %#v", got.RoutingStatus)
				}
			}
			policy := `{"version":1,"outbounds":[],"rules":[{"id":"rr_11111111-1111-4111-8111-111111111111","enabled":true,"kind":"domain","values":["example.com"],"outbound_id":"block"}],"default_outbound_id":"direct"}`
			if err := update(`{"server_id":1,"routing_revision":0,"routing":` + policy + `}`); err != nil {
				t.Fatalf("offline local save: %v", err)
			}
			checkStatus(true, 1)
			if err := update(`{"server_id":1,"inherit_routing":true,"routing_revision":0}`); err == nil {
				t.Fatal("stale inheritance accepted")
			} else {
				var code *xerr.CodeError
				if !errors.As(errors.Cause(err), &code) || code.GetErrCode() != xerr.NetworkRoutingRevisionConflict {
					t.Fatalf("stale error: %v", err)
				}
			}
			invalid := &dto.UpdateServerNodeConfigRequest{ServerID: 1, RequestFields: map[string]struct{}{"routing": {}, "routing_revision": {}}, ServerNodeConfigOverride: dto.ServerNodeConfigOverride{RoutingRevision: 1, Routing: &config.NodeRouting{Version: 1, Outbounds: []config.NodeOutbound{}, Rules: []config.NodeRoute{}, DefaultOutboundID: "missing"}}}
			if err := newUpdateServerNodeConfigLogic(context.Background(), deps).UpdateServerNodeConfig(invalid); err == nil {
				t.Fatal("invalid reference accepted")
			}
			checkStatus(true, 1)
			if err := update(`{"server_id":1,"inherit_routing":true,"routing_revision":1}`); err != nil {
				t.Fatalf("offline inheritance: %v", err)
			}
			checkStatus(false, 2)
		})
	}
}

func TestServerOrderedInheritanceRevisionAndRemovedLegacy(t *testing.T) {
	store := newSerializedAdminNodeStore()
	global := config.NodeConfig{RoutingRevision: 4, Routing: &config.NodeRouting{Version: 1, Outbounds: []config.NodeOutbound{}, Rules: []config.NodeRoute{}, DefaultOutboundID: "direct"}}
	deps := Deps{Store: store, ReadGlobalNodeConfig: func(context.Context) (config.NodeConfig, error) { return global, nil }}
	policy := `{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct"}`
	legacy := `["geosite:cn"]`
	store.override = &node.ServerConfigOverride{Id: 1, ServerId: 1, Routing: &policy, RoutingRevision: 2, Block: &legacy}
	update := func(body string) error {
		var req dto.UpdateServerNodeConfigRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatal(err)
		}
		return newUpdateServerNodeConfigLogic(context.Background(), deps).UpdateServerNodeConfig(&req)
	}
	if err := update(`{"server_id":1,"inherit_dns":false,"dns":[]}`); err != nil {
		t.Fatal(err)
	}
	if store.override.Routing == nil || store.override.RoutingRevision != 2 || *store.override.Block != legacy {
		t.Fatal("non-routing update changed routing or backup")
	}
	if err := update(`{"server_id":1,"block":[]}`); err == nil {
		t.Fatal("removed legacy field accepted")
	} else {
		var c *xerr.CodeError
		if !errors.As(errors.Cause(err), &c) || c.GetErrCode() != xerr.NetworkLegacyRoutingRemoved {
			t.Fatalf("error %v", err)
		}
	}
	if err := update(`{"server_id":1,"routing":null,"routing_revision":2}`); err == nil {
		t.Fatal("null routing accepted")
	}
	if err := update(`{"server_id":1,"inherit_routing":true}`); err == nil {
		t.Fatal("missing local revision accepted")
	}
	if err := update(`{"server_id":1,"inherit_routing":true,"routing_revision":0}`); err == nil {
		t.Fatal("zero stale local revision accepted")
	}
	var nullRevision dto.UpdateServerNodeConfigRequest
	if err := json.Unmarshal([]byte(`{"server_id":1,"routing_revision":null}`), &nullRevision); err == nil {
		t.Fatal("null local revision accepted")
	}
	if err := update(`{"server_id":1,"inherit_routing":true,"routing_revision":1}`); err == nil {
		t.Fatal("stale revision accepted")
	}
	if err := update(`{"server_id":1,"inherit_routing":true,"routing_revision":2}`); err != nil {
		t.Fatal(err)
	}
	if store.override.Routing != nil || store.override.RoutingRevision != 3 || *store.override.Block != legacy {
		t.Fatalf("inherit %#v", store.override)
	}
	if err := update(`{"server_id":1,"routing":` + policy + `,"routing_revision":3}`); err != nil {
		t.Fatal(err)
	}
	if store.override.Routing == nil || store.override.RoutingRevision != 4 || *store.override.Block != legacy {
		t.Fatalf("local %#v", store.override)
	}
}
