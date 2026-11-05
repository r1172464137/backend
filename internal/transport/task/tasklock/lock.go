// Package tasklock keeps two runs of one periodic task from overlapping,
// across replicas, with a Redis lock that only its owner may release or
// extend.
package tasklock

import (
	"context"
	"errors"
	"sync"
	"time"

	"uuid"

	"github.com/redis/go-redis/v9"
)

// ErrLost reports a heartbeat that found the lock no longer held by this
// run: it expired, and possibly another run took it over.
var ErrLost = errors.New("task lock is no longer held by this run")

// releaseScript deletes the lock only while it still holds the owner's token:
// a run that outlived its TTL must not free the lock of the run that took it
// over.
var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

// extendScript refreshes the TTL only while the lock still holds the owner's
// token, for the same reason.
var extendScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`)

// Lock is a held task lock.
type Lock struct {
	client *redis.Client
	key    string
	token  string
}

// Acquire takes the lock at key for ttl. ok is false, with a nil error, when
// another run holds it.
func Acquire(ctx context.Context, client *redis.Client, key string, ttl time.Duration) (lock *Lock, ok bool, err error) {
	token := uuid.NewV7().String()
	ok, err = client.SetNX(ctx, key, token, ttl).Result()
	if err != nil || !ok {
		return nil, false, err
	}
	return &Lock{client: client, key: key, token: token}, true, nil
}

// Release frees the lock if this run still owns it and reports whether it
// did; an expired lock taken over by another run is left alone.
func (l *Lock) Release(ctx context.Context) (bool, error) {
	// Release after the run's context ended (a cancelled task) still frees
	// the lock instead of leaving it to its TTL.
	deleted, err := releaseScript.Run(context.WithoutCancel(ctx), l.client, []string{l.key}, l.token).Int()
	return deleted == 1, err
}

// Extend gives the lock ttl more from now if this run still owns it and
// reports whether it did; a lock that expired, or that another run took
// over, is left alone.
func (l *Lock) Extend(ctx context.Context, ttl time.Duration) (bool, error) {
	extended, err := extendScript.Run(ctx, l.client, []string{l.key}, l.token, ttl.Milliseconds()).Int()
	return extended == 1, err
}

// KeepAlive extends the lock by ttl every third of ttl until stop is called
// or ctx ends, so a run that outlives its TTL keeps the lock a replica
// would otherwise take over. report, if not nil, is told of a failed
// extension: ErrLost when the lock is gone, after which the heartbeat
// stops, or the Redis error, after which it keeps trying. Call stop before
// Release; it waits for the heartbeat to end.
func (l *Lock) KeepAlive(ctx context.Context, ttl time.Duration, report func(error)) (stop func()) {
	interval := ttl / 3
	if interval <= 0 {
		interval = time.Second
	}
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				extended, err := l.Extend(ctx, ttl)
				if err != nil {
					if report != nil {
						report(err)
					}
					continue
				}
				if !extended {
					if report != nil {
						report(ErrLost)
					}
					return
				}
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			<-finished
		})
	}
}
