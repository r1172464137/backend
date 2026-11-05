package tasklock

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestLockIsExclusiveAndOnlyItsOwnerReleasesIt(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()

	first, ok, err := Acquire(ctx, client, "task:lock", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first acquire = %v, %v", ok, err)
	}
	if _, ok, err := Acquire(ctx, client, "task:lock", time.Minute); err != nil || ok {
		t.Fatalf("second acquire while held = %v, %v; want refused", ok, err)
	}

	// The first run outlives its TTL and a second run takes the lock over.
	server.FastForward(2 * time.Minute)
	second, ok, err := Acquire(ctx, client, "task:lock", time.Minute)
	if err != nil || !ok {
		t.Fatalf("acquire after expiry = %v, %v", ok, err)
	}
	if released, err := first.Release(ctx); err != nil || released {
		t.Fatalf("stale owner released = %v, %v; want the new owner's lock kept", released, err)
	}
	if !server.Exists("task:lock") {
		t.Fatal("stale owner deleted the new owner's lock")
	}
	if released, err := second.Release(ctx); err != nil || !released {
		t.Fatalf("owner release = %v, %v", released, err)
	}
	if server.Exists("task:lock") {
		t.Fatal("owner release left the lock")
	}
}

func TestReleaseSurvivesACancelledRun(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	lock, ok, err := Acquire(ctx, client, "task:lock", time.Minute)
	if err != nil || !ok {
		t.Fatalf("acquire = %v, %v", ok, err)
	}
	cancel()
	if released, err := lock.Release(ctx); err != nil || !released {
		t.Fatalf("release after cancellation = %v, %v", released, err)
	}
}
