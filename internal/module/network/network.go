// Package network is the facade of the network module: admin-side server and
// node management, the node-facing server API and the edge client manifest.
// See docs/design/adr-001-modular-monolith.md.
package network

import (
	"context"
	"time"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/internal/adminserver"
	"github.com/perfect-panel/server/internal/module/network/internal/edge"
	"github.com/perfect-panel/server/internal/module/network/internal/repo"
	"github.com/perfect-panel/server/internal/module/network/internal/serverapi"
	"github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// Service is the only surface other code may depend on; the implementation
// lives under internal/ where the compiler seals it off.
type Service interface {
	// Admin-side server and node management.
	CreateServer(ctx context.Context, req *dto.CreateServerRequest) (*dto.CreateServerResponse, error)
	UpdateServer(ctx context.Context, req *dto.UpdateServerRequest) error
	DeleteServer(ctx context.Context, req *dto.DeleteServerRequest) error
	FilterServerList(ctx context.Context, req *dto.FilterServerListRequest) (*dto.FilterServerListResponse, error)
	ResetSortWithServer(ctx context.Context, req *dto.ResetSortRequest) error
	GetServerProtocols(ctx context.Context, req *dto.GetServerProtocolsRequest) (*dto.GetServerProtocolsResponse, error)
	GetServerNodeConfig(ctx context.Context, req *dto.GetServerNodeConfigRequest) (*dto.GetServerNodeConfigResponse, error)
	UpdateServerNodeConfig(ctx context.Context, req *dto.UpdateServerNodeConfigRequest) error
	CreateNode(ctx context.Context, req *dto.CreateNodeRequest) error
	UpdateNode(ctx context.Context, req *dto.UpdateNodeRequest) error
	DeleteNode(ctx context.Context, req *dto.DeleteNodeRequest) error
	FilterNodeList(ctx context.Context, req *dto.FilterNodeListRequest) (*dto.FilterNodeListResponse, error)
	ToggleNodeStatus(ctx context.Context, req *dto.ToggleNodeStatusRequest) error
	ResetSortWithNode(ctx context.Context, req *dto.ResetSortRequest) error
	QueryNodeTag(ctx context.Context) (*dto.QueryNodeTagResponse, error)

	// The node-facing server API. The ETag negotiation flows through
	// RequestMeta/ResponseMeta; node authentication stays in the handlers.
	GetServerUserList(ctx context.Context, req *dto.GetServerUserListRequest, meta RequestMeta) (*dto.GetServerUserListResponse, ResponseMeta, error)
	QueryServerProtocolConfig(ctx context.Context, req *dto.QueryServerConfigRequest) (*dto.QueryServerConfigResponse, error)
	PushOnlineUsers(ctx context.Context, req *dto.OnlineUsersRequest) error
	ServerPushStatus(ctx context.Context, req *dto.ServerPushStatusRequest) error
	InvalidateRoutingStatus(ctx context.Context, serverID int64) error
	ServerPushUserTraffic(ctx context.Context, req *dto.ServerPushUserTrafficRequest) error

	// EdgeManifest serves the token-authenticated edge client manifest.
	EdgeManifest(ctx context.Context, token string) (*dto.EdgeManifestResponse, error)
}

// ErrManifestNotFound re-exports the edge subdomain's not-found sentinel for
// the transport handler's status mapping.
var ErrManifestNotFound = edge.ErrManifestNotFound

// RequestMeta and ResponseMeta re-export the server API's conditional-request
// negotiation types for the transport handlers.
type (
	RequestMeta  = serverapi.RequestMeta
	ResponseMeta = serverapi.ResponseMeta
)

// Snapshot is the per-request view of the runtime-mutable settings the
// network flows consume.
type Snapshot struct {
	Node      config.NodeConfig
	Subscribe config.SubscribeConfig
}

// Deps declares everything the module needs; the composition root
// (internal/app) provides them. The module wraps the legacy store during
// migration and will own its persistence once the domain data moves in
// (ADR-001 step 5).
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

// NewRepoBuilder exports the module-owned repository implementations for
// store assembly. The node cache retrier is a background singleton that
// survives per-transaction rebuilds, so it lives in this closure
// (ADR-001 step-6 preparation).
// ReconcileServerCaches is the startup recovery path for invalidations that
// may have been lost with a previous process. It runs before HTTP/workers.
func ReconcileServerCaches(ctx context.Context, rds *redis.Client) error {
	return repo.ReconcileServerCaches(ctx, rds)
}

func NewRepoBuilder(rds *redis.Client) repository.NetworkBuilder {
	retrier := repo.NewServerCacheInvalidationRetrier(rds)
	return func(c repository.ModuleConn) repository.NetworkRepos {
		nodes := repo.NewNodeRepo(c.DB, c.Redis, retrier)
		return repository.NetworkRepos{
			Nodes:    nodes,
			Traffic:  repo.NewTrafficRepo(c.DB),
			NodeKeys: nodes.(repository.NodeCacheKeyBridge),
		}
	}
}

func New(deps Deps) Service {
	return &service{
		admin: adminserver.NewService(adminserver.Deps{
			Store:                deps.Store,
			Redis:                deps.Redis,
			ReadGlobalNodeConfig: deps.ReadGlobalNodeConfig,
		}),
		api: serverapi.NewService(serverapi.Deps{
			TrafficUsage:         deps.TrafficUsage,
			Store:                deps.Store,
			Redis:                deps.Redis,
			ReadGlobalNodeConfig: deps.ReadGlobalNodeConfig,
			Config: func() serverapi.Snapshot {
				cfg := deps.Config()
				return serverapi.Snapshot{Node: cfg.Node, Subscribe: cfg.Subscribe}
			},
			Multiplier: deps.Multiplier,
		}),
		edge: edge.NewService(edge.Deps{
			Store: deps.Store,
			Config: func() edge.Snapshot {
				return edge.Snapshot{Subscribe: deps.Config().Subscribe}
			},
		}),
	}
}

type service struct {
	admin *adminserver.Service
	api   *serverapi.Service
	edge  *edge.Service
}

func (s *service) CreateServer(ctx context.Context, req *dto.CreateServerRequest) (*dto.CreateServerResponse, error) {
	return s.admin.CreateServer(ctx, req)
}

func (s *service) UpdateServer(ctx context.Context, req *dto.UpdateServerRequest) error {
	return s.admin.UpdateServer(ctx, req)
}

func (s *service) DeleteServer(ctx context.Context, req *dto.DeleteServerRequest) error {
	return s.admin.DeleteServer(ctx, req)
}

func (s *service) FilterServerList(ctx context.Context, req *dto.FilterServerListRequest) (*dto.FilterServerListResponse, error) {
	return s.admin.FilterServerList(ctx, req)
}

func (s *service) ResetSortWithServer(ctx context.Context, req *dto.ResetSortRequest) error {
	return s.admin.ResetSortWithServer(ctx, req)
}

func (s *service) GetServerProtocols(ctx context.Context, req *dto.GetServerProtocolsRequest) (*dto.GetServerProtocolsResponse, error) {
	return s.admin.GetServerProtocols(ctx, req)
}

func (s *service) GetServerNodeConfig(ctx context.Context, req *dto.GetServerNodeConfigRequest) (*dto.GetServerNodeConfigResponse, error) {
	return s.admin.GetServerNodeConfig(ctx, req)
}

func (s *service) UpdateServerNodeConfig(ctx context.Context, req *dto.UpdateServerNodeConfigRequest) error {
	return s.admin.UpdateServerNodeConfig(ctx, req)
}

func (s *service) CreateNode(ctx context.Context, req *dto.CreateNodeRequest) error {
	return s.admin.CreateNode(ctx, req)
}

func (s *service) UpdateNode(ctx context.Context, req *dto.UpdateNodeRequest) error {
	return s.admin.UpdateNode(ctx, req)
}

func (s *service) DeleteNode(ctx context.Context, req *dto.DeleteNodeRequest) error {
	return s.admin.DeleteNode(ctx, req)
}

func (s *service) FilterNodeList(ctx context.Context, req *dto.FilterNodeListRequest) (*dto.FilterNodeListResponse, error) {
	return s.admin.FilterNodeList(ctx, req)
}

func (s *service) ToggleNodeStatus(ctx context.Context, req *dto.ToggleNodeStatusRequest) error {
	return s.admin.ToggleNodeStatus(ctx, req)
}

func (s *service) ResetSortWithNode(ctx context.Context, req *dto.ResetSortRequest) error {
	return s.admin.ResetSortWithNode(ctx, req)
}

func (s *service) QueryNodeTag(ctx context.Context) (*dto.QueryNodeTagResponse, error) {
	return s.admin.QueryNodeTag(ctx)
}

func (s *service) GetServerUserList(ctx context.Context, req *dto.GetServerUserListRequest, meta RequestMeta) (*dto.GetServerUserListResponse, ResponseMeta, error) {
	return s.api.GetServerUserList(ctx, req, meta)
}

func (s *service) QueryServerProtocolConfig(ctx context.Context, req *dto.QueryServerConfigRequest) (*dto.QueryServerConfigResponse, error) {
	return s.api.QueryServerProtocolConfig(ctx, req)
}

func (s *service) PushOnlineUsers(ctx context.Context, req *dto.OnlineUsersRequest) error {
	return s.api.PushOnlineUsers(ctx, req)
}

func (s *service) ServerPushStatus(ctx context.Context, req *dto.ServerPushStatusRequest) error {
	return s.api.ServerPushStatus(ctx, req)
}
func (s *service) InvalidateRoutingStatus(ctx context.Context, serverID int64) error {
	return s.api.InvalidateRoutingStatus(ctx, serverID)
}

func (s *service) ServerPushUserTraffic(ctx context.Context, req *dto.ServerPushUserTrafficRequest) error {
	return s.api.ServerPushUserTraffic(ctx, req)
}

func (s *service) EdgeManifest(ctx context.Context, token string) (*dto.EdgeManifestResponse, error) {
	return s.edge.Manifest(ctx, token)
}

// Store is the persistence capability required by this package. It excludes
// unrelated repositories and application-wide transactions.
type Store interface {
	adminserver.Store
	serverapi.Store
	edge.Store
}
