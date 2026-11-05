package tasklock

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newLockClient(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return server, client
}

// Only the owner extends its lock: a run that lost the lock to another run
// leaves the new owner's TTL alone.
func TestExtendRefreshesOnlyTheOwnersLock(t *testing.T) {
	server, client := newLockClient(t)
	ctx := context.Background()

	first, ok, err := Acquire(ctx, client, "task:lock", time.Minute)
	if err != nil || !ok {
		t.Fatalf("acquire = %v, %v", ok, err)
	}
	server.FastForward(40 * time.Second)
	if ttl := server.TTL("task:lock"); ttl != 20*time.Second {
		t.Fatalf("TTL before the extension = %v, want 20s", ttl)
	}
	if extended, err := first.Extend(ctx, time.Minute); err != nil || !extended {
		t.Fatalf("owner extend = %v, %v", extended, err)
	}
	if ttl := server.TTL("task:lock"); ttl != time.Minute {
		t.Fatalf("TTL after the extension = %v, want 1m", ttl)
	}

	// The run outlives even the extension, and another run takes over.
	server.FastForward(2 * time.Minute)
	second, ok, err := Acquire(ctx, client, "task:lock", 30*time.Second)
	if err != nil || !ok {
		t.Fatalf("acquire after expiry = %v, %v", ok, err)
	}
	if extended, err := first.Extend(ctx, time.Minute); err != nil || extended {
		t.Fatalf("stale owner extend = %v, %v; want refused", extended, err)
	}
	if ttl := server.TTL("task:lock"); ttl != 30*time.Second {
		t.Fatalf("the stale owner changed the new owner's TTL: %v", ttl)
	}
	if released, err := second.Release(ctx); err != nil || !released {
		t.Fatalf("new owner release = %v, %v", released, err)
	}
}

// The heartbeat refreshes the TTL while the run lasts and stops with it.
func TestKeepAliveExtendsTheLockWhileTheRunLasts(t *testing.T) {
	server, client := newLockClient(t)
	ctx := context.Background()
	const ttl = 300 * time.Millisecond

	lock, ok, err := Acquire(ctx, client, "task:lock", ttl)
	if err != nil || !ok {
		t.Fatalf("acquire = %v, %v", ok, err)
	}
	var reported []error
	stop := lock.KeepAlive(ctx, ttl, func(err error) { reported = append(reported, err) })

	// The TTL runs down (miniredis time only moves when told), and the next
	// heartbeat, a third of the TTL later, refreshes it.
	server.FastForward(ttl - 10*time.Millisecond)
	deadline := time.Now().Add(2 * time.Second)
	for server.TTL("task:lock") < ttl/2 {
		if time.Now().After(deadline) {
			t.Fatalf("the heartbeat did not refresh the TTL: %v", server.TTL("task:lock"))
		}
		time.Sleep(10 * time.Millisecond)
	}

	stop()
	stop() // idempotent
	server.FastForward(ttl - 10*time.Millisecond)
	time.Sleep(3 * ttl / 2)
	if remaining := server.TTL("task:lock"); remaining != 10*time.Millisecond {
		t.Fatalf("the heartbeat kept extending after stop: TTL %v", remaining)
	}
	if len(reported) != 0 {
		t.Fatalf("heartbeat reported %v while the lock was held", reported)
	}
	if released, err := lock.Release(ctx); err != nil || !released {
		t.Fatalf("release = %v, %v", released, err)
	}
}

// A heartbeat that finds the lock gone reports the loss once and stops.
func TestKeepAliveReportsALostLock(t *testing.T) {
	server, client := newLockClient(t)
	ctx := context.Background()
	const ttl = 150 * time.Millisecond

	lock, ok, err := Acquire(ctx, client, "task:lock", ttl)
	if err != nil || !ok {
		t.Fatalf("acquire = %v, %v", ok, err)
	}
	if err := server.Set("task:lock", "another-run"); err != nil {
		t.Fatal(err)
	}
	reported := make(chan error, 8)
	stop := lock.KeepAlive(ctx, ttl, func(err error) { reported <- err })
	defer stop()

	select {
	case err := <-reported:
		if !errors.Is(err, ErrLost) {
			t.Fatalf("reported %v, want %v", err, ErrLost)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the lost lock was never reported")
	}
	time.Sleep(3 * ttl)
	if len(reported) != 0 {
		t.Fatalf("the heartbeat kept reporting after the loss: %d more", len(reported))
	}
	if got, _ := server.Get("task:lock"); got != "another-run" {
		t.Fatalf("the heartbeat touched the other run's lock: %q", got)
	}
}
