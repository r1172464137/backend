package telegram

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// GetDel hands a pending confirmation to exactly one caller: the value comes
// back once and the key is gone, and a missing key reads as redis.Nil.
func TestRedisStoreGetDelConsumesTheKeyOnce(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := NewTelegramRedisStore(client)
	ctx := context.Background()
	if err := store.Set(ctx, tgActionPrefix+"abc", "{}", time.Minute); err != nil {
		t.Fatal(err)
	}

	if got, err := store.GetDel(ctx, tgActionPrefix+"abc"); err != nil || got != "{}" {
		t.Fatalf("GetDel = (%q, %v), want the stored action", got, err)
	}
	if _, err := store.GetDel(ctx, tgActionPrefix+"abc"); !errors.Is(err, redis.Nil) {
		t.Fatalf("second GetDel error = %v, want redis.Nil", err)
	}
	if _, _, err := store.Take(ctx, tgActionPrefix+"abc"); !errors.Is(err, redis.Nil) {
		t.Fatalf("Take after GetDel error = %v, want the key gone", err)
	}
}
