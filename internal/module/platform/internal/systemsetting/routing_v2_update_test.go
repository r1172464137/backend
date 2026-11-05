package systemsetting

import (
	"context"
	"encoding/json"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type routingTestUsers struct{}

func (routingTestUsers) FindExistingUserIDs(_ context.Context, ids []int64) ([]int64, error) {
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id == 1 {
			out = append(out, id)
		}
	}
	return out, nil
}

func TestGlobalV2PartialCASAndDowngrade(t *testing.T) {
	store := testPlatformStore()
	deps := Deps{Store: NewSettingsTransactor(store), RoutingUsers: routingTestUsers{}}
	call := func(body string) error {
		var req dto.NodeConfig
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatal(err)
		}
		return NewService(deps).UpdateNodeConfig(context.Background(), &req)
	}
	if err := call(`{"routing_revision":2,"routing":{"version":2,"outbounds":[],"rules":[],"default_outbound_id":"direct","user_defaults":[{"user_id":1,"outbound_id":"direct"}]}}`); err != nil {
		t.Fatalf("partial v2: %v", err)
	}
	raw := store.systems["Routing"].Value
	if err := call(`{"routing_revision":3,"routing":{"version":2,"outbounds":[],"rules":[],"default_outbound_id":"direct","user_defaults":[{"user_id":2,"outbound_id":"direct"}]}}`); err == nil {
		t.Fatal("unknown account accepted")
	}
	if store.systems["RoutingRevision"].Value != "3" {
		t.Fatal("revision not advanced")
	}
	if err := call(`{"node_secret":"changed"}`); err != nil {
		t.Fatalf("non-routing partial: %v", err)
	}
	if store.systems["Routing"].Value != raw || store.systems["RoutingRevision"].Value != "3" {
		t.Fatal("partial update overwrote routing")
	}
	stale := call(`{"routing_revision":2,"routing":{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct"}}`)
	var e *xerr.CodeError
	if !errors.As(errors.Cause(stale), &e) || e.GetErrCode() != xerr.PlatformRoutingRevisionConflict {
		t.Fatalf("stale %v", stale)
	}
	err := call(`{"routing_revision":3,"routing":{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct"}}`)
	if !errors.As(errors.Cause(err), &e) || e.GetErrCode() != xerr.PlatformRoutingDowngradeRequired {
		t.Fatalf("downgrade %v", err)
	}
	if err := call(`{"routing_revision":3,"routing_downgrade":true,"routing":{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct"}}`); err != nil {
		t.Fatal(err)
	}
}
