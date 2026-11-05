package cache

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var (
	// ErrNotFound is the error when cache not found.
	ErrNotFound = redis.Nil

	cacheVersionPrefix  = "cache:version:"
	cacheNotFoundPrefix = "cache:not-found:"
	setIfVersionScript  = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if not current then current = '0' end
if current == ARGV[1] then
  return redis.call('SET', KEYS[2], ARGV[2], 'PX', ARGV[3])
end
return 0
`)
)

type (
	// ExecCtxFn defines the sql exec method.
	ExecCtxFn func(conn *gorm.DB) error
	// QueryCtxFn defines the query method.
	QueryCtxFn func(conn *gorm.DB, v any) error

	// CachedConn is a database connection with the cache in front of it;
	// the zero value is not usable, see NewConn.
	CachedConn struct {
		db             *gorm.DB
		cache          *redis.Client
		expiry         time.Duration
		notFoundExpiry time.Duration
		invalidations  *InvalidationQueue
	}
)

// NewConn returns a CachedConn with a redis cluster cache.
func NewConn(db *gorm.DB, c *redis.Client, opts ...Option) CachedConn {
	o := newOptions(opts...)
	return CachedConn{
		db:             db,
		cache:          c,
		expiry:         o.Expiry,
		notFoundExpiry: o.NotFoundExpiry,
		invalidations:  o.Invalidations,
	}
}

// DelCacheCtx deletes cache with keys.
func (cc CachedConn) DelCacheCtx(ctx context.Context, keys ...string) error {
	if cc.invalidations != nil {
		cc.invalidations.Add(keys...)
		return nil
	}
	return cc.invalidateCacheKeys(ctx, keys...)
}

// GetCacheCtx unmarshals cache with given key and context into v.
func (cc CachedConn) GetCacheCtx(ctx context.Context, key string, v any) error {
	// query redis key
	val, err := cc.cache.Get(ctx, key).Result()
	if err != nil {
		return err
	}
	// unmarshal value
	return json.Unmarshal([]byte(val), v)
}

// SetCacheCtx sets cache with key, value, and context.
func (cc CachedConn) SetCacheCtx(ctx context.Context, key string, v any) error {
	version, err := cc.cacheVersion(ctx, key)
	if err != nil {
		return err
	}
	return cc.setCacheIfVersion(ctx, key, version, v)
}

func cacheVersionKey(key string) string {
	return cacheVersionPrefix + key
}

func cacheNotFoundKey(key string) string {
	return cacheNotFoundPrefix + key
}

func (cc CachedConn) cacheVersion(ctx context.Context, key string) (string, error) {
	version, err := cc.cache.Get(ctx, cacheVersionKey(key)).Result()
	if errors.Is(err, redis.Nil) {
		return "0", nil
	}
	return version, err
}

func (cc CachedConn) setCacheIfVersion(ctx context.Context, key, version string, v any) error {
	value, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return cc.setRawCacheIfVersion(ctx, key, key, version, value, cc.expiry)
}

func (cc CachedConn) setRawCacheIfVersion(ctx context.Context, versionedKey, targetKey, version string, value []byte, expiry time.Duration) error {
	_, err := setIfVersionScript.Run(ctx, cc.cache,
		[]string{cacheVersionKey(versionedKey), targetKey}, version, value, expiry.Milliseconds()).Result()
	return err
}

func (cc CachedConn) setNotFoundIfVersion(ctx context.Context, key, version string) error {
	return cc.setRawCacheIfVersion(ctx, key, cacheNotFoundKey(key), version, []byte("1"), cc.notFoundExpiry)
}

func (cc CachedConn) invalidateCacheKeys(ctx context.Context, keys ...string) error {
	return invalidateCacheKeys(ctx, cc.cache, keys...)
}

// ExecCtx runs given exec on given keys, and returns execution result.
func (cc CachedConn) ExecCtx(ctx context.Context, execCtx ExecCtxFn, keys ...string) error {
	err := execCtx(cc.db.WithContext(ctx))
	if err != nil {
		return err
	}
	// The database mutation is already durable at this point (unless the
	// connection belongs to Store.InTx, where invalidation is queued). Cache
	// invalidation is best-effort so a Redis outage must not make callers retry
	// an operation that has already succeeded in the database.
	_ = cc.DelCacheCtx(ctx, keys...)
	return nil
}

// ExecNoCacheCtx runs exec with given sql statement, without affecting cache.
func (cc CachedConn) ExecNoCacheCtx(ctx context.Context, execCtx ExecCtxFn) (err error) {
	return execCtx(cc.db.WithContext(ctx))
}

// QueryCtx fills v from the cache entry key, or on a miss by running query
// and caching its result; a query that finds no record is remembered for
// the not-found expiry, so repeated lookups of a missing key stay off the
// database. Redis failures fall back to the database, and so does every
// read inside a transaction, which must see the transaction's own writes.
func (cc CachedConn) QueryCtx(ctx context.Context, v any, key string, query QueryCtxFn) (err error) {
	// A transaction must always read through its GORM connection. Reading Redis
	// here could return a value from before an earlier write in the same
	// transaction, violating read-your-writes semantics.
	if cc.invalidations != nil {
		return query(cc.db.WithContext(ctx), v)
	}

	err = cc.GetCacheCtx(ctx, key, v)
	if err == nil {
		return nil
	}

	cacheVersion, versionErr := cc.cacheVersion(ctx, key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			if versionErr == nil && cc.notFoundExpiry > 0 {
				if _, negativeErr := cc.cache.Get(ctx, cacheNotFoundKey(key)).Result(); negativeErr == nil {
					return gorm.ErrRecordNotFound
				}
			}
			// Cache miss (redis.Nil): query DB and cache result. The version
			// fence prevents a read that started before a concurrent committed
			// write from repopulating an invalidated key afterwards.
			err = query(cc.db.WithContext(ctx), v)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) && versionErr == nil && cc.notFoundExpiry > 0 {
					_ = cc.setNotFoundIfVersion(ctx, key, cacheVersion)
				}
				return err
			}
			if versionErr == nil {
				_ = cc.setCacheIfVersion(ctx, key, cacheVersion, v)
			}
			return nil
		}

		// Non-redis.Nil errors: could be JSON unmarshal error (corrupt
		// cache entry) or Redis connection/timeout read error.
		//
		// If JSON error: delete bad key best-effort, then fallback to DB.
		// If Redis error: fallback to DB directly (no bad key to purge).
		var jsonSynErr *json.SyntaxError
		var jsonTypeErr *json.UnmarshalTypeError
		if errors.As(err, &jsonSynErr) || errors.As(err, &jsonTypeErr) {
			// Delete corrupt cache key best-effort
			_ = cc.invalidateCacheKeys(ctx, key)
		}

		// Fallback to DB query
		err = query(cc.db.WithContext(ctx), v)
		if err != nil {
			return err
		}

		if versionErr == nil {
			_ = cc.setCacheIfVersion(ctx, key, cacheVersion, v)
		}
		return nil
	}
	return nil
}

// QueryNoCacheCtx runs query with given sql statement, without affecting cache.
func (cc CachedConn) QueryNoCacheCtx(ctx context.Context, v any, query QueryCtxFn) (err error) {
	return query(cc.db.WithContext(ctx), v)
}

// TransactCtx runs given fn in transaction mode.
func (cc CachedConn) TransactCtx(ctx context.Context, fn func(db *gorm.DB) error, opts ...*sql.TxOptions) error {
	return cc.db.WithContext(ctx).Transaction(fn, opts...)
}
