package repo

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ServerCacheInvalidationRetrier coalesces failed server-response cache
// invalidations. It is shared by every Store derived from one root Store, so
// a Redis outage cannot create one retry goroutine per successful DB write.
type ServerCacheInvalidationRetrier struct {
	invalidate func(context.Context, int64) error
	delay      time.Duration

	mu        sync.Mutex
	serverIDs map[int64]uint64
	sequence  uint64
	running   bool
}

// NewServerCacheInvalidationRetrier builds the background retry singleton
// for node cache invalidation.
func NewServerCacheInvalidationRetrier(client *redis.Client) *ServerCacheInvalidationRetrier {
	return newServerCacheInvalidationRetrierWithFunc(func(ctx context.Context, serverID int64) error {
		return clearServerCache(ctx, client, serverID)
	}, time.Second)
}

func newServerCacheInvalidationRetrierWithFunc(invalidate func(context.Context, int64) error, delay time.Duration) *ServerCacheInvalidationRetrier {
	if delay <= 0 {
		delay = time.Second
	}
	return &ServerCacheInvalidationRetrier{
		invalidate: invalidate,
		delay:      delay,
		serverIDs:  make(map[int64]uint64),
	}
}

// Enqueue schedules the invalidation of the servers' caches, starting the
// retry loop unless it is already running.
func (r *ServerCacheInvalidationRetrier) Enqueue(serverIDs ...int64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	for _, serverID := range serverIDs {
		if serverID > 0 {
			r.sequence++
			r.serverIDs[serverID] = r.sequence
		}
	}
	if len(r.serverIDs) == 0 || r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.mu.Unlock()
	go r.run()
}

// run retries the pending invalidations, backing off while any fails, until
// none is left.
func (r *ServerCacheInvalidationRetrier) run() {
	delay := r.delay
	for {
		pending := r.snapshot()
		if len(pending) == 0 {
			r.mu.Lock()
			if len(r.serverIDs) == 0 {
				r.running = false
				r.mu.Unlock()
				return
			}
			r.mu.Unlock()
			continue
		}

		failed := make(map[int64]struct{})
		for serverID := range pending {
			// The loop outlives the requests whose writes queued these
			// servers and serves all of them, so it bounds each attempt
			// under its own root context rather than any request's.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := r.invalidate(ctx, serverID)
			cancel()
			if err != nil {
				failed[serverID] = struct{}{}
			}
		}
		r.removeSucceeded(pending, failed)
		if len(failed) == 0 {
			delay = r.delay
			continue
		}
		time.Sleep(delay)
		if delay < 30*time.Second {
			delay *= 2
		}
	}
}

func (r *ServerCacheInvalidationRetrier) snapshot() map[int64]uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	serverIDs := make(map[int64]uint64, len(r.serverIDs))
	for serverID, sequence := range r.serverIDs {
		serverIDs[serverID] = sequence
	}
	return serverIDs
}

func (r *ServerCacheInvalidationRetrier) removeSucceeded(serverIDs map[int64]uint64, failed map[int64]struct{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for serverID, sequence := range serverIDs {
		if _, failed := failed[serverID]; !failed && r.serverIDs[serverID] == sequence {
			delete(r.serverIDs, serverID)
		}
	}
}
