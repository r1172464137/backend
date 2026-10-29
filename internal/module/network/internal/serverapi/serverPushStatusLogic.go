package serverapi

import (
	"context"
	"errors"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/network/internal/routingstatus"
	"github.com/perfect-panel/server/internal/module/network/internal/trafficagg"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
)

type ServerPushStatusLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewServerPushStatusLogic Push server status
func newServerPushStatusLogic(ctx context.Context, deps Deps) *ServerPushStatusLogic {
	return &ServerPushStatusLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *ServerPushStatusLogic) ServerPushStatus(req *dto.ServerPushStatusRequest) error {
	// Find server info
	serverInfo, err := l.deps.Store.Node().FindOneServer(l.ctx, req.ServerId)
	if err != nil || serverInfo.Id <= 0 {
		l.Errorw("[PushOnlineUsers] FindOne error", logger.Field("error", err))
		return errors.New("server not found")
	}
	if l.deps.Redis != nil {
		if err := routingstatus.Record(l.ctx, l.deps.Redis, req.ServerId, req); err != nil {
			return err
		}
	}
	err = l.deps.Store.Node().UpdateStatusCache(l.ctx, req.ServerId, &node.Status{
		Cpu:       req.Cpu,
		Mem:       req.Mem,
		Disk:      req.Disk,
		UpdatedAt: req.UpdatedAt,
	})
	if err != nil {
		l.Errorw("[ServerPushStatus] UpdateNodeStatus error", logger.Field("error", err))
		return errors.New("update node status failed")
	}
	now := timeutil.Now()
	if err := trafficagg.New(trafficagg.Deps{Store: l.deps.Store, Redis: l.deps.Redis}).RecordServerReport(l.ctx, req.ServerId, now); err != nil {
		l.Errorw("[ServerPushStatus] RecordServerReport error", logger.Field("error", err))
		return errors.New("update node report time failed")
	}

	// Certificate metadata is exceptional: most heartbeats have nothing to
	// persist after their status and last-seen values reach Redis.
	certPinChanged := false
	currentProtocols := serverInfo.Protocols
	if req.CertPinSHA256 != "" {
		certPinChanged, err = serverInfo.ApplyReportedCertPin(req.Protocol, req.CertPinSHA256)
		if err != nil {
			l.Errorw("[ServerPushStatus] ApplyReportedCertPin error", logger.Field("error", err))
			certPinChanged = false
		}
	}

	if certPinChanged {
		updated, updateErr := l.deps.Store.Node().UpdateServerProtocolsIfCurrent(l.ctx, serverInfo.Id, currentProtocols, serverInfo.Protocols)
		if updateErr != nil {
			l.Errorw("[ServerPushStatus] UpdateServerProtocols error", logger.Field("error", updateErr))
			return errors.New("update node certificate metadata failed")
		}
		if !updated {
			// An administrator changed the protocol configuration after this
			// heartbeat read it. The next heartbeat will apply the fingerprint to
			// the fresh value instead of overwriting that change.
			return nil
		}
		if err := l.deps.Store.Node().ClearServerCache(l.ctx, req.ServerId); err != nil {
			l.Errorw("[ServerPushStatus] ClearServerCache error", logger.Field("error", err))
		}
	}

	return nil
}
