package adminserver

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
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
	return &UpdateServerNodeConfigLogic{Logger: logger.WithContext(ctx), ctx: ctx, deps: deps}
}

func (l *UpdateServerNodeConfigLogic) UpdateServerNodeConfig(req *dto.UpdateServerNodeConfigRequest) error {
	if req == nil || req.ServerID <= 0 {
		return xerr.NewErrCodeMsg(xerr.InvalidParams, "server_id is required")
	}
	if req.RequestFields == nil {
		return xerr.NewErrCodeMsg(xerr.InvalidParams, "explicit request field presence is required")
	}
	// DNS and the IP preference live inside the routing package; partial
	// legacy overrides are rejected, never read, cleared or converted.
	for _, field := range []string{"dns", "ip_strategy", "inherit_dns", "inherit_ip_strategy", "block", "block_ips", "block_protocols", "outbound", "inherit_block", "inherit_outbound"} {
		if _, ok := req.RequestFields[field]; ok {
			return xerr.NewErrCode(xerr.NetworkLegacyRoutingRemoved)
		}
	}
	var validationErr, queryErr, businessErr error
	err := l.deps.Store.InNetworkTx(l.ctx, func(store repository.NetworkStore) error {
		nodes := store.Node()
		if _, err := nodes.FindOneServerForUpdate(l.ctx, req.ServerID); err != nil {
			queryErr = err
			return err
		}
		old, err := nodes.FindServerRoutingOverride(l.ctx, req.ServerID)
		if err != nil {
			queryErr = err
			return err
		}
		// The row's legacy DNS/IP columns are rollback material: kept as is.
		data := &node.ServerConfigOverride{ServerId: req.ServerID}
		if old != nil {
			data.IPStrategy, data.DNS = old.IPStrategy, old.DNS
		}
		currentRevision := int64(0)
		var oldRouting *string
		if old != nil {
			currentRevision = old.RoutingRevision
			oldRouting = old.Routing
		}
		if currentRevision < 0 {
			queryErr = fmt.Errorf("invalid server routing revision")
			return queryErr
		}
		hasRouting, hasInherit := req.Routing != nil, false
		if req.RequestFields != nil {
			_, hasRouting = req.RequestFields["routing"]
			_, hasInherit = req.RequestFields["inherit_routing"]
		}
		if hasRouting && req.Routing == nil {
			validationErr = fmt.Errorf("routing must not be null")
			return validationErr
		}
		if hasRouting && hasInherit && req.InheritRouting {
			validationErr = fmt.Errorf("cannot save and inherit routing together")
			return validationErr
		}
		// Persist desired state independently of live node reports; only the
		// node can report a candidate as pending or applied.
		transition := hasRouting || (hasInherit && req.InheritRouting)
		if transition {
			if req.RequestFields != nil {
				if _, ok := req.RequestFields["routing_revision"]; !ok {
					validationErr = fmt.Errorf("routing_revision is required")
					return validationErr
				}
			}
			if req.RoutingRevision != currentRevision {
				businessErr = xerr.NewErrCode(xerr.NetworkRoutingRevisionConflict)
				return businessErr
			}
			if currentRevision == math.MaxInt64 {
				validationErr = fmt.Errorf("routing revision overflow")
				return validationErr
			}
		}
		data.Routing = oldRouting
		data.RoutingRevision = currentRevision
		if hasRouting {
			policy, err := config.NormalizeNodeRouting(req.Routing)
			if err != nil {
				validationErr = err
				return err
			}
			var previous *config.NodeRouting
			if oldRouting != nil {
				if previous, err = config.ParseNodeRouting([]byte(*oldRouting)); err != nil {
					queryErr = err
					return err
				}
			}
			if missing, err := repository.MissingUserIDs(l.ctx, l.deps.Store.User(), config.NewAccountIDs(policy, previous)); err != nil {
				queryErr = err
				return err
			} else if len(missing) > 0 {
				validationErr = fmt.Errorf("unknown accounts referenced: %v", missing)
				return validationErr
			}
			body, err := json.Marshal(policy)
			if err != nil {
				return err
			}
			raw := string(body)
			data.Routing = &raw
			// Identical canonical content is a successful no-op: the local
			// revision and the nodes' candidate stay unchanged.
			if oldRouting != nil && *oldRouting == raw {
				transition = false
			}
		} else if hasInherit && req.InheritRouting {
			data.Routing = nil
		} else if hasInherit && !req.InheritRouting && oldRouting == nil {
			validationErr = fmt.Errorf("local routing requires a complete policy")
			return validationErr
		}
		if transition {
			data.RoutingRevision++
		}
		if data.Routing == nil && data.RoutingRevision == 0 && data.DNS == nil && data.IPStrategy == nil && old == nil {
			return nil
		}
		return nodes.SaveServerRoutingOverride(l.ctx, data)
	})
	if businessErr != nil {
		return businessErr
	}
	if validationErr != nil {
		return errors.Wrapf(xerr.NewErrCodeMsg(xerr.InvalidParams, validationErr.Error()), "invalid server node config: %v", validationErr)
	}
	if queryErr != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "query server node config: %v", queryErr)
	}
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update server node config: %v", err)
	}
	// Routing/DNS/IP overrides are always read from the database. The
	// server-user cache has no dependency on these fields; invalidating that
	// authorization-sensitive cache here creates an unrelated post-commit
	// failure/retry window without making this write safer.
	return nil
}
