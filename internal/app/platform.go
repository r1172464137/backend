package app

import (
	"context"
	"time"

	"github.com/oschwald/geoip2-golang"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/mapping"
	"github.com/perfect-panel/server/internal/module/platform"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/repository"
)

// newPlatformModule wires the platform module against the legacy store. The
// log-retention callbacks read and mutate the running configuration exactly
// as the legacy logic did.
func newPlatformModule(store repository.Store, srv *Application) platform.Service {
	return platform.New(platform.Deps{
		Logs:    store.Log(),
		System:  store.System(),
		Traffic: store.TrafficLog(),
		Store:   store,
		Orders:  store.Order(),
		Users:   store.User(),
		MissingAccounts: func(ctx context.Context, ids []int64) ([]int64, error) {
			return repository.MissingUserIDs(ctx, store.User(), ids)
		},
		Tickets: store.Ticket(),
		Nodes:   store.Node(),
		Cache:   srv.Redis,
		OnLogSettingChanged: func(autoClear bool, clearDays int64) {
			srv.Runtime.UpdateConfig(func(current *config.Config) {
				current.Log = config.Log{AutoClear: autoClear, ClearDays: clearDays}
			})
		},
		LogRetention: func() (bool, int64) {
			current := srv.Runtime.Config().Log
			return current.AutoClear, current.ClearDays
		},
		Reinitialize: srv.Runtime.Reinitialize,
		Restart:      srv.Runtime.Restart,
		SubscribePath: func() string {
			return srv.Runtime.Config().Subscribe.SubscribePath
		},
		ApplyVerifyConfig: func(req *dto.VerifyConfig) {
			srv.Runtime.UpdateConfig(func(current *config.Config) {
				mapping.DeepCopy(&current.Verify, req)
			})
		},
		Multiplier: func(at time.Time) float32 {
			manager := srv.Runtime.NodeMultiplierManager()
			if manager == nil {
				return 1
			}
			return manager.GetMultiplier(at)
		},
		PublicStore: store,
		Redis:       srv.Redis,
		PublicConfig: func() platform.GlobalConfigSnapshot {
			c := srv.Runtime.Config()
			return platform.GlobalConfigSnapshot{
				Site:      c.Site,
				Subscribe: c.Subscribe,
				Email:     c.Email,
				Mobile:    c.Mobile,
				Register:  c.Register,
				Verify:    c.Verify,
				Invite:    c.Invite,
			}
		},
		LogPath: srv.Runtime.Config().Logger.Path,
		GeoIP: func() *geoip2.Reader {
			if srv.GeoIP == nil {
				return nil
			}
			return srv.GeoIP.DB
		},
	})
}
