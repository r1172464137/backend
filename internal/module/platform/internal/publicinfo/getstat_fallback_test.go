package publicinfo

import (
	"context"
	"errors"
	"testing"
	"time"
)

// While Redis is unreachable the process serves its own copy of the
// statistics for as long as the cache would have, instead of rebuilding
// them (four counts and a resolution of every node hostname) on every
// anonymous call.
func TestGetStatServesTheProcessCopyWhileRedisIsDown(t *testing.T) {
	w := newStatWorld(t)
	w.users.enabled = 300
	w.nodes.enabled = 5
	if _, err := w.svc.GetStat(context.Background()); err != nil {
		t.Fatal(err)
	}
	w.redis.Close()

	for range 3 {
		got, err := w.svc.GetStat(context.Background())
		if err != nil || got.User != 300 || got.Node != 5 {
			t.Fatalf("stat with Redis down = %+v (err %v), want the process's copy", got, err)
		}
	}
	if calls := w.users.calls.Load(); calls != 1 {
		t.Fatalf("the store was queried %d times, want once: Redis being down must not rebuild the statistics per call", calls)
	}
	// The copy ages like the cache would.
	w.svc.statMemo.mu.Lock()
	w.svc.statMemo.at = time.Now().Add(-2 * statCacheTTL)
	w.svc.statMemo.mu.Unlock()
	w.users.enabled = 400
	if got, err := w.svc.GetStat(context.Background()); err != nil || got.User != 400 || w.users.calls.Load() != 2 {
		t.Fatalf("stat after the copy aged = %+v (err %v, %d queries), want a rebuild", got, err, w.users.calls.Load())
	}
}

// A failed refresh is answered from memory for a while: a store outage is
// not turned into a query per anonymous call.
func TestGetStatDoesNotRetryAFailedRefreshAtOnce(t *testing.T) {
	w := newStatWorld(t)
	w.users.err = errStatBackend
	for range 3 {
		if _, err := w.svc.GetStat(context.Background()); !errors.Is(err, errStatBackend) {
			t.Fatalf("err = %v, want the store failure", err)
		}
	}
	if calls := w.users.calls.Load(); calls != 1 {
		t.Fatalf("the failing store was queried %d times, want once within the backoff", calls)
	}
	// After the backoff the store is asked again, and a recovered store
	// answers.
	w.svc.statMemo.mu.Lock()
	w.svc.statMemo.failedAt = time.Now().Add(-2 * statFailureBackoff)
	w.svc.statMemo.mu.Unlock()
	w.users.err = nil
	w.users.enabled = 50
	if got, err := w.svc.GetStat(context.Background()); err != nil || got.User != 50 || w.users.calls.Load() != 2 {
		t.Fatalf("stat after the backoff = %+v (err %v, %d queries), want a fresh refresh", got, err, w.users.calls.Load())
	}
}
