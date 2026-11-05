package systemsetting

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	appconfig "github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

// UpdateNodeConfig updates the platform-owned package in its own transaction.
// Dormant legacy values are never read or written by this runtime.
func (s *Service) UpdateNodeConfig(ctx context.Context, req *dto.NodeConfig) error {
	if req == nil {
		return xerr.NewErrCodeMsg(xerr.InvalidParams, "nil request")
	}
	for _, field := range []string{"block", "block_ips", "block_protocols", "outbound", "routing_action"} {
		if _, ok := req.RequestFields[field]; ok {
			return xerr.NewErrCode(xerr.PlatformLegacyRoutingRemoved)
		}
	}
	update := nodeConfigUpdate{service: s, ctx: ctx, req: req}
	err := s.deps.Store.InSettingsTx(ctx, update.apply)
	if update.businessErr != nil {
		return update.businessErr
	}
	if update.validationErr != nil {
		return errors.Wrapf(xerr.NewErrCodeMsg(xerr.InvalidParams, update.validationErr.Error()), "invalid node config: %v", update.validationErr)
	}
	if update.readErr != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "read node config: %v", update.readErr)
	}
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update node config: %v", err)
	}
	return s.deps.reinit("node")
}

// nodeConfigUpdate keeps error classification across the transaction boundary.
type nodeConfigUpdate struct {
	service                             *Service
	ctx                                 context.Context
	req                                 *dto.NodeConfig
	businessErr, validationErr, readErr error
}

func (u *nodeConfigUpdate) apply(systems SettingsStore) error {
	entries, err := systems.GetNodeConfigForUpdate(u.ctx)
	if err != nil {
		u.readErr = err
		return err
	}
	current, err := u.readCurrent(entries)
	if err != nil {
		return err
	}
	routingFields, err := u.routingFields(systems, current)
	if err != nil {
		return err
	}
	fields, err := activeNodeConfigFields(u.req)
	if err != nil {
		u.validationErr = err
		return err
	}
	fields = append(fields, routingFields...)
	previous := make([]configFieldValue, 0, len(entries))
	for _, row := range entries {
		previous = append(previous, configFieldValue{key: row.Key, value: row.Value})
	}
	return updateConfigFieldsWithStore(u.ctx, systems, settingsChange{category: "server", next: fields, previous: previous})
}

// An explicitly supplied, valid DNS document may repair corrupt stored DNS.
// Routing and cutover remain strictly parsed from the locked snapshot.
func (u *nodeConfigUpdate) readCurrent(entries []*system.System) (*dto.NodeConfig, error) {
	current, err := nodeConfigFromSystemEntries(entries)
	if err == nil {
		return current, nil
	}
	_, hasDNS := u.req.RequestFields["dns"]
	if u.req.RequestFields == nil {
		hasDNS = true
	}
	if hasDNS {
		if _, validateErr := activeNodeConfigFields(u.req); validateErr != nil {
			u.validationErr = validateErr
			return nil, validateErr
		}
		dns, marshalErr := json.Marshal(u.req.DNS)
		if marshalErr != nil {
			u.validationErr = marshalErr
			return nil, marshalErr
		}
		repaired := append([]*system.System(nil), entries...)
		for i, row := range repaired {
			if row.Key == "DNS" {
				copyRow := *row
				copyRow.Value = string(dns)
				repaired[i] = &copyRow
			}
		}
		current, err = nodeConfigFromSystemEntries(repaired)
	}
	if err != nil {
		u.readErr = err
	}
	return current, err
}

func (u *nodeConfigUpdate) routingFields(systems SettingsStore, current *dto.NodeConfig) ([]configFieldValue, error) {
	hasRouting := u.req.Routing != nil
	if u.req.RequestFields != nil {
		_, hasRouting = u.req.RequestFields["routing"]
	}
	if !hasRouting {
		return nil, nil
	}
	if u.req.Routing == nil {
		u.validationErr = fmt.Errorf("routing must not be null")
		return nil, u.validationErr
	}
	if u.req.RequestFields != nil {
		if _, ok := u.req.RequestFields["routing_revision"]; !ok {
			u.validationErr = fmt.Errorf("routing_revision is required")
			return nil, u.validationErr
		}
	}
	if u.req.RoutingRevision != current.RoutingRevision {
		u.businessErr = xerr.NewErrCode(xerr.PlatformRoutingRevisionConflict)
		return nil, u.businessErr
	}
	if u.req.Routing.Version == appconfig.RoutingVersion1 && appconfig.HasRoutingV2Capabilities(current.Routing) && !u.req.RoutingDowngrade {
		u.businessErr = xerr.NewErrCode(xerr.PlatformRoutingDowngradeRequired)
		return nil, u.businessErr
	}
	policy, err := appconfig.NormalizeNodeRouting(u.req.Routing)
	if err != nil {
		u.validationErr = err
		return nil, err
	}
	accounts := u.service.deps.RoutingUsers
	if provider, ok := systems.(interface {
		RoutingAccountLookup() appconfig.RoutingAccountLookup
	}); ok {
		if txAccounts := provider.RoutingAccountLookup(); txAccounts != nil {
			accounts = txAccounts
		}
	}
	if err := appconfig.ValidateNewRoutingAccounts(u.ctx, accounts, current.Routing, policy); err != nil {
		var missing *appconfig.RoutingAccountNotFoundError
		if errors.As(err, &missing) {
			u.validationErr = err
		} else {
			u.readErr = err
		}
		return nil, err
	}
	if current.RoutingRevision == math.MaxInt64 {
		u.validationErr = fmt.Errorf("routing revision overflow")
		return nil, u.validationErr
	}
	body, err := json.Marshal(policy)
	if err != nil {
		return nil, err
	}
	return []configFieldValue{{key: "Routing", value: string(body), valueType: "string"}, {key: "RoutingRevision", value: strconv.FormatInt(current.RoutingRevision+1, 10), valueType: "int64"}}, nil
}
func activeNodeConfigFields(req *dto.NodeConfig) ([]configFieldValue, error) {
	if req.RequestFields == nil || func() bool { _, ok := req.RequestFields["dns"]; return ok }() {
		values := make([]appconfig.NodeDNS, 0, len(req.DNS))
		for _, d := range req.DNS {
			values = append(values, appconfig.NodeDNS{Proto: d.Proto, Address: d.Address, ServerName: d.ServerName, Domains: d.Domains})
		}
		normal, err := appconfig.NormalizeNodeDNS(values)
		if err != nil {
			return nil, err
		}
		req.DNS = make([]dto.PlatformNodeDNSSnapshot, 0, len(normal))
		for _, d := range normal {
			req.DNS = append(req.DNS, dto.PlatformNodeDNSSnapshot{Proto: d.Proto, Address: d.Address, ServerName: d.ServerName, Domains: d.Domains})
		}
	}
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
	names := map[string]string{"NodeSecret": "node_secret", "NodePullInterval": "node_pull_interval", "NodePushInterval": "node_push_interval", "TrafficReportThreshold": "traffic_report_threshold", "IPStrategy": "ip_strategy", "DNS": "dns"}
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
		if field.key == "DNS" {
			if err := appconfig.ValidatePersistedJSON("dns", req.DNS); err != nil {
				return nil, err
			}
			field.valueType = "string"
		}
		fields = append(fields, field)
	}
	return fields, nil
}
