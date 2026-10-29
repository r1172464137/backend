// Service assembly for the node-facing server API subdomain of the network
// module: config pulls, user lists, status/traffic pushes. Only the module
// facade may reach it.
package serverapi

import (
	"context"
	"time"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/internal/routingstatus"
	"github.com/perfect-panel/server/internal/module/subscription"
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
}

// Service is the node-facing API entry point used by the network facade.
type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) GetServerUserList(ctx context.Context, req *dto.GetServerUserListRequest, meta RequestMeta) (*dto.GetServerUserListResponse, ResponseMeta, error) {
	l := newGetServerUserListLogic(ctx, s.deps, meta)
	resp, err := l.GetServerUserList(req)
	return resp, l.ResponseMeta(), err
}

func (s *Service) QueryServerProtocolConfig(ctx context.Context, req *dto.QueryServerConfigRequest) (*dto.QueryServerConfigResponse, error) {
	return newQueryServerProtocolConfigLogic(ctx, s.deps).QueryServerProtocolConfig(req)
}

func (s *Service) PushOnlineUsers(ctx context.Context, req *dto.OnlineUsersRequest) error {
	return newPushOnlineUsersLogic(ctx, s.deps).PushOnlineUsers(req)
}

func (s *Service) ServerPushStatus(ctx context.Context, req *dto.ServerPushStatusRequest) error {
	return newServerPushStatusLogic(ctx, s.deps).ServerPushStatus(req)
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

func (s *Service) ServerPushUserTraffic(ctx context.Context, req *dto.ServerPushUserTrafficRequest) error {
	return newServerPushUserTrafficLogic(ctx, s.deps).ServerPushUserTraffic(req)
}

// Store is the persistence capability required by this package. It excludes
// unrelated repositories and application-wide transactions.
type Store interface {
	Inbox() repository.InboxRepo
	repository.NetworkTransactor
	Node() repository.NodeRepo
	Subscribe() repository.SubscribeRepo
	User() repository.UserRepo
	UserSubscription() repository.UserSubscriptionRepo
}
