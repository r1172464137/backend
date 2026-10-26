package adminserver

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/internal/nodeconfig"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateServerNodeConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newUpdateServerNodeConfigLogic(ctx context.Context, deps Deps) *UpdateServerNodeConfigLogic {
	return &UpdateServerNodeConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateServerNodeConfigLogic) UpdateServerNodeConfig(req *dto.UpdateServerNodeConfigRequest) error {
	var validationErr, queryErr, parseErr error
	err := l.deps.Store.InNetworkTx(l.ctx, func(store repository.NetworkStore) error {
		nodeStore := store.Node()
		// The parent server always exists, so locking it serializes both updates
		// to an existing override and concurrent creation of the first override.
		if _, err := nodeStore.FindOneServerForUpdate(l.ctx, req.ServerID); err != nil {
			queryErr = err
			return errors.Wrap(err, "find server for update")
		}
		existingModel, err := nodeStore.FindServerConfigOverride(l.ctx, req.ServerID)
		if err != nil {
			queryErr = err
			return errors.Wrap(err, "find server node config override")
		}
		existing, err := nodeconfig.OverrideResponse(existingModel)
		if err != nil {
			parseErr = err
			return errors.Wrap(err, "parse current server node config override")
		}
		global := nodeconfig.GlobalValues(l.deps.Config().Node)
		if err := nodeconfig.MergeRouteRuleOmissions(req, existing, global.Outbound); err != nil {
			validationErr = err
			return err
		}

		data, allInherited, err := nodeconfig.OverrideModel(req.ServerID, req.ServerNodeConfigOverride)
		if err != nil {
			validationErr = err
			return err
		}
		if allInherited {
			return nodeStore.DeleteServerConfigOverride(l.ctx, req.ServerID)
		}
		return nodeStore.SaveServerConfigOverride(l.ctx, data)
	})
	if validationErr != nil {
		l.Errorf("[UpdateServerNodeConfig] invalid server node config: %v", validationErr)
		return errors.Wrapf(xerr.NewErrCodeMsg(xerr.InvalidParams, validationErr.Error()), "server node config is invalid: %v", validationErr)
	}
	if queryErr != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "query server node config for update: %v", queryErr)
	}
	if parseErr != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "parse current server node config override: %v", parseErr)
	}
	if err != nil {
		l.Errorf("[UpdateServerNodeConfig] transactional update error: %v", err)
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update server node config error: %v", err)
	}
	return l.deps.Store.Node().ClearServerCache(l.ctx, req.ServerID)
}
