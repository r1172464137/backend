package repo

import (
	"context"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/redis/go-redis/v9"
)

func TestStartupReconcilesLostInvalidationAndOrphanCaches(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer client.Close()
	ctx := context.Background()
	repo := NewNodeRepo(nil, client)
	key := fmt.Sprintf("%s%d:vless", node.ServerUserListCacheKey, 7)
	if err := repo.SetServerCache(ctx, 7, key, "sensitive-user-list", 0); err != nil {
		t.Fatal(err)
	}
	// A crash after a durable DB change but before the in-memory retry runs
	// leaves indexed keys; deleted-server and pre-index legacy keys must also
	// be recovered even though no server row exists anymore.
	legacy := fmt.Sprintf("%s%d:legacy", node.ServerUserListCacheKey, 7)
	if err := client.Set(ctx, legacy, "unindexed-legacy", node.ServerCacheTTL).Err(); err != nil {
		t.Fatal(err)
	}
	orphan := fmt.Sprintf("%s%d:trojan", node.ServerUserListCacheKey, 99)
	if err := client.Set(ctx, orphan, "orphan-users", node.ServerCacheTTL).Err(); err != nil {
		t.Fatal(err)
	}
	if err := ReconcileServerCaches(ctx, client); err != nil {
		t.Fatal(err)
	}
	if client.Exists(ctx, key, legacy, orphan).Val() != 0 {
		t.Fatal("startup kept stale response cache")
	}
	for _, id := range []int64{7, 99} {
		generation, err := repo.ServerCacheGeneration(ctx, id)
		if err != nil || generation == 0 {
			t.Fatalf("missing generation fence for %d: %d %v", id, generation, err)
		}
	}
	if err := repo.SetServerCache(ctx, 7, key, "raced-stale-fill", 0); err != nil {
		t.Fatal(err)
	}
	if client.Exists(ctx, key).Val() != 0 {
		t.Fatal("pre-recovery reader repopulated stale data")
	}
}
func TestStartupCacheReconciliationFailsClosed(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	_ = client.Close()
	if err := ReconcileServerCaches(context.Background(), client); err == nil {
		t.Fatal("unavailable Redis was accepted")
	}
}
