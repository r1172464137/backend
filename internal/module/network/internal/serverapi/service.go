// Service assembly for the node-facing server API subdomain of the network
// module: config pulls, user lists, status/traffic pushes. Only the module
// facade may reach it.
package serverapi

import (
	"context"
	"errors"
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
	filter, err := routingstatus.ReadFilter(ctx, s.deps.Redis, serverID, identity.SnapshotID)
	if err != nil {
		return nil, nil
	} // Missing filter is unknown, never an assumed all-protocol delivery.
	filter, err = routingstatus.CanonicalFilter(filter)
	if err != nil {
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
