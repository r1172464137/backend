package routingstatus

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Exercise the actual Redis Lua/cjson/TIME implementation using only a unique
// disposable key on a dedicated loopback Redis. A short test TTL keeps the
// integration test short; production uses the same scripts with TTL above.
func TestNativeRedisReplayFreshness(t *testing.T) {
	addr := os.Getenv("PPANEL_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set PPANEL_TEST_REDIS_ADDR to a dedicated loopback Redis")
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		t.Fatal("non-loopback Redis refused")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	defer client.Close()
	ctx := context.Background()
	key := "ppanel:test:routingstatus:" + uuid.NewString()
	defer client.Del(ctx, key)
	const ttl = 2
	const retention = 60
	instanceA := "11111111-1111-4111-8111-111111111111"
	instanceB := "22222222-2222-4222-8222-222222222222"
	send := func(instance, sequence, fingerprint string) {
		t.Helper()
		payload := fmt.Sprintf(`{"routing_version":1,"instance_id":%q}`, instance)
		if err := storeScript.Run(ctx, client, []string{key}, payload, instance, ttl, maxInstances, sequence, "1", fingerprint, retention, maxRetainedInstances).Err(); err != nil {
			t.Fatal(err)
		}
	}
	known := func() bool {
		t.Helper()
		_, err := readScript.Run(ctx, client, []string{key}, ttl, retention).Text()
		if err == redis.Nil {
			return false
		}
		if err != nil {
			t.Fatal(err)
		}
		return true
	}
	send(instanceA, "3", "content-3")
	if !known() {
		t.Fatal("new content not known")
	}
	time.Sleep(2200 * time.Millisecond)
	send(instanceA, "1", "content-1")
	if known() {
		t.Fatal("old packet revived stale content")
	}
	send(instanceA, "3", "content-3")
	if known() {
		t.Fatal("duplicate packet revived stale content")
	}
	send(instanceA, "4", "content-4")
	if !known() {
		t.Fatal("higher sequence did not recover")
	}
	send(instanceB, "1", "content-b")
	if known() {
		t.Fatal("multiple active instances treated as known")
	}
	time.Sleep(2200 * time.Millisecond)
	send(instanceA, "5", "content-5")
	send(instanceB, "1", "content-b")
	if known() {
		t.Fatal("stale-content live instance was ignored")
	}
}
