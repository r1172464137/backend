package devicesession

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRevokeOnlyRotatesTargetAndNeverExpires(t *testing.T) {
	rdb := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: rdb.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	if _, err := Epoch(ctx, client, 1); err == nil {
		t.Fatal("missing generation accepted")
	}
	initial, err := AcquireEpoch(ctx, client, 1)
	if err != nil || initial == "" {
		t.Fatalf("initial epoch %q: %v", initial, err)
	}
	if err := Revoke(ctx, client, 1); err != nil {
		t.Fatal(err)
	}
	first, _ := Epoch(ctx, client, 1)
	if first == initial || first == "" || rdb.TTL(key(1)) != 0 {
		t.Fatal("generation missing or expiring")
	}
	if _, err := Epoch(ctx, client, 2); !errors.Is(err, redis.Nil) {
		t.Fatal("unrelated device was revoked")
	}
	if err := Revoke(ctx, client, 1); err != nil {
		t.Fatal(err)
	}
	second, _ := Epoch(ctx, client, 1)
	if first == second {
		t.Fatal("generation did not change")
	}
	rdb.Del(key(1))
	if _, err := Epoch(ctx, client, 1); err == nil {
		t.Fatal("evicted generation accepted")
	}
	afterEviction, err := AcquireEpoch(ctx, client, 1)
	if err != nil || afterEviction == initial || afterEviction == first || afterEviction == second {
		t.Fatal("generation reused after eviction")
	}
	rdb.SetError("unavailable")
	if _, err := Epoch(ctx, client, 1); err == nil {
		t.Fatal("read failed open")
	}
	if err := Revoke(ctx, client, 1); err == nil {
		t.Fatal("revoke failed open")
	}
}

func TestBindingRejectsLegacyAndMalformedDeviceClaims(t *testing.T) {
	for _, claims := range []map[string]any{
		{"LoginType": "device"},
		{IDClaim: "1"},
		{EpochClaim: "0"},
		{IDClaim: float64(1), EpochClaim: "0"},
		{IDClaim: "-1", EpochClaim: "0"},
		{IDClaim: "01", EpochClaim: "0"},
		{IDClaim: "1", EpochClaim: ""},
	} {
		if _, _, err := Binding(claims); err == nil {
			t.Fatalf("accepted malformed claims: %v", claims)
		}
	}
	if id, _, err := Binding(map[string]any{}); err != nil || id != 0 {
		t.Fatal("web session rejected")
	}
	if id, epoch, err := Binding(map[string]any{IDClaim: "9007199254740993", EpochClaim: "0"}); err != nil || id != 9007199254740993 || epoch != "0" {
		t.Fatal("binding lost ID precision")
	}
}
