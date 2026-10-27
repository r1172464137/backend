package adminserver

import (
	"context"
	"fmt"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/internal/nodeconfig"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

// UpdateServerNodeConfig locks the parent server so read/merge/write also
// serializes creation of its first override, then clears node-facing caches
// after commit.
func (s *Service) UpdateServerNodeConfig(ctx context.Context, req *dto.UpdateServerNodeConfigRequest) error {
	log := logger.WithContext(ctx)
	var validationErr, queryErr, parseErr error
	err := s.deps.Store.InNetworkTx(ctx, func(store repository.NetworkStore) error {
		nodeStore := store.Node()
		if _, err := nodeStore.FindOneServerForUpdate(ctx, req.ServerID); err != nil {
			queryErr = err
			return fmt.Errorf("find server for update: %w", err)
		}
		existingModel, err := nodeStore.FindServerConfigOverride(ctx, req.ServerID)
		if err != nil {
			queryErr = err
			return fmt.Errorf("find server node config override: %w", err)
		}
		existing, err := nodeconfig.OverrideResponse(existingModel)
		if err != nil {
			parseErr = err
			return fmt.Errorf("parse current server node config override: %w", err)
		}
		global := nodeconfig.GlobalValues(s.deps.Config().Node)
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
			return nodeStore.DeleteServerConfigOverride(ctx, req.ServerID)
		}
		return nodeStore.SaveServerConfigOverride(ctx, data)
	})
	if validationErr != nil {
		log.Errorf("[UpdateServerNodeConfig] invalid server node config: %v", validationErr)
		return fmt.Errorf("server node config is invalid: %w: %w", validationErr, xerr.NewErrCodeMsg(xerr.InvalidParams, validationErr.Error()))
	}
	if queryErr != nil {
		return xerr.Wrapf(queryErr, xerr.DatabaseQueryError, "query server node config for update: %v", queryErr)
	}
	if parseErr != nil {
		return xerr.Wrapf(parseErr, xerr.ERROR, "parse current server node config override: %v", parseErr)
	}
	if err != nil {
		log.Errorf("[UpdateServerNodeConfig] transactional update error: %v", err)
		return xerr.Wrapf(err, xerr.DatabaseUpdateError, "update server node config: %v", err)
	}
	return s.deps.Store.Node().ClearServerCache(ctx, req.ServerID)
}
