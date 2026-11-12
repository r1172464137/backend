package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/perfect-panel/server/pkg/logger"
)

// RetryPolicy bounds how long startup waits for a dependency (MySQL, Redis)
// that is not ready yet. Each attempt has its own deadline covering the real
// driver connection and authentication handshake; the whole wait is bounded by
// Total and stops as soon as the parent context is cancelled (SIGTERM).
type RetryPolicy struct {
	Total          time.Duration
	Attempt        time.Duration
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

// DefaultStartupRetry covers a database or cache container that starts a little
// after the backend (typical compose restart) without hanging forever on a
// dependency that never becomes usable.
var DefaultStartupRetry = RetryPolicy{
	Total:          2 * time.Minute,
	Attempt:        10 * time.Second,
	InitialBackoff: time.Second,
	MaxBackoff:     10 * time.Second,
}

// PermanentError marks a failure that retrying cannot fix (wrong password,
// unknown database, invalid configuration).
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

// connectWithRetry runs attempt until it succeeds, returns a permanent error,
// the budget is spent, or ctx is cancelled. The returned error names the
// dependency, the attempt count and the last cause.
func connectWithRetry[T any](ctx context.Context, name string, p RetryPolicy, attempt func(context.Context) (T, error), permanent func(error) bool) (T, error) {
	var zero T
	start := time.Now()
	deadline := start.Add(p.Total)
	backoff := p.InitialBackoff
	var last error
	for n := 1; ; n++ {
		if err := ctx.Err(); err != nil {
			return zero, fmt.Errorf("%s: startup cancelled after %d attempt(s): %w", name, n-1, err)
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return zero, fmt.Errorf("%s: not ready after %d attempt(s) in %s: %w", name, n-1, p.Total, last)
		}
		attemptTimeout := p.Attempt
		if attemptTimeout > remaining {
			attemptTimeout = remaining
		}
		actx, cancel := context.WithTimeout(ctx, attemptTimeout)
		v, err := attempt(actx)
		cancel()
		if err == nil {
			if n > 1 {
				logger.Infof("[startup] %s ready after %d attempts (%s)", name, n, time.Since(start).Round(time.Millisecond))
			}
			return v, nil
		}
		if ctx.Err() != nil {
			return zero, fmt.Errorf("%s: startup cancelled after %d attempt(s): %w", name, n, ctx.Err())
		}
		if permanent != nil && permanent(err) {
			return zero, fmt.Errorf("%s: %w", name, &PermanentError{Err: err})
		}
		last = err
		wait := backoff
		if r := time.Until(deadline); wait > r {
			wait = max(r, 0)
		}
		logger.Infof("[startup] %s not ready (attempt %d): %v; retrying in %s", name, n, err, wait.Round(time.Millisecond))
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return zero, fmt.Errorf("%s: startup cancelled after %d attempt(s): %w", name, n, ctx.Err())
		case <-t.C:
		}
		backoff *= 2
		if backoff > p.MaxBackoff {
			backoff = p.MaxBackoff
		}
	}
}

// permanentDatabaseError reports errors where the server answered and refused
// the configuration, as opposed to "not reachable / not ready yet".
func permanentDatabaseError(err error) bool {
	var me *mysqldriver.MySQLError
	if errors.As(err, &me) {
		switch me.Number {
		case 1044, // access denied to database
			1045, // access denied for user (wrong password)
			1049, // unknown database
			1251, // client does not support authentication protocol
			1524: // authentication plugin not loaded
			return true
		}
		return false
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "28P01", "28000", "3D000":
			return true
		}
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "database name is empty") ||
		strings.Contains(msg, "unsupported database driver") ||
		strings.Contains(msg, "invalid DSN")
}

// permanentRedisError reports authentication/configuration failures. LOADING
// (dataset still loading) and network errors are transient.
func permanentRedisError(err error) bool {
	msg := err.Error()
	for _, p := range []string{"WRONGPASS", "NOAUTH", "invalid password", "invalid username-password", "ERR AUTH", "ERR DB index is out of range", "ERR invalid DB index"} {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}
