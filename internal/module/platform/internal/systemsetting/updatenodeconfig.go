package systemsetting

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	appconfig "github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateNodeConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newUpdateNodeConfigLogic(ctx context.Context, deps Deps) *UpdateNodeConfigLogic {
	return &UpdateNodeConfigLogic{Logger: logger.WithContext(ctx), ctx: ctx, deps: deps}
}

// UpdateNodeConfig updates the platform-owned package in its own transaction.
// Dormant legacy values are never read or written by this runtime.
func (l *UpdateNodeConfigLogic) UpdateNodeConfig(req *dto.NodeConfig) error {
	if req == nil {
		return xerr.NewErrCodeMsg(xerr.InvalidParams, "nil request")
	}
	// DNS and the IP preference live inside routing format 3; the separate
	// legacy fields are rejected, never read, cleared or converted.
	for _, field := range []string{"dns", "ip_strategy", "block", "block_ips", "block_protocols", "outbound", "routing_action"} {
		if _, ok := req.RequestFields[field]; ok {
			return xerr.NewErrCode(xerr.PlatformLegacyRoutingRemoved)
		}
	}
	var businessErr, validationErr, readErr error
	err := l.deps.Store.InPlatformTx(l.ctx, func(store repository.PlatformStore) error {
		systems := store.System()
		entries, err := systems.GetNodeConfigForUpdate(l.ctx)
		if err != nil {
			readErr = err
			return err
		}
		current, err := nodeConfigFromSystemEntries(entries)
		if err != nil {
			readErr = err
			return err
		}
		if current.Routing == nil || !cutoverComplete(entries) {
			readErr = fmt.Errorf("routing format 3 migration is incomplete")
			return readErr
		}
		hasRouting := req.Routing != nil
		if req.RequestFields != nil {
			_, hasRouting = req.RequestFields["routing"]
		}
		if hasRouting && req.Routing == nil {
			validationErr = fmt.Errorf("routing must not be null")
			return validationErr
		}
		if hasRouting {
			if req.RequestFields != nil {
				if _, ok := req.RequestFields["routing_revision"]; !ok {
					validationErr = fmt.Errorf("routing_revision is required")
					return validationErr
				}
			}
			if req.RoutingRevision != current.RoutingRevision {
				businessErr = xerr.NewErrCode(xerr.PlatformRoutingRevisionConflict)
				return businessErr
			}
			policy, err := appconfig.NormalizeNodeRouting(req.Routing)
			if err != nil {
				validationErr = err
				return err
			}
			if added := appconfig.NewAccountIDs(policy, current.Routing); len(added) > 0 {
				if l.deps.MissingAccounts == nil {
					readErr = fmt.Errorf("account lookup unavailable")
					return readErr
				}
				missing, err := l.deps.MissingAccounts(l.ctx, added)
				if err != nil {
					readErr = err
					return err
				}
				if len(missing) > 0 {
					validationErr = fmt.Errorf("unknown accounts referenced: %v", missing)
					return validationErr
				}
			}
			if current.RoutingRevision == math.MaxInt64 {
				validationErr = fmt.Errorf("routing revision overflow")
				return validationErr
			}
			body, err := json.Marshal(policy)
			if err != nil {
				return err
			}
			currentBody, err := json.Marshal(current.Routing)
			if err != nil {
				return err
			}
			// Saving identical canonical content is a successful no-op: the
			// revision (and so every node's candidate) stays unchanged.
			if string(body) != string(currentBody) {
				if err := systems.UpdateValueByCategoryKey(l.ctx, "server", "Routing", string(body), "string"); err != nil {
					return err
				}
				if err := systems.UpdateValueByCategoryKey(l.ctx, "server", "RoutingRevision", strconv.FormatInt(current.RoutingRevision+1, 10), "int64"); err != nil {
					return err
				}
			}
		}
		fields, err := activeNodeConfigFields(req)
		if err != nil {
			validationErr = err
			return err
		}
		return updateConfigFieldsWithStore(l.ctx, systems, "server", fields)
	})
	if businessErr != nil {
		return businessErr
	}
	if validationErr != nil {
		return errors.Wrapf(xerr.NewErrCodeMsg(xerr.InvalidParams, validationErr.Error()), "invalid node config: %v", validationErr)
	}
	if readErr != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "read node config: %v", readErr)
	}
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update node config: %v", err)
	}
	l.deps.reinit("node")
	return nil
}
func activeNodeConfigFields(req *dto.NodeConfig) ([]configFieldValue, error) {
	for _, field := range []struct {
		name  string
		value int64
	}{{"node_pull_interval", req.NodePullInterval}, {"node_push_interval", req.NodePushInterval}} {
		if req.RequestFields != nil {
			if _, present := req.RequestFields[field.name]; !present {
				continue
			}
		}
		if err := appconfig.ValidateNodeInterval(field.name, field.value); err != nil {
			return nil, err
		}
	}
	all := convertedConfigFields(*req)
	fields := make([]configFieldValue, 0, len(all))
	names := map[string]string{"NodeSecret": "node_secret", "NodePullInterval": "node_pull_interval", "NodePushInterval": "node_push_interval", "TrafficReportThreshold": "traffic_report_threshold"}
	for _, field := range all {
		jsonName, ok := names[field.key]
		if !ok {
			continue
		}
		if req.RequestFields != nil {
			if _, provided := req.RequestFields[jsonName]; !provided {
				continue
			}
		}
		fields = append(fields, field)
	}
	return fields, nil
}
