package app

import (
	"time"

	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/internal/repository"
)

// newNetworkModule wires the network module against the legacy store; the
// node/subscribe configuration is runtime-mutable, so the module receives a
// per-request snapshot closure.
func newNetworkModule(store repository.Store, srv *Application) network.Service {
	return network.New(network.Deps{
		Store:                store,
		TrafficUsage:         subscription.NewTrafficUsage(store),
		Redis:                srv.Redis,
		ReadGlobalNodeConfig: srv.Platform.ReadNodeConfigSnapshot,
		Config: func() network.Snapshot {
			current := srv.Runtime.Config()
			return network.Snapshot{
				Node:      current.Node,
				Subscribe: current.Subscribe,
			}
		},
		Multiplier: func(at time.Time) float32 {
			manager := srv.Runtime.NodeMultiplierManager()
			if manager == nil {
				return 1
			}
			return manager.GetMultiplier(at)
		},
	})
}
