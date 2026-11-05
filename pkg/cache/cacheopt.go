// Package cache is the read-through Redis cache of the GORM repositories.
// CachedConn runs a repository's queries and writes: a cached query reads
// Redis first and fills it from the database on a miss (a missing record is
// remembered briefly too), and a write invalidates the keys it names. A
// per-key version fence keeps a read that raced a committed write from
// putting the old value back, and inside a transaction invalidations wait in
// an InvalidationQueue until the transaction commits, so no entry is filled
// from uncommitted data.
package cache

import "time"

const (
	defaultExpiry         = time.Hour * 24 * 7
	defaultNotFoundExpiry = time.Minute
)

type (
	// Options is used to store the cache options.
	Options struct {
		Expiry         time.Duration
		NotFoundExpiry time.Duration
		Invalidations  *InvalidationQueue
	}

	// Option defines the method to customize an Options.
	Option func(o *Options)
)

func newOptions(opts ...Option) Options {
	var o Options
	for _, opt := range opts {
		opt(&o)
	}

	if o.Expiry <= 0 {
		o.Expiry = defaultExpiry
	}
	if o.NotFoundExpiry <= 0 {
		o.NotFoundExpiry = defaultNotFoundExpiry
	}

	return o
}

// WithInvalidationQueue defers cache-key invalidation until the owner flushes
// the queue. It is used by database transactions so cache entries cannot be
// repopulated from data that has not committed yet.
func WithInvalidationQueue(queue *InvalidationQueue) Option {
	return func(o *Options) {
		o.Invalidations = queue
	}
}
