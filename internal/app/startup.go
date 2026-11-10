package app

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/orm"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const (
	// defaultStartupRetryBudget bounds how long the process waits for the
	// database and Redis to come back before it gives up. A container
	// restart takes a few seconds; 30 s covers it without turning a
	// configuration error into a minute-long hang.
	defaultStartupRetryBudget = 30 * time.Second
	// defaultStartupAttemptTimeout bounds one connection attempt, so an
	// address that drops packets cannot hold the process past its budget.
	defaultStartupAttemptTimeout = 5 * time.Second
	// defaultStartupRetryInitialDelay and defaultStartupRetryMaxDelay shape
	// the exponential backoff: 0.5 s, 1 s, 2 s, 4 s and then 5 s between
	// attempts.
	defaultStartupRetryInitialDelay = 500 * time.Millisecond
	defaultStartupRetryMaxDelay     = 5 * time.Second
)

// startupRetry bounds the connection retries of the process start. The
// budget covers the attempts and the waits between them; a context or signal
// ends the retries earlier.
type startupRetry struct {
	budget         time.Duration
	attemptTimeout time.Duration
	initialDelay   time.Duration
	maxDelay       time.Duration
}

// defaultStartupRetry is the policy of a process start. Its 30 s budget
// outlasts the few seconds a restarted database or Redis container needs,
// and then fails the start rather than running without its infrastructure.
var defaultStartupRetry = startupRetry{
	budget:         defaultStartupRetryBudget,
	attemptTimeout: defaultStartupAttemptTimeout,
	initialDelay:   defaultStartupRetryInitialDelay,
	maxDelay:       defaultStartupRetryMaxDelay,
}

// normalized fills in the defaults of a policy a test built with shorter
// values, and keeps its backoff from exceeding the budget it is given.
func (r startupRetry) normalized() startupRetry {
	if r.budget <= 0 {
		r.budget = defaultStartupRetryBudget
	}
	if r.attemptTimeout <= 0 {
		r.attemptTimeout = defaultStartupAttemptTimeout
	}
	if r.initialDelay <= 0 {
		r.initialDelay = defaultStartupRetryInitialDelay
	}
	if r.maxDelay < r.initialDelay {
		r.maxDelay = r.initialDelay
	}
	return r
}

// connectAtStartup runs connect until it succeeds, the budget is spent or
// ctx is done, backing off exponentially between attempts. Every attempt gets
// its own timeout within the budget. A connection that is still down when the
// budget ends is an error: the process cannot run without it, so it must not
// silently start degraded. The attempts and the final reason are logged.
//
// name and target identify the dependency in the logs and the error; target
// is an address, never credentials.
func connectAtStartup[T any](
	ctx context.Context,
	name, target string,
	retry startupRetry,
	connect func(context.Context) (T, error),
) (T, error) {
	var zero T
	retry = retry.normalized()
	deadline := time.Now().Add(retry.budget)
	delay := retry.initialDelay
	attempts := 0
	var lastErr error

	for {
		if err := ctx.Err(); err != nil {
			return zero, startupCanceled(name, target, attempts, err)
		}
		if lastErr != nil && time.Until(deadline) <= 0 {
			return zero, startupExhausted(name, target, attempts, retry.budget, lastErr)
		}
		attempts++

		attemptTimeout := retry.attemptTimeout
		if remaining := time.Until(deadline); remaining < attemptTimeout {
			attemptTimeout = remaining
		}
		attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
		value, err := connect(attemptCtx)
		cancel()
		if err == nil {
			return value, nil
		}
		lastErr = err
		if ctxErr := ctx.Err(); ctxErr != nil {
			return zero, startupCanceled(name, target, attempts, ctxErr)
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return zero, startupExhausted(name, target, attempts, retry.budget, err)
		}

		wait := delay
		if wait > remaining {
			wait = remaining
		}
		logger.Infow(fmt.Sprintf("[Startup] the %s connection is not available yet; retrying", name),
			logger.Field("dependency", name),
			logger.Field("address", target),
			logger.Field("attempt", attempts),
			logger.Field("retry_in", wait.String()),
			logger.Field("error", err.Error()))

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, startupCanceled(name, target, attempts, ctx.Err())
		case <-timer.C:
		}

		if delay < retry.maxDelay {
			delay *= 2
			if delay > retry.maxDelay {
				delay = retry.maxDelay
			}
		}
	}
}

// startupCanceled reports retries stopped by the process context, not by a
// spent budget: a signal during start-up must end the process quietly.
func startupCanceled(name, target string, attempts int, err error) error {
	logger.Infow(fmt.Sprintf("[Startup] the %s connection retries stopped; the process is shutting down", name),
		logger.Field("dependency", name),
		logger.Field("address", target),
		logger.Field("attempts", attempts),
		logger.Field("error", err.Error()))
	return fmt.Errorf("%s startup connection canceled after %d attempt(s): %w", name, attempts, err)
}

// startupExhausted reports the last connection failure once the budget is
// spent. The error wraps that failure, so the caller logs the real reason the
// dependency stayed unavailable rather than a generic timeout.
func startupExhausted(name, target string, attempts int, budget time.Duration, err error) error {
	logger.Errorw(fmt.Sprintf("[Startup] the %s connection did not come up before the retry budget ran out; giving up", name),
		logger.Field("dependency", name),
		logger.Field("address", target),
		logger.Field("attempts", attempts),
		logger.Field("budget", budget.String()),
		logger.Field("error", err.Error()))
	return fmt.Errorf("%s startup connection failed after %d attempt(s) within %s: %w", name, attempts, budget, err)
}

// connectDatabase opens the database pool the configuration describes. It
// first dials the address with the attempt's context: the MySQL driver's
// own dial runs under gorm.Open without one, so a host that drops packets
// could hold the process past its retry budget. Once the address accepts,
// gorm.Open performs the handshake with the driver's normal settings, so the
// pool the process runs on is unchanged.
func connectDatabase(ctx context.Context, cfg orm.Config) (*gorm.DB, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("dial database at %s: %w", cfg.Addr, err)
	}
	// The connection only proves the address accepts; gorm.Open opens the
	// pool's own connections.
	_ = conn.Close()

	db, err := orm.ConnectMysql(orm.Mysql{Config: cfg})
	if err != nil {
		return nil, err
	}
	return db, nil
}

// connectRedis opens a Redis client and pings it. A failed attempt's client
// is closed, so the retries do not leave pools behind; the successful one is
// the application's client.
func connectRedis(ctx context.Context, cfg config.RedisConfig) (*redis.Client, error) {
	rds := redis.NewClient(&redis.Options{
		Addr:     cfg.Host,
		Password: cfg.Pass,
		DB:       cfg.DB,
	})
	if err := rds.Ping(ctx).Err(); err != nil {
		// The client is not returned; closing the failed attempt's pool
		// cannot lose a connection another caller uses.
		_ = rds.Close()
		return nil, err
	}
	return rds, nil
}
