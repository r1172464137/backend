// Package adminserver is the network module's admin-side management of the
// servers and nodes: their creation, updates and removal, their sort order,
// the protocols a server offers and its node configuration overrides, and the
// node-facing caches these changes invalidate. Only the module facade may
// reach it.
package adminserver

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

type Snapshot struct{ Node config.NodeConfig }

// Deps declares the subdomain's dependencies; the module facade forwards
// them from the composition root.
type Deps struct {
	Store                Store
	Redis                *redis.Client
	ReadGlobalNodeConfig func(context.Context) (config.NodeConfig, error)
	DesiredSnapshotID    func(context.Context, int64, dto.RoutingStatusView) (*string, error)
	RoutingAccounts      config.RoutingAccountLookup
	Subscriptions        OnlineSubscriptionReader
	Config               func() Snapshot
}

// Service is the admin server-management entry point used by the network
// facade.
type Service struct {
	deps Deps
}

// NewService returns the subdomain's service over deps.
func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

// Store is the persistence capability required by this package. It excludes
// unrelated repositories and application-wide transactions.
type Store interface {
	repository.NetworkTransactor
	Node() repository.NodeRepo
}
