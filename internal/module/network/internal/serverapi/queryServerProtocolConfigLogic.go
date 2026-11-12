package serverapi

import (
	"context"
	"time"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/internal/identity"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

type QueryServerProtocolConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewQueryServerProtocolConfigLogic Get Server Protocol Config
func newQueryServerProtocolConfigLogic(ctx context.Context, deps Deps) *QueryServerProtocolConfigLogic {
	return &QueryServerProtocolConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

// QueryServerProtocolConfig delivers one complete candidate: every enabled
// protocol, the effective routing package and the identity snapshot, read
// consistently. A protocol filter is not supported: the snapshot ID covers
// the whole candidate, so the administrator can recompute it without hidden
// request state.
func (l *QueryServerProtocolConfigLogic) QueryServerProtocolConfig(req *dto.QueryServerConfigRequest) (*dto.QueryServerConfigResponse, error) {
	if len(req.Protocols) > 0 {
		return nil, xerr.NewErrCodeMsg(xerr.InvalidParams, "protocol filtering is not supported; request all protocols")
	}
	candidate, err := identity.BuildCandidate(l.ctx, l.deps.Store, l.deps.ReadGlobalNodeConfig, req.ServerID, time.Now(), true)
	if err != nil {
		l.Errorf("[QueryServerProtocolConfig] build candidate: %s", err.Error())
		return nil, err
	}
	return candidate.Response, nil
}
