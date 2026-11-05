// Package serverapi is the network module's node-facing server API: the
// configuration and user lists the nodes pull, cached per server and
// negotiated with ETags, and the status, online-user and traffic reports they
// push. Only the module facade may reach it.
package serverapi

import (
	"context"
	"errors"
	"time"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/network/internal/routingstatus"
	"github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// Snapshot is the per-request view of the runtime-mutable settings the node
// API consumes.
type Snapshot struct {
	Node      config.NodeConfig
	Subscribe config.SubscribeConfig
}

// Deps declares the subdomain's dependencies; the module facade forwards
// them from the composition root.
type Deps struct {
	Store                Store
	TrafficUsage         subscription.TrafficUsage
	Redis                *redis.Client
	ReadGlobalNodeConfig func(context.Context) (config.NodeConfig, error)
	// Config snapshots the runtime-mutable settings per request.
	Config func() Snapshot
	// Multiplier returns the node traffic multiplier in effect at the given
	// time; nil means no multiplier is configured.
	Multiplier func(at time.Time) float32

	// The ports list only what the node API uses of each dependency, so a
	// fake implements exactly that. Servers reads the server rows,
	// Overrides their configuration overrides and Nodes a server's nodes.
	Servers   ServerReader
	Overrides OverrideReader
	Nodes     NodeLister
	// Caches keeps a server's node-facing responses under its cache
	// generation.
	Caches ServerCache
	// Status records the status reports, Online the online users.
	Status StatusRecorder
	Online OnlineRecorder
	// Subscriptions is the subscription read port (the subscription
	// facade): the subscriptions a server's nodes serve.
	Subscriptions SubscriptionReader
	// Accounts is the identity read port: which subscription owners have
	// an enabled account.
	Accounts AccountReader
}

// ServerReader reads a server row.
type ServerReader interface {
	FindOneServer(ctx context.Context, id int64) (*node.Server, error)
}

// OverrideReader reads a server's configuration override; nil means none.
type OverrideReader interface {
	FindServerConfigOverride(ctx context.Context, serverId int64) (*node.ServerConfigOverride, error)
}

// NodeLister lists the nodes a filter selects.
type NodeLister interface {
	ListNodes(ctx context.Context, params *node.FilterNodeParams) ([]*node.Node, error)
}

// ServerCache stores a server's node-facing responses only while the cache
// generation read before building them is still current.
type ServerCache interface {
	ServerCacheGeneration(ctx context.Context, serverId int64) (int64, error)
	SetServerCache(ctx context.Context, serverId int64, key string, value any, generation int64) error
}

// StatusRecorder records a status report: the reported status, a changed
// certificate pin, and the invalidation of the server's caches that carry
// the old pin.
type StatusRecorder interface {
	UpdateStatusCache(ctx context.Context, serverId int64, status *node.Status) error
	UpdateServerProtocolsIfCurrent(ctx context.Context, id int64, current, updated string) (bool, error)
	ClearServerCache(ctx context.Context, serverId int64) error
}

// OnlineRecorder records the users a server reports online.
type OnlineRecorder interface {
	UpdateOnlineUserSubscribe(ctx context.Context, serverId int64, protocol string, subscribe node.OnlineUserSubscribe) error
	UpdateOnlineUserSubscribeGlobal(ctx context.Context, subscribe node.OnlineUserSubscribe) error
}

// SubscriptionReader is the subscription read port (the subscription
// facade): the subscriptions the nodes of a scope serve, and one by id.
type SubscriptionReader interface {
	ServableSubscriptionsByNodeScope(ctx context.Context, nodeIDs []int64, tags []string) ([]subscription.ServedSubscription, error)
	SubscriptionByID(ctx context.Context, id int64) (*usersub.Subscribe, error)
}

// AccountReader reports which of the accounts are enabled.
type AccountReader interface {
	FindEnabledUserIDs(ctx context.Context, ids []int64) ([]int64, error)
}

// Service is the node-facing API entry point used by the network facade.
type Service struct {
	deps Deps
}

// NewService returns the node API's service over deps.
func NewService(deps Deps) *Service {
	// Older callers provide the store rather than the individual ports.
	// Keep that composition valid while letting explicit ports override it.
	if deps.Store != nil {
		nodes := deps.Store.Node()
		if deps.Servers == nil {
			deps.Servers = nodes
		}
		if deps.Overrides == nil {
			deps.Overrides = nodes
		}
		if deps.Nodes == nil {
			deps.Nodes = nodes
		}
		if deps.Caches == nil {
			deps.Caches = nodes
		}
		if deps.Status == nil {
			deps.Status = nodes
		}
		if deps.Online == nil {
			deps.Online = nodes
		}
	}
	return &Service{deps: deps}
}

// DesiredSnapshotID derives the current snapshot for the verified node's exact filter.
func (s *Service) DesiredSnapshotID(ctx context.Context, serverID int64, status dto.RoutingStatusView) (*string, error) {
	if !status.Known || !status.ReporterVerified || status.Report == nil || status.Report.Version != 2 {
		return nil, nil
	}
	var identity *dto.RoutingCandidateIdentity
	var failed *dto.RoutingCandidateIdentity
	if status.Report.Error != nil {
		failed = status.Report.Error.Identity
	}
	for _, id := range []*dto.RoutingCandidateIdentity{status.Report.Pending, status.Report.Applied, failed} {
		if id != nil && id.SnapshotID != "" {
			identity = id
			break
		}
	}
	if identity == nil {
		return nil, nil
	}
	filter, known, err := s.desiredFilter(ctx, serverID, identity.SnapshotID)
	if err != nil {
		return nil, err
	}
	if !known {
		return nil, nil
	}
	// Administrative observation must not activate subscriptions or publish a
	// delivery filter for a candidate that no node has fetched.
	logic := newQueryServerProtocolConfigLogic(ctx, s.deps)
	response, err := logic.v2Snapshot(&dto.QueryServerConfigRequest{ServerID: serverID, Protocols: filter}, filter)
	if errors.Is(err, errPendingSnapshotSubscription) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if response.IdentitySnapshot == nil || response.Routing.Version != 2 {
		return nil, nil
	}
	id := response.IdentitySnapshot.SnapshotID
	return &id, nil
}

// desiredFilter distinguishes a missing delivery from a Redis failure. A
// missing or invalid filter cannot establish any delivered candidate.
func (s *Service) desiredFilter(ctx context.Context, serverID int64, snapshotID string) ([]string, bool, error) {
	if s.deps.Redis == nil {
		return nil, false, nil
	}
	filter, err := routingstatus.ReadFilter(ctx, s.deps.Redis, serverID, snapshotID)
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	filter, known := canonicalDesiredFilter(filter)
	return filter, known, nil
}

func canonicalDesiredFilter(filter []string) ([]string, bool) {
	canonical, err := routingstatus.CanonicalFilter(filter)
	return canonical, err == nil
}

// InvalidateRoutingStatus records no untrusted body or error text.
func (s *Service) InvalidateRoutingStatus(ctx context.Context, serverID int64) error {
	if serverID <= 0 {
		return nil
	}
	if _, err := s.deps.Store.Node().FindOneServer(ctx, serverID); err != nil {
		return err
	}
	return routingstatus.MarkUnknown(ctx, s.deps.Redis, serverID)
}

// Store is the persistence capability required by this package. It excludes
// unrelated repositories and application-wide transactions.
type Store interface {
	Node() repository.NodeRepo
}
