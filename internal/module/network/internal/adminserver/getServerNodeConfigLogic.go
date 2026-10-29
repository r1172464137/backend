package adminserver

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/internal/nodeconfig"
	"github.com/perfect-panel/server/internal/module/network/internal/routingstatus"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetServerNodeConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newGetServerNodeConfigLogic(ctx context.Context, deps Deps) *GetServerNodeConfigLogic {
	return &GetServerNodeConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetServerNodeConfigLogic) GetServerNodeConfig(req *dto.GetServerNodeConfigRequest) (*dto.GetServerNodeConfigResponse, error) {
	nodeStore := l.deps.Store.Node()
	if _, err := nodeStore.FindOneServer(l.ctx, req.ServerID); err != nil {
		l.Errorf("[GetServerNodeConfig] FindOneServer Error: %v", err.Error())
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find server error: %v", err)
	}

	globalSnapshot, override, err := nodeconfig.ResolveSnapshot(l.ctx, l.deps.ReadGlobalNodeConfig, nodeStore, req.ServerID)
	if err != nil {
		l.Errorf("[GetServerNodeConfig] authoritative snapshot Error: %v", err.Error())
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find server node config error: %v", err)
	}

	global := nodeconfig.GlobalValues(globalSnapshot)
	globalPolicy, err := nodeconfig.DeliveryPolicy(globalSnapshot, nil, req.ServerID)
	if err != nil {
		return nil, err
	}
	global.Routing, global.RoutingMode = globalPolicy, "global-v1"
	effective := nodeconfig.CloneValues(global)
	if err := nodeconfig.ApplyOverride(&effective, override); err != nil {
		l.Errorf("[GetServerNodeConfig] ApplyOverride Error: %v", err.Error())
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "apply server node config override error: %v", err)
	}
	overrideResp, err := nodeconfig.OverrideResponse(override)
	if err != nil {
		l.Errorf("[GetServerNodeConfig] OverrideResponse Error: %v", err.Error())
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "parse server node config override error: %v", err)
	}

	effectivePolicy, err := nodeconfig.DeliveryPolicy(globalSnapshot, override, req.ServerID)
	if err != nil {
		return nil, err
	}
	effective.Routing, effective.RoutingRevision = effectivePolicy, effectivePolicy.SourceRevision
	effective.RoutingMode = "global-v1"
	if !overrideResp.InheritRouting {
		effective.RoutingMode = "local-v1"
	}
	status, err := routingstatus.Read(l.ctx, l.deps.Redis, req.ServerID)
	if err != nil {
		status = dto.RoutingStatusView{Known: false}
	}
	return &dto.GetServerNodeConfigResponse{
		Global:        global,
		Override:      overrideResp,
		Effective:     effective,
		RoutingStatus: status,
	}, nil
}
