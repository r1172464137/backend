package adminserver

import (
	"context"
	"time"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/internal/identity"
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
	globalPolicy, err := nodeconfig.DeliveryPolicy(globalSnapshot, nil, req.ServerID)
	if err != nil {
		return nil, err
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
	status, err := routingstatus.Read(l.ctx, l.deps.Redis, req.ServerID)
	if err != nil {
		status = dto.RoutingStatusView{Known: false}
	}
	// Desired is computed read-only (no pending activation). If a real pull
	// would activate pending subscriptions, the identity cannot be predicted.
	if candidate, err := identity.BuildCandidate(l.ctx, l.deps.Store, l.deps.ReadGlobalNodeConfig, req.ServerID, time.Now(), false); err == nil && !candidate.Result.PendingActivation {
		id := candidate.Result.Snapshot.SnapshotID
		status.DesiredSnapshotID = &id
		status.Synced = appliedMatches(status, candidate.Response.Routing, id)
	} else if err != nil {
		l.Errorf("[GetServerNodeConfig] desired candidate: %v", err)
	}
	return &dto.GetServerNodeConfigResponse{
		Global:        dto.ServerNodeConfigValues{Routing: globalPolicy, RoutingRevision: globalSnapshot.RoutingRevision, RoutingMode: nodeconfig.ModeInherited},
		Override:      overrideResp,
		Effective:     dto.ServerNodeConfigValues{Routing: effectivePolicy, RoutingRevision: effectivePolicy.SourceRevision, RoutingMode: effectivePolicy.Mode},
		RoutingStatus: status,
	}, nil
}

// appliedMatches is true only for a verified reporter whose applied identity
// equals the desired one in every field, including the snapshot ID; equal
// revisions alone do not prove the user identities are in sync.
func appliedMatches(status dto.RoutingStatusView, desired *dto.NodeRoutingPolicy, desiredID string) bool {
	if !status.Known || !status.ReporterVerified || status.Report == nil || status.Report.Applied == nil || desired == nil {
		return false
	}
	a := status.Report.Applied
	return a.Mode == desired.Mode && a.Source == desired.Source && a.SourceServerID == desired.SourceServerID &&
		a.SourceRevision == desired.SourceRevision && a.LocalTransitionRevision == desired.LocalTransitionRevision &&
		a.SnapshotID == desiredID
}
