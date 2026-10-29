package systemsetting

import (
	"context"
	"encoding/json"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

func TestAdminNodeIntervalsBounds(t *testing.T) {
	store := testPlatformStore()
	deps := Deps{Store: store, System: &serializedSystemRepo{owner: store}}
	for _, field := range []string{"node_pull_interval", "node_push_interval"} {
		for _, value := range []string{"0", "1", "86400"} {
			var req dto.NodeConfig
			if err := json.Unmarshal([]byte(`{"`+field+`":`+value+`}`), &req); err != nil {
				t.Fatal(err)
			}
			if err := newUpdateNodeConfigLogic(context.Background(), deps).UpdateNodeConfig(&req); err != nil {
				t.Fatalf("%s=%s: %v", field, value, err)
			}
		}
		for _, value := range []string{"-1", "86401", "9223372036854775807"} {
			var req dto.NodeConfig
			if err := json.Unmarshal([]byte(`{"`+field+`":`+value+`}`), &req); err != nil {
				t.Fatal(err)
			}
			if err := newUpdateNodeConfigLogic(context.Background(), deps).UpdateNodeConfig(&req); err == nil {
				t.Fatalf("accepted %s=%s", field, value)
			}
		}
		var req dto.NodeConfig
		if json.Unmarshal([]byte(`{"`+field+`":null}`), &req) == nil {
			t.Fatalf("accepted null %s", field)
		}
	}
}

func TestOrderedNodeConfigPresenceAndRemovedLegacy(t *testing.T) {
	store := testPlatformStore()
	deps := Deps{Store: store, System: &serializedSystemRepo{owner: store}}
	update := func(body string) error {
		var req dto.NodeConfig
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatal(err)
		}
		return newUpdateNodeConfigLogic(context.Background(), deps).UpdateNodeConfig(&req)
	}
	if err := update(`{"node_push_interval":33}`); err != nil {
		t.Fatal(err)
	}
	if store.systems["RoutingRevision"].Value != "2" || store.systems["Block"].Value != `["geosite:cn"]` {
		t.Fatal("non-routing update changed routing or backup")
	}
	if err := update(`{"block":[]}`); err == nil {
		t.Fatal("removed legacy field accepted")
	} else {
		var c *xerr.CodeError
		if !errors.As(errors.Cause(err), &c) || c.GetErrCode() != xerr.PlatformLegacyRoutingRemoved {
			t.Fatalf("error %v", err)
		}
	}
	if err := update(`{"routing":null,"routing_revision":2}`); err == nil {
		t.Fatal("null routing accepted")
	}
	if err := update(`{"routing":{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"block"}}`); err == nil {
		t.Fatal("missing revision accepted")
	}
	if err := update(`{"routing_revision":0,"routing":{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"block"}}`); err == nil {
		t.Fatal("zero stale global revision accepted")
	}
	var nullRevision dto.NodeConfig
	if err := json.Unmarshal([]byte(`{"routing_revision":null}`), &nullRevision); err == nil {
		t.Fatal("null revision accepted")
	}
	if err := update(`{"routing_revision":1,"routing":{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"block"}}`); err == nil {
		t.Fatal("stale revision accepted")
	}
	if err := update(`{"routing_revision":2,"routing":{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"block"}}`); err != nil {
		t.Fatal(err)
	}
	if store.systems["RoutingRevision"].Value != "3" {
		t.Fatal("ordered save did not advance revision")
	}
	var invalid dto.NodeConfig
	if err := json.Unmarshal([]byte(`{"routing":{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct","unknown":true}}`), &invalid); err == nil {
		t.Fatal("unknown routing field accepted")
	}
	got, err := newGetNodeConfigLogic(context.Background(), deps).GetNodeConfig()
	if err != nil || got.Routing == nil || got.RoutingRevision != 3 || got.RoutingMode != "global-v1" {
		t.Fatalf("get %#v %v", got, err)
	}
	store.systems["Routing"].Value = ""
	if _, err = newGetNodeConfigLogic(context.Background(), deps).GetNodeConfig(); err == nil {
		t.Fatal("missing global routing accepted")
	}
}
