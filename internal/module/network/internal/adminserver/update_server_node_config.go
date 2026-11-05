package adminserver

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/network/internal/nodeconfig"
	"github.com/perfect-panel/server/internal/module/platform"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

// nodeConfigUpdate keeps both the state and error classification of one
// locked server update. Every read and write below runs inside the same tx.
type nodeConfigUpdate struct {
	service                              *Service
	ctx                                  context.Context
	req                                  *dto.UpdateServerNodeConfigRequest
	store                                repository.NetworkStore
	old                                  *node.ServerConfigOverride
	data                                 *node.ServerConfigOverride
	revision                             int64
	oldRouting                           *string
	hasRouting, hasInherit, transition   bool
	validationErr, queryErr, businessErr error
}

func (s *Service) UpdateServerNodeConfig(ctx context.Context, req *dto.UpdateServerNodeConfigRequest) error {
	if req == nil || req.ServerID <= 0 {
		return xerr.NewErrCodeMsg(xerr.InvalidParams, "server_id is required")
	}
	if req.RequestFields == nil {
		return xerr.NewErrCodeMsg(xerr.InvalidParams, "explicit request field presence is required")
	}
	for _, field := range []string{"block", "block_ips", "block_protocols", "outbound", "inherit_block", "inherit_outbound"} {
		if _, ok := req.RequestFields[field]; ok {
			return xerr.NewErrCode(xerr.NetworkLegacyRoutingRemoved)
		}
	}
	u := &nodeConfigUpdate{service: s, ctx: ctx, req: req}
	err := s.deps.Store.InNetworkTx(ctx, func(store repository.NetworkStore) error {
		u.store = store
		if err := u.prepare(); err != nil {
			return err
		}
		if err := u.checkDowngrade(); err != nil {
			return err
		}
		if err := u.applyRouting(); err != nil {
			return err
		}
		if u.transition {
			u.data.RoutingRevision++
		}
		if u.data.Routing == nil && u.data.RoutingRevision == 0 && u.data.DNS == nil && u.data.IPStrategy == nil && u.old == nil {
			return nil
		}
		return store.Node().SaveServerRoutingOverride(ctx, u.data)
	})
	if u.businessErr != nil {
		return u.businessErr
	}
	if u.validationErr != nil {
		return errors.Wrapf(xerr.NewErrCodeMsg(xerr.InvalidParams, u.validationErr.Error()), "invalid server node config: %v", u.validationErr)
	}
	if u.queryErr != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "query server node config: %v", u.queryErr)
	}
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update server node config: %v", err)
	}
	// Routing/DNS/IP overrides are read from DB; do not invalidate the
	// unrelated authorization-sensitive server-user cache after commit.
	return nil
}

func (u *nodeConfigUpdate) readGlobal() (config.NodeConfig, error) {
	if view, ok := u.store.(interface{ System() repository.SystemRepo }); ok {
		return platform.ReadNodeConfigSnapshotFromRepo(u.ctx, view.System())
	}
	if u.service.deps.ReadGlobalNodeConfig == nil {
		return config.NodeConfig{}, fmt.Errorf("global routing reader unavailable")
	}
	return u.service.deps.ReadGlobalNodeConfig(u.ctx)
}

func (u *nodeConfigUpdate) prepare() error {
	nodes := u.store.Node()
	if _, err := nodes.FindOneServerForUpdate(u.ctx, u.req.ServerID); err != nil {
		u.queryErr = err
		return err
	}
	old, err := nodes.FindServerRoutingOverride(u.ctx, u.req.ServerID)
	if err != nil {
		u.queryErr = err
		return err
	}
	u.old = old
	current, err := nodeconfig.OverrideResponse(old)
	if err != nil {
		u.queryErr = err
		return err
	}
	mergeOverrideFields(u.req, current)
	u.data, err = nodeconfig.OverrideModel(u.req.ServerID, u.req.ServerNodeConfigOverride)
	if err != nil {
		u.validationErr = err
		return err
	}
	if old != nil {
		u.revision, u.oldRouting = old.RoutingRevision, old.Routing
	}
	if u.revision < 0 {
		u.queryErr = fmt.Errorf("invalid server routing revision")
		return u.queryErr
	}
	_, u.hasRouting = u.req.RequestFields["routing"]
	_, u.hasInherit = u.req.RequestFields["inherit_routing"]
	if u.hasRouting && u.req.Routing == nil {
		u.validationErr = fmt.Errorf("routing must not be null")
		return u.validationErr
	}
	if u.hasRouting && u.hasInherit && u.req.InheritRouting {
		u.validationErr = fmt.Errorf("cannot save and inherit routing together")
		return u.validationErr
	}
	u.transition = u.hasRouting || (u.hasInherit && u.req.InheritRouting)
	if err := u.checkRevision(); err != nil {
		return err
	}
	u.data.Routing, u.data.RoutingRevision = u.oldRouting, u.revision
	return nil
}

