package adminserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/internal/routingstatus"
	"github.com/redis/go-redis/v9"
)

func desiredDeps(t *testing.T) (Deps, *serializedAdminNodeStore) {
	store := newSerializedAdminNodeStore()
	global := config.NodeConfig{RoutingRevision: 5, Routing: config.DefaultNodeRouting("global")}
	mini := miniredis.RunT(t)
	rds := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rds.Close() })
	return Deps{Store: store, Redis: rds, ReadGlobalNodeConfig: func(context.Context) (config.NodeConfig, error) { return global, nil }}, store
}

// The administrator sees the snapshot the node should run and "synced" only
// when a verified report's applied identity equals it in every field.
func TestDesiredSnapshotAndSynced(t *testing.T) {
	deps, _ := desiredDeps(t)
	read := func() dto.RoutingStatusView {
		t.Helper()
		got, err := newGetServerNodeConfigLogic(context.Background(), deps).GetServerNodeConfig(&dto.GetServerNodeConfigRequest{ServerID: 1})
		if err != nil {
			t.Fatal(err)
		}
		return got.RoutingStatus
	}
	status := read()
	if status.DesiredSnapshotID == nil || len(*status.DesiredSnapshotID) != 64 || status.Synced {
		t.Fatalf("no report: %#v", status)
	}
	desired := *status.DesiredSnapshotID
	report := func(seq uint64, id *dto.RoutingCandidateIdentity) {
		if err := routingstatus.Record(context.Background(), deps.Redis, 1, &dto.ServerPushStatusRequest{RoutingVersion: 3, RoutingSequence: seq,
			InstanceID: "11111111-1111-4111-8111-111111111111", RoutingApplied: id}); err != nil {
			t.Fatal(err)
		}
	}
	// Same revision, different users: not synced.
	report(1, &dto.RoutingCandidateIdentity{Mode: "inherited", Source: "global", SourceRevision: 5, SnapshotID: strings.Repeat("b", 64)})
	if s := read(); s.Synced || !s.Known {
		t.Fatalf("revision alone reported synced: %#v", s)
	}
	report(2, &dto.RoutingCandidateIdentity{Mode: "inherited", Source: "global", SourceRevision: 5, SnapshotID: desired})
	if s := read(); !s.Synced {
		t.Fatalf("matching applied identity not synced: %#v", s)
	}
	if again := read(); *again.DesiredSnapshotID != desired {
		t.Fatal("desired snapshot id is not stable across reads")
	}
}

func TestNewAccountReferencesMustExist(t *testing.T) {
	deps, store := desiredDeps(t)
	save := func(revision int64, userIDs string) error {
		policy := strings.Replace(mustJSON(t, config.DefaultNodeRouting("server-1")), `"rules":[]`,
			`"rules":[{"id":"rr_11111111-1111-4111-8111-111111111111","enabled":true,"kind":"domain","values":["example.com"],"outbound_id":"block","user_ids":`+userIDs+`}]`, 1)
		var req dto.UpdateServerNodeConfigRequest
		if err := json.Unmarshal([]byte(`{"server_id":1,"routing_revision":`+itoa(revision)+`,"routing":`+policy+`}`), &req); err != nil {
			t.Fatal(err)
		}
		return newUpdateServerNodeConfigLogic(context.Background(), deps).UpdateServerNodeConfig(&req)
	}
	if err := save(0, `[1000]`); err == nil || store.override != nil {
		t.Fatalf("unknown account accepted: %v", err)
	}
	if err := save(0, `[7]`); err != nil {
		t.Fatalf("existing account rejected: %v", err)
	}
	// Account 7 is deleted later; its reference stays and keeps matching no
	// identity, while other parts of the policy remain editable. Here the
	// stub treats IDs >= 1000 as missing, so simulate the deleted reference
	// as already persisted and verify a re-save keeping it is accepted.
	deleted := strings.Replace(*store.override.Routing, `"user_ids":[7]`, `"user_ids":[1001]`, 1)
	store.override.Routing = &deleted
	if err := save(store.override.RoutingRevision, `[1001,8]`); err != nil {
		t.Fatalf("re-save keeping a deleted account reference rejected: %v", err)
	}
	if err := save(store.override.RoutingRevision, `[1001,1002]`); err == nil {
		t.Fatal("a newly introduced unknown account accepted")
	}
}

// Saving identical canonical content is a no-op: the local revision stays.
func TestIdenticalLocalSaveKeepsRevision(t *testing.T) {
	deps, store := desiredDeps(t)
	policy := mustJSON(t, config.DefaultNodeRouting("server-1"))
	save := func(revision int64) error {
		var req dto.UpdateServerNodeConfigRequest
		if err := json.Unmarshal([]byte(`{"server_id":1,"routing_revision":`+itoa(revision)+`,"routing":`+policy+`}`), &req); err != nil {
			t.Fatal(err)
		}
		return newUpdateServerNodeConfigLogic(context.Background(), deps).UpdateServerNodeConfig(&req)
	}
	if err := save(0); err != nil || store.override.RoutingRevision != 1 {
		t.Fatalf("first save: %v %#v", err, store.override)
	}
	if err := save(1); err != nil || store.override.RoutingRevision != 1 {
		t.Fatalf("identical save changed revision: %v %#v", err, store.override)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func itoa(v int64) string { b, _ := json.Marshal(v); return string(b) }
