package usersession

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRevokeInvalidatesEarlierSessionsOnly(t *testing.T) {
	rdb := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: rdb.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()

	legacy := map[string]any{"UserId": float64(1)}
	if err := Check(legacy, ""); err != nil {
		t.Fatalf("token from before epochs rejected for a never-revoked user: %v", err)
	}

	first, err := AcquireEpoch(ctx, client, 1)
	if err != nil || first == "" {
		t.Fatalf("AcquireEpoch() = %q, %v", first, err)
	}
	if again, _ := AcquireEpoch(ctx, client, 1); again != first {
		t.Fatalf("second session got epoch %q, want the shared %q", again, first)
	}
	issued := map[string]any{EpochClaim: first}
	if err := Check(issued, first); err != nil {
		t.Fatalf("current session rejected: %v", err)
	}
	if err := Check(legacy, first); err != nil {
		t.Fatalf("legacy token rejected before any revocation: %v", err)
	}

	if err := Revoke(ctx, client, 1); err != nil {
		t.Fatal(err)
	}
	current, _ := client.Get(ctx, Key(1)).Result()
	if rdb.TTL(Key(1)) != 0 {
		t.Fatal("epoch must not expire")
	}
	if Check(issued, current) == nil || Check(legacy, current) == nil {
		t.Fatal("a session issued before the revocation is still valid")
	}
	next, _ := AcquireEpoch(ctx, client, 1)
	if next != current || Check(map[string]any{EpochClaim: next}, current) != nil {
		t.Fatal("a session issued after the revocation is not valid")
	}
	if other, _ := AcquireEpoch(ctx, client, 2); other == current {
		t.Fatal("users share an epoch")
	}

	// An evicted epoch fails tokens that carry one closed.
	rdb.Del(Key(1))
	if Check(map[string]any{EpochClaim: next}, "") == nil {
		t.Fatal("token accepted after its epoch was evicted")
	}
}

// A revocation is dated, so a capability issued before it can be told apart
// from one issued after; an unreadable revocation and a missing store both
// fail closed.
func TestRevokedSinceDatesTheRevocation(t *testing.T) {
	_, client := newTestClient(t)
	ctx := context.Background()
	longAgo := time.Now().Add(-time.Hour)

	if revoked, err := RevokedSince(ctx, client, 7, longAgo); err != nil || revoked {
		t.Fatalf("user without an epoch: RevokedSince() = %t, %v", revoked, err)
	}
	if _, err := AcquireEpoch(ctx, client, 7); err != nil {
		t.Fatal(err)
	}
	if revoked, err := RevokedSince(ctx, client, 7, longAgo); err != nil || revoked {
		t.Fatalf("issued epoch: RevokedSince() = %t, %v, want no revocation", revoked, err)
	}

	justBefore := time.Now()
	if err := Revoke(ctx, client, 7); err != nil {
		t.Fatal(err)
	}
	if revoked, err := RevokedSince(ctx, client, 7, longAgo); err != nil || !revoked {
		t.Fatalf("revocation after since: RevokedSince() = %t, %v, want revoked", revoked, err)
	}
	// The revocation is dated to the millisecond; one in the same
	// millisecond as since counts.
	if revoked, err := RevokedSince(ctx, client, 7, justBefore); err != nil || !revoked {
		t.Fatalf("revocation right after since: RevokedSince() = %t, %v, want revoked", revoked, err)
	}
	if revoked, err := RevokedSince(ctx, client, 7, time.Now().Add(time.Minute)); err != nil || revoked {
		t.Fatalf("revocation before since: RevokedSince() = %t, %v, want not revoked", revoked, err)
	}
	epoch, _ := client.Get(ctx, Key(7)).Result()
	if at, ok := RevocationTime(epoch); !ok || time.Since(at) > time.Second || at.After(time.Now().Add(time.Millisecond)) {
		t.Fatalf("RevocationTime(%q) = %v, %t; want about now", epoch, at, ok)
	}
	if _, ok := RevocationTime("i:" + epoch[2:]); ok {
		t.Fatal("an issued epoch reads as a revocation")
	}

	if err := client.Set(ctx, Key(7), "r:not-a-uuid", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if revoked, err := RevokedSince(ctx, client, 7, time.Now().Add(time.Hour)); err != nil || !revoked {
		t.Fatalf("unreadable revocation: RevokedSince() = %t, %v, want revoked", revoked, err)
	}
	var none *redis.Client
	if _, err := RevokedSince(ctx, none, 7, longAgo); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("RevokedSince() without a store: error = %v, want ErrUnavailable", err)
	}
}
