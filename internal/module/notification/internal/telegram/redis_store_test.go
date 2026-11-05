package telegram

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// Take hands a binding token to exactly one caller, with the life it had
// left, so a redemption that fails can put it back without extending it; a
// missing key reads as redis.Nil.
func TestRedisStoreTakeConsumesTheKeyWithItsRemainingLife(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := NewTelegramRedisStore(client)
	ctx := context.Background()
	if err := store.Set(ctx, bindKey("tok"), "7", 5*time.Minute); err != nil {
		t.Fatal(err)
	}
	mini.FastForward(2 * time.Minute)

	value, ttl, err := store.Take(ctx, bindKey("tok"))
	if err != nil || value != "7" {
		t.Fatalf("Take = (%q, %v, %v), want the token", value, ttl, err)
	}
	if ttl <= 2*time.Minute || ttl > 3*time.Minute {
		t.Fatalf("ttl = %v, want the three minutes the token had left", ttl)
	}
	if _, _, err := store.Take(ctx, bindKey("tok")); !errors.Is(err, redis.Nil) {
		t.Fatalf("second Take error = %v, want redis.Nil", err)
	}
	// A key without an expiry has no life to restore.
	if err := client.Set(ctx, bindKey("forever"), "7", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if _, ttl, err := store.Take(ctx, bindKey("forever")); err != nil || ttl != 0 {
		t.Fatalf("Take of a key without expiry = (ttl %v, %v), want no life", ttl, err)
	}
}

// Acquire takes a lock once until it is released or expires.
func TestRedisStoreAcquireIsExclusive(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := NewTelegramRedisStore(client)
	ctx := context.Background()

	if ok, err := store.Acquire(ctx, bindLockKey(7), time.Minute); err != nil || !ok {
		t.Fatalf("first Acquire = (%v, %v), want the lock", ok, err)
	}
	if ok, err := store.Acquire(ctx, bindLockKey(7), time.Minute); err != nil || ok {
		t.Fatalf("second Acquire = (%v, %v), want the lock held", ok, err)
	}
	if err := store.Delete(ctx, bindLockKey(7)); err != nil {
		t.Fatal(err)
	}
	if ok, err := store.Acquire(ctx, bindLockKey(7), time.Minute); err != nil || !ok {
		t.Fatalf("Acquire after release = (%v, %v), want the lock", ok, err)
	}
	mini.FastForward(2 * time.Minute)
	if ok, err := store.Acquire(ctx, bindLockKey(7), time.Minute); err != nil || !ok {
		t.Fatalf("Acquire after expiry = (%v, %v), want the lock", ok, err)
	}
}
