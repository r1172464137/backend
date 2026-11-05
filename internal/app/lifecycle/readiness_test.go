package lifecycle

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// The group starts its services together, so the worker that reads what the
// bootstrap loads has to wait for the bootstrap's signal before consuming.
func TestGroupWorkerWaitsForTheBootstrap(t *testing.T) {
	loaded := NewReadiness()
	var settingsLoaded, consumedBeforeLoad atomic.Bool
	consumed := make(chan struct{})

	group := NewServiceGroup()
	// The HTTP service: its bootstrap takes a while, then the settings are
	// published.
	group.Add(WithStart(func() {
		time.Sleep(50 * time.Millisecond)
		settingsLoaded.Store(true)
		loaded.Ready()
	}))
	// The task worker: consumes once the settings it reads are loaded.
	group.Add(WithStart(func() {
		<-loaded.Done()
		if !settingsLoaded.Load() {
			consumedBeforeLoad.Store(true)
		}
		close(consumed)
	}))

	go group.Start()
	select {
	case <-consumed:
	case <-time.After(5 * time.Second):
		t.Fatal("the worker never consumed")
	}
	group.Stop()

	if consumedBeforeLoad.Load() {
		t.Fatal("the worker consumed before the bootstrap loaded the settings")
	}
	if err := loaded.Err(); err != nil {
		t.Fatalf("Err() = %v after Ready", err)
	}
}

// The first outcome stands: a failure the waiters acted on is not revoked by
// a later success, nor a success by a later failure. A nil signal has no
// waiters to inform.
func TestReadinessKeepsTheFirstOutcome(t *testing.T) {
	failure := errors.New("bootstrap failed")

	r := NewReadiness()
	select {
	case <-r.Done():
		t.Fatal("Done closed before the work finished")
	default:
	}
	if err := r.Err(); err != nil {
		t.Fatalf("Err() = %v while the work runs, want nil", err)
	}
	r.Fail(failure)
	r.Ready()
	<-r.Done()
	if err := r.Err(); !errors.Is(err, failure) {
		t.Fatalf("Err() = %v after Fail then Ready, want the failure", err)
	}

	r = NewReadiness()
	r.Ready()
	r.Fail(failure)
	if err := r.Err(); err != nil {
		t.Fatalf("Err() = %v after Ready then Fail, want nil", err)
	}

	r = NewReadiness()
	r.Fail(nil)
	if err := r.Err(); !errors.Is(err, ErrNotReady) {
		t.Fatalf("Err() = %v after Fail(nil), want ErrNotReady", err)
	}

	var none *Readiness
	none.Ready()
	none.Fail(failure)
}