func mergeOverrideFields(req *dto.UpdateServerNodeConfigRequest, current dto.ServerNodeConfigOverride) {
	if _, ok := req.RequestFields["inherit_ip_strategy"]; !ok {
		req.InheritIPStrategy = current.InheritIPStrategy
	}
	if _, ok := req.RequestFields["ip_strategy"]; !ok {
		req.IPStrategy = current.IPStrategy
	}
	if _, ok := req.RequestFields["inherit_dns"]; !ok {
		req.InheritDNS = current.InheritDNS
	}
	if _, ok := req.RequestFields["dns"]; !ok {
		req.DNS = current.DNS
	}
}

func (u *nodeConfigUpdate) checkRevision() error {
	if !u.transition {
		return nil
	}
	if _, ok := u.req.RequestFields["routing_revision"]; !ok {
		u.validationErr = fmt.Errorf("routing_revision is required")
		return u.validationErr
	}
	if u.req.RoutingRevision != u.revision {
		u.businessErr = xerr.NewErrCode(xerr.NetworkRoutingRevisionConflict)
		return u.businessErr
	}
	if u.revision == math.MaxInt64 {
		u.validationErr = fmt.Errorf("routing revision overflow")
		return u.validationErr
	}
	return nil
}

func (u *nodeConfigUpdate) checkDowngrade() error {
	if !u.transition || u.oldRouting == nil || u.req.RoutingDowngrade {
		return nil
	}
	old, err := config.ParseNodeRouting([]byte(*u.oldRouting))
	if err != nil {
		u.queryErr = err
		return err
	}
	if !config.HasRoutingV2Capabilities(old) {
		return nil
	}
	downgrade := u.hasRouting && u.req.Routing.Version == config.RoutingVersion1
	if u.hasInherit && u.req.InheritRouting {
		global, err := u.readGlobal()
		if err != nil {
			u.queryErr = err
			return err
		}
		downgrade = global.Routing == nil || global.Routing.Version == config.RoutingVersion1
	}
	if downgrade {
		u.businessErr = xerr.NewErrCode(xerr.NetworkRoutingDowngradeRequired)
		return u.businessErr
	}
	return nil
}

func (u *nodeConfigUpdate) applyRouting() error {
	if !u.hasRouting {
		if u.hasInherit && u.req.InheritRouting {
			u.data.Routing = nil
		} else if u.hasInherit && u.oldRouting == nil {
			u.validationErr = fmt.Errorf("local routing requires a complete policy")
			return u.validationErr
		}
		return nil
	}
	policy, err := config.NormalizeNodeRouting(u.req.Routing)
	if err != nil {
		u.validationErr = err
		return err
	}
	previous, err := u.previousPolicy(policy)
	if err != nil {
		u.queryErr = err
		return err
	}
	accounts := u.service.deps.RoutingAccounts
	if provider, ok := u.store.(interface {
		RoutingAccountLookup() config.RoutingAccountLookup
	}); ok {
		accounts = provider.RoutingAccountLookup()
	}
	if err := config.ValidateNewRoutingAccounts(u.ctx, accounts, previous, policy); err != nil {
		var missing *config.RoutingAccountNotFoundError
		if errors.As(err, &missing) {
			u.validationErr = err
		} else {
			u.queryErr = err
		}
		return err
	}
	body, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	raw := string(body)
	u.data.Routing = &raw
	return nil
}

func (u *nodeConfigUpdate) previousPolicy(policy *config.NodeRouting) (*config.NodeRouting, error) {
	if u.oldRouting != nil {
		return config.ParseNodeRouting([]byte(*u.oldRouting))
	}
	if config.HasRoutingAccounts(policy) && u.service.deps.ReadGlobalNodeConfig != nil {
		global, err := u.readGlobal()
		if err != nil {
			return nil, err
		}
		return global.Routing, nil
	}
	return nil, nil
}
