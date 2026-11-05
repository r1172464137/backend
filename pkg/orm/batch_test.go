package orm

import (
	"context"
	"errors"
	"testing"
	"time"
)

// batchRows holds rows to delete and deletes up to the limit per call,
// failing at call failAt when it is set.
type batchRows struct {
	left, calls, failAt int
	before              time.Time
}

var _ BatchDeleter = (*batchRows)(nil)

func (r *batchRows) DeleteBeforeBatch(_ context.Context, before time.Time, limit int) (int64, error) {
	r.calls++
	r.before = before
	if r.calls == r.failAt {
		return 0, errors.New("delete failed")
	}
	n := min(r.left, limit)
	r.left -= n
	return int64(n), nil
}

func TestDeleteBeforeDeletesInBatchesUntilAShortOne(t *testing.T) {
	before := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	rows := &batchRows{left: 25}
	deleted, err := DeleteBefore(context.Background(), rows, before, 10)
	if err != nil || deleted != 25 || rows.calls != 3 || !rows.before.Equal(before) {
		t.Fatalf("deleted %d in %d calls (err %v, before %v); want 25 in 3", deleted, rows.calls, err, rows.before)
	}

	// A backlog of whole batches ends with an empty one.
	rows = &batchRows{left: 20}
	if deleted, err := DeleteBefore(context.Background(), rows, before, 10); err != nil || deleted != 20 || rows.calls != 3 {
		t.Fatalf("deleted %d in %d calls (err %v); want 20 in 3", deleted, rows.calls, err)
	}
}

// A batch size that cannot shrink the backlog must not spin: the caller gets
// an error and the deleter is never called.
func TestDeleteBeforeRejectsANonPositiveBatchSize(t *testing.T) {
	for _, size := range []int{0, -1} {
		rows := &batchRows{left: 10}
		done := make(chan error, 1)
		go func() {
			_, err := DeleteBefore(context.Background(), rows, time.Now(), size)
			done <- err
		}()
		select {
		case err := <-done:
			if err == nil || rows.calls != 0 {
				t.Fatalf("batch size %d: error = %v after %d calls, want an error before any delete", size, err, rows.calls)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("batch size %d: DeleteBefore did not return", size)
		}
	}
}

func TestDeleteBeforeStopsAtAFailureAndCountsTheBatchesBefore(t *testing.T) {
	rows := &batchRows{left: 50, failAt: 2}
	deleted, err := DeleteBefore(context.Background(), rows, time.Now(), 10)
	if err == nil || deleted != 10 || rows.calls != 2 {
		t.Fatalf("deleted %d in %d calls (err %v); want the failure after 10", deleted, rows.calls, err)
	}
}
