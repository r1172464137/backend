package systemsetting

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/xerr"
)

type failingAccounts struct{ calls int }

func (a *failingAccounts) FindExistingUserIDs(context.Context, []int64) ([]int64, error) {
	a.calls++
	return nil, errors.New("identity unavailable")
}

func TestRoutingAccountLookupFallbackAndCASOrdering(t *testing.T) {
	store := testPlatformStore()
	lookup := &failingAccounts{}
	svc := NewService(Deps{Store: NewSettingsTransactor(store), RoutingUsers: lookup})
	var req dto.NodeConfig
	if err := json.Unmarshal([]byte(`{"routing_revision":1,"routing":{"version":2,"outbounds":[],"rules":[],"default_outbound_id":"direct","user_defaults":[{"user_id":42,"outbound_id":"direct"}]}}`), &req); err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateNodeConfig(context.Background(), &req); xerr.CodeOf(err) != xerr.PlatformRoutingRevisionConflict {
		t.Fatalf("stale revision: %v", err)
	}
	if lookup.calls != 0 {
		t.Fatal("stale revision called account lookup")
	}
	req.RoutingRevision = 2
	if err := svc.UpdateNodeConfig(context.Background(), &req); xerr.CodeOf(err) != xerr.DatabaseQueryError {
		t.Fatalf("lookup failure: %v", err)
	}
	if lookup.calls != 1 || store.systems["RoutingRevision"].Value != "2" {
		t.Fatalf("calls %d, revision %s", lookup.calls, store.systems["RoutingRevision"].Value)
	}
	// A plain non-routing save needs no account lookup.
	var partial dto.NodeConfig
	if err := json.Unmarshal([]byte(`{"node_push_interval":42}`), &partial); err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateNodeConfig(context.Background(), &partial); err != nil {
		t.Fatal(err)
	}
	if lookup.calls != 1 {
		t.Fatal("ordinary save called account lookup")
	}
}
