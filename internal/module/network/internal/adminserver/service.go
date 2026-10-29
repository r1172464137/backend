// Service assembly for the admin-side server and node management subdomain
// of the network module. Only the module facade may reach it.
package adminserver

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// Deps declares the subdomain's dependencies; the module facade forwards
// them from the composition root.
type Deps struct {
	Store                Store
	Redis                *redis.Client
	ReadGlobalNodeConfig func(context.Context) (config.NodeConfig, error)
}

// Service is the admin server-management entry point used by the network
// facade.
type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) CreateServer(ctx context.Context, req *dto.CreateServerRequest) (*dto.CreateServerResponse, error) {
	return newCreateServerLogic(ctx, s.deps).CreateServer(req)
}

func (s *Service) UpdateServer(ctx context.Context, req *dto.UpdateServerRequest) error {
	return newUpdateServerLogic(ctx, s.deps).UpdateServer(req)
}

func (s *Service) DeleteServer(ctx context.Context, req *dto.DeleteServerRequest) error {
	return newDeleteServerLogic(ctx, s.deps).DeleteServer(req)
}

func (s *Service) FilterServerList(ctx context.Context, req *dto.FilterServerListRequest) (*dto.FilterServerListResponse, error) {
	return newFilterServerListLogic(ctx, s.deps).FilterServerList(req)
}

func (s *Service) ResetSortWithServer(ctx context.Context, req *dto.ResetSortRequest) error {
	return newResetSortWithServerLogic(ctx, s.deps).ResetSortWithServer(req)
}

func (s *Service) GetServerProtocols(ctx context.Context, req *dto.GetServerProtocolsRequest) (*dto.GetServerProtocolsResponse, error) {
	return newGetServerProtocolsLogic(ctx, s.deps).GetServerProtocols(req)
}

func (s *Service) GetServerNodeConfig(ctx context.Context, req *dto.GetServerNodeConfigRequest) (*dto.GetServerNodeConfigResponse, error) {
	return newGetServerNodeConfigLogic(ctx, s.deps).GetServerNodeConfig(req)
}

func (s *Service) UpdateServerNodeConfig(ctx context.Context, req *dto.UpdateServerNodeConfigRequest) error {
	return newUpdateServerNodeConfigLogic(ctx, s.deps).UpdateServerNodeConfig(req)
}

func (s *Service) CreateNode(ctx context.Context, req *dto.CreateNodeRequest) error {
	return newCreateNodeLogic(ctx, s.deps).CreateNode(req)
}

func (s *Service) UpdateNode(ctx context.Context, req *dto.UpdateNodeRequest) error {
	return newUpdateNodeLogic(ctx, s.deps).UpdateNode(req)
}

func (s *Service) DeleteNode(ctx context.Context, req *dto.DeleteNodeRequest) error {
	return newDeleteNodeLogic(ctx, s.deps).DeleteNode(req)
}

func (s *Service) FilterNodeList(ctx context.Context, req *dto.FilterNodeListRequest) (*dto.FilterNodeListResponse, error) {
	return newFilterNodeListLogic(ctx, s.deps).FilterNodeList(req)
}

func (s *Service) ToggleNodeStatus(ctx context.Context, req *dto.ToggleNodeStatusRequest) error {
	return newToggleNodeStatusLogic(ctx, s.deps).ToggleNodeStatus(req)
}

func (s *Service) ResetSortWithNode(ctx context.Context, req *dto.ResetSortRequest) error {
	return newResetSortWithNodeLogic(ctx, s.deps).ResetSortWithNode(req)
}

func (s *Service) QueryNodeTag(ctx context.Context) (*dto.QueryNodeTagResponse, error) {
	return newQueryNodeTagLogic(ctx, s.deps).QueryNodeTag()
}

// Store is the persistence capability required by this package. It excludes
// unrelated repositories and application-wide transactions.
type Store interface {
	repository.NetworkTransactor
	Node() repository.NodeRepo
	UserSubscription() repository.UserSubscriptionRepo
}
