package lifecycle

import (
	"errors"
	"sync"
)

// ErrNotReady is the failure Fail records when it is given none.
var ErrNotReady = errors.New("startup work failed")

// Readiness is a one-shot signal that a piece of startup work is done. The
// service doing the work calls Ready, or Fail with the reason; the services
// that depend on it wait on Done and read the outcome from Err. The group
// starts its services together, so a service that needs another's startup
// work waits on the signal inside its own Start. The first call decides:
// later ones change nothing, so a restart re-running the work cannot revoke
// an outcome the waiters already acted on. A nil Readiness has no waiters.
type Readiness struct {
	once sync.Once
	done chan struct{}
	err  error
}

// NewReadiness returns a signal that has not fired yet.
func NewReadiness() *Readiness {
	return &Readiness{done: make(chan struct{})}
}

// Ready marks the work done.
func (r *Readiness) Ready() {
	r.finish(nil)
}

// Fail marks the work failed; the waiters read err from Err.
func (r *Readiness) Fail(err error) {
	if err == nil {
		err = ErrNotReady
	}
	r.finish(err)
}

func (r *Readiness) finish(err error) {
	if r == nil {
		return
	}
	r.once.Do(func() {
		// Written before the close, so a waiter that saw Done closed reads
		// the outcome.
		r.err = err
		close(r.done)
	})
}

// Done is closed once Ready or Fail was called.
func (r *Readiness) Done() <-chan struct{} {
	return r.done
}

// Err is nil while the work is running and after Ready, and the failure
// after Fail.
func (r *Readiness) Err() error {
	select {
	case <-r.done:
		return r.err
	default:
		return nil
	}
}
