package adminserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

func TestV2PartialAndInheritanceDowngrade(t *testing.T) {
	const local = `{"version":2,"outbounds":[],"rules":[],"default_outbound_id":"direct","user_defaults":[{"user_id":1,"outbound_id":"direct"}]}`
	for _, globalVersion := range []int{1, 2} {
		t.Run(string(rune('0'+globalVersion)), func(t *testing.T) {
			store := newSerializedAdminNodeStore()
			raw := local
			store.override = &node.ServerConfigOverride{Id: 1, ServerId: 1, Routing: &raw, RoutingRevision: 3}
			global := &config.NodeRouting{Version: globalVersion, Outbounds: []config.NodeOutbound{}, Rules: []config.NodeRoute{}, DefaultOutboundID: "direct"}
			if globalVersion == 2 {
				global.UserDefaults = []config.NodeUserDefault{}
			}
			deps := Deps{Store: store, ReadGlobalNodeConfig: func(context.Context) (config.NodeConfig, error) {
				return config.NodeConfig{Routing: global, RoutingRevision: 4}, nil
			}}
			call := func(body string) error {
				var req dto.UpdateServerNodeConfigRequest
				if err := json.Unmarshal([]byte(body), &req); err != nil {
					t.Fatal(err)
				}
				return NewService(deps).UpdateServerNodeConfig(context.Background(), &req)
			}
			if err := call(`{"server_id":1,"ip_strategy":"prefer_ipv4","inherit_ip_strategy":false}`); err != nil {
				t.Fatalf("partial update: %v", err)
			}
			if *store.override.Routing != local || store.override.RoutingRevision != 3 {
				t.Fatal("partial overwrote routing")
			}
			err := call(`{"server_id":1,"inherit_routing":true,"routing_revision":3}`)
			if globalVersion == 1 {
				var e *xerr.CodeError
				if !errors.As(errors.Cause(err), &e) || e.GetErrCode() != xerr.NetworkRoutingDowngradeRequired {
					t.Fatalf("downgrade: %v", err)
				}
				if err := call(`{"server_id":1,"inherit_routing":true,"routing_revision":3,"routing_downgrade":true}`); err != nil {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatalf("v2 inheritance: %v", err)
			}
			if store.override.Routing != nil || store.override.RoutingRevision != 4 {
				t.Fatal("inherit did not commit")
			}
		})
	}
}
