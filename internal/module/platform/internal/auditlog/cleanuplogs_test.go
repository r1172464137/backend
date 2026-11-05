package auditlog

import (
	"context"
	"errors"
	"testing"
	"time"
)

// cleanupTraffic records the traffic-log prune the cleanup asked for.
type cleanupTraffic struct {
	before time.Time
	calls  int
	err    error
}

var _ TrafficLogPruner = (*cleanupTraffic)(nil)

func (r *cleanupTraffic) PruneTrafficLogs(_ context.Context, before time.Time) (int64, error) {
	r.before = before
	r.calls++
	return 2, r.err
}

// cleanupLogRows records the system-log batch deletes, deleting one row per
// batch.
type cleanupLogRows struct {
	before time.Time
	calls  int
}

func (r *cleanupLogRows) DeleteBeforeBatch(_ context.Context, before time.Time, _ int) (int64, error) {
	r.before = before
	r.calls++
	return 1, nil
}

// Both logs are pruned at the same threshold, the start of the day the
// retention reaches back to; a prune failure is the cleanup's failure.
func TestCleanupLogsPrunesBothLogsAtTheRetentionThreshold(t *testing.T) {
	traffic, logs := &cleanupTraffic{}, &cleanupLogRows{}
	if err := cleanupLogs(context.Background(), true, 7, traffic, logs); err != nil {
		t.Fatal(err)
	}
	if traffic.calls != 1 || logs.calls != 1 || traffic.before.IsZero() || !traffic.before.Equal(logs.before) {
		t.Fatalf("prunes = %d/%d, thresholds = %v/%v", traffic.calls, logs.calls, traffic.before, logs.before)
	}
	if h, m, s := traffic.before.Clock(); h != 0 || m != 0 || s != 0 {
		t.Fatalf("threshold %v is not the start of a day", traffic.before)
	}

	traffic.err = errors.New("delete failed")
	if err := cleanupLogs(context.Background(), true, 7, traffic, logs); !errors.Is(err, traffic.err) {
		t.Fatalf("cleanup error = %v, want %v", err, traffic.err)
	}
}

func TestCleanupLogsRejectsUnsafeRetentionAndSkipsWhenOff(t *testing.T) {
	for _, days := range []int64{0, -1, maxRetentionDays + 1} {
		traffic, logs := &cleanupTraffic{}, &cleanupLogRows{}
		if err := cleanupLogs(context.Background(), true, days, traffic, logs); err == nil {
			t.Fatalf("retention of %d days was accepted", days)
		}
		if traffic.calls != 0 || logs.calls != 0 {
			t.Fatalf("an unsafe retention of %d days deleted: %d/%d", days, traffic.calls, logs.calls)
		}
	}

	traffic, logs := &cleanupTraffic{}, &cleanupLogRows{}
	if err := cleanupLogs(context.Background(), false, 7, traffic, logs); err != nil || traffic.calls != 0 || logs.calls != 0 {
		t.Fatalf("cleanup with automatic clearing off: err %v, prunes %d/%d", err, traffic.calls, logs.calls)
	}
}
