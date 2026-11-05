package adminserver

import (
	"context"
	"fmt"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/internal/nodeconfig"
	"github.com/perfect-panel/server/internal/module/network/internal/routingstatus"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

func (s *Service) GetServerNodeConfig(ctx context.Context, req *dto.GetServerNodeConfigRequest) (*dto.GetServerNodeConfigResponse, error) {
	nodeStore := s.deps.Store.Node()
	if _, err := nodeStore.FindOneServer(ctx, req.ServerID); err != nil {
		logger.WithContext(ctx).Errorf("[GetServerNodeConfig] FindOneServer Error: %v", err.Error())
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find server error: %v", err)
	}

	globalSnapshot, override, err := nodeconfig.ResolveSnapshot(ctx, s.deps.ReadGlobalNodeConfig, nodeStore, req.ServerID)
	if err != nil {
		logger.WithContext(ctx).Errorf("[GetServerNodeConfig] authoritative snapshot Error: %v", err.Error())
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find server node config error: %v", err)
	}

	global := nodeconfig.GlobalValues(globalSnapshot)
	globalPolicy, err := nodeconfig.DeliveryPolicy(globalSnapshot, nil, req.ServerID)
	if err != nil {
		return nil, err
	}
	global.Routing, global.RoutingMode = globalPolicy, fmt.Sprintf("global-v%d", globalPolicy.Version)
	effective := nodeconfig.CloneValues(global)
	if err := nodeconfig.ApplyOverride(&effective, override); err != nil {
		logger.WithContext(ctx).Errorf("[GetServerNodeConfig] ApplyOverride Error: %v", err.Error())
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "apply server node config override error: %v", err)
	}
	overrideResp, err := nodeconfig.OverrideResponse(override)
	if err != nil {
		logger.WithContext(ctx).Errorf("[GetServerNodeConfig] OverrideResponse Error: %v", err.Error())
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "parse server node config override error: %v", err)
	}

	effectivePolicy, err := nodeconfig.DeliveryPolicy(globalSnapshot, override, req.ServerID)
	if err != nil {
		return nil, err
	}
	effective.Routing, effective.RoutingRevision = effectivePolicy, effectivePolicy.SourceRevision
	effective.RoutingMode = fmt.Sprintf("global-v%d", effectivePolicy.Version)
	if !overrideResp.InheritRouting {
		effective.RoutingMode = fmt.Sprintf("local-v%d", effectivePolicy.Version)
	}
	status, err := routingstatus.Read(ctx, s.deps.Redis, req.ServerID)
	if err != nil {
		status = dto.RoutingStatusView{Known: false}
	}
	var desired *string
	if effectivePolicy.Version == 2 && s.deps.DesiredSnapshotID != nil {
		desired, err = s.deps.DesiredSnapshotID(ctx, req.ServerID, status)
		if err != nil {
			logger.WithContext(ctx).Errorw("cannot calculate desired snapshot", logger.Field("error", err.Error()))
			desired = nil
		}
	}
	return &dto.GetServerNodeConfigResponse{
		Global:            global,
		Override:          overrideResp,
		Effective:         effective,
		RoutingStatus:     status,
		DesiredSnapshotID: desired,
	}, nil
}
