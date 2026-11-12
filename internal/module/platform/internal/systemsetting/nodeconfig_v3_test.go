package systemsetting

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	appconfig "github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

func v3Deps() (Deps, *serializedPlatformConfigStore) {
	store := testPlatformStore()
	return Deps{Store: store, System: &serializedSystemRepo{owner: store}, MissingAccounts: func(_ context.Context, ids []int64) ([]int64, error) {
		var missing []int64
		for _, id := range ids {
			if id >= 1000 {
				missing = append(missing, id)
			}
		}
		return missing, nil
	}}, store
}

func v3Update(t *testing.T, deps Deps, body string) error {
	t.Helper()
	var req dto.NodeConfig
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	return newUpdateNodeConfigLogic(context.Background(), deps).UpdateNodeConfig(&req)
}

// DNS and the IP preference are part of the routing package; the former
// separate global fields are rejected like the other removed legacy fields.
func TestGlobalLegacyDNSAndIPStrategyRemoved(t *testing.T) {
	deps, store := v3Deps()
	before := store.systems["RoutingRevision"].Value
	for _, body := range []string{`{"dns":[]}`, `{"ip_strategy":"prefer_ipv4"}`, `{"node_push_interval":30,"dns":[]}`} {
		err := v3Update(t, deps, body)
		var c *xerr.CodeError
		if err == nil || !errors.As(errors.Cause(err), &c) || c.GetErrCode() != xerr.PlatformLegacyRoutingRemoved {
			t.Fatalf("%s: %v, want legacy-removed", body, err)
		}
	}
	if store.systems["RoutingRevision"].Value != before {
		t.Fatal("rejected legacy request changed the revision")
	}
}

func TestGlobalAccountReferencesAndNoop(t *testing.T) {
	deps, store := v3Deps()
	policy := func(userIDs string) string {
		b, _ := json.Marshal(appconfig.DefaultNodeRouting("global"))
		return strings.Replace(string(b), `"rules":[]`, `"rules":[{"id":"rr_11111111-1111-4111-8111-111111111111","enabled":true,"kind":"domain","values":["example.com"],"outbound_id":"block","user_ids":`+userIDs+`}]`, 1)
	}
	rev := func() string { return store.systems["RoutingRevision"].Value }
	start := rev()
	if err := v3Update(t, deps, `{"routing_revision":`+start+`,"routing":`+policy(`[1000]`)+`}`); err == nil || rev() != start {
		t.Fatalf("unknown account accepted: %v", err)
	}
	if err := v3Update(t, deps, `{"routing_revision":`+start+`,"routing":`+policy(`[5]`)+`}`); err != nil {
		t.Fatalf("existing account rejected: %v", err)
	}
	saved := rev()
	if saved == start {
		t.Fatal("content change did not advance the revision")
	}
	if err := v3Update(t, deps, `{"routing_revision":`+saved+`,"routing":`+policy(`[5]`)+`}`); err != nil || rev() != saved {
		t.Fatalf("identical save changed revision: %v %s->%s", err, saved, rev())
	}
}
