package app

import (
	"context"
	"errors"
	"github.com/perfect-panel/server/pkg/logger"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/app/state"
	"github.com/perfect-panel/server/internal/auth/ratelimit"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/eventbus"
	"github.com/perfect-panel/server/internal/infra/geoip"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
	"github.com/perfect-panel/server/internal/module/billing"
	"github.com/perfect-panel/server/internal/module/identity"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/internal/module/notification"
	"github.com/perfect-panel/server/internal/module/platform"
	"github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/internal/module/support"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/internal/transport/devicesocket"
	"github.com/perfect-panel/server/pkg/orm"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Application struct {
	Redis        *redis.Client
	Runtime      *state.State
	Queue        *taskqueue.Client
	Inspector    *asynq.Inspector
	ExchangeRate *billing.CurrencyRateCache
	GeoIP        *geoip.IPLocation
	Store        repository.Store

	// Domain modules (see docs/design/adr-001-modular-monolith.md). Application is
	// their composition root; handlers call the module facades.
	Support      support.Service
	Billing      billing.Service
	Platform     platform.Service
	Subscription subscription.Service
	Identity     identity.Service
	Network      network.Service
	Notification notification.Service
	EventBus     *eventbus.Bus

	//NodeCache   *cache.NodeCacheClient
	AuthLimiter   *ratelimit.PeriodLimit
	DeviceManager *devicesocket.DeviceManager

	db *gorm.DB
}

// NewApplication connects the database and Redis and assembles the modules.
// Dependencies that are not ready yet are retried within DefaultStartupRetry;
// cancelling ctx (SIGTERM during startup) aborts the wait. Errors name the
// dependency and the last cause instead of panicking.
func NewApplication(ctx context.Context, c config.Config) (*Application, error) {
	return newApplication(ctx, c, DefaultStartupRetry)
}

func newApplication(ctx context.Context, c config.Config, retry RetryPolicy) (*Application, error) {
	dbConfig := orm.Mysql{Config: c.DatabaseConfig()}
	db, err := connectWithRetry(ctx, "database", retry, func(actx context.Context) (*gorm.DB, error) {
		return orm.OpenDatabase(actx, dbConfig)
	}, permanentDatabaseError)
	if err != nil {
		return nil, err
	}
	closeDB := func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	}

	// IP location initialize
	// GeoIP only enriches logs and lookups; without it the panel still runs (90156b8d).
	geoIP, err := geoip.NewIPLocation("./cache/GeoLite2-City.mmdb")
	if err != nil {
		logger.Errorf("[GeoIP] unavailable, continuing without geolocation: %v", err)
		geoIP = nil
	}

	rds := redis.NewClient(&redis.Options{
		Addr:     c.Redis.Host,
		Password: c.Redis.Pass,
		DB:       c.Redis.DB,
	})
	if _, err = connectWithRetry(ctx, "redis", retry, func(actx context.Context) (struct{}, error) {
		return struct{}{}, rds.Ping(actx).Err()
	}, permanentRedisError); err != nil {
		_ = rds.Close()
		closeDB()
		return nil, err
	}
	authLimiter := ratelimit.NewPeriodLimit(86400, 15, rds, config.SendCountLimitKeyPrefix, ratelimit.Align())
	store := NewStore(db, rds)
	queue := NewAsynqClient(c)
	rate := billing.NewCurrencyRateCache(0)
	srv := &Application{
		Redis:        rds,
		Runtime:      state.New(c),
		Queue:        queue,
		Inspector:    NewAsynqInspector(c),
		ExchangeRate: rate,
		GeoIP:        geoIP,
		Store:        store,
		//NodeCache:   cache.NewNodeCacheClient(rds),
		AuthLimiter: authLimiter,
		db:          db,
	}
	// Support takes srv for the ticket→Telegram mirror; the adapter reads
	// srv.Notification lazily, so constructing it before Notification is safe.
	srv.Support = newSupportModule(store, queue, srv)
	srv.Billing = newBillingModule(c, store, queue, rds, rate, srv)
	srv.Platform = newPlatformModule(store, srv)
	srv.DeviceManager = NewDeviceManager(srv)
	srv.Subscription = newSubscriptionModule(store, srv)
	srv.Identity = newIdentityModule(store, srv)
	srv.Network = newNetworkModule(store, srv)
	srv.Notification = newNotificationModule(store, srv)
	srv.EventBus = newEventBus(store, srv)
	return srv, nil
}

// Ready reports whether the backend can serve requests that need its
// dependencies: the database answers a ping and Redis answers PING within ctx.
// The returned error names the failing dependency only (no addresses or
// credentials).
func (a *Application) Ready(ctx context.Context) error {
	sqlDB, err := a.db.DB()
	if err != nil {
		return errors.New("database unavailable")
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return errors.New("database unavailable")
	}
	if err := a.Redis.Ping(ctx).Err(); err != nil {
		return errors.New("redis unavailable")
	}
	return nil
}
