package systemsetting

import (
	"context"
	"fmt"

	appconfig "github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

// UpdateNodeConfig stores one omission-safe, validated node-settings snapshot
// and its audit row in the same transaction, then reloads the node subsystem.
func (s *Service) UpdateNodeConfig(ctx context.Context, req *dto.NodeConfig) error {
	var validationErr, queryErr, parseErr error
	err := s.deps.Store.InSettingsTx(ctx, func(settings SettingsStore) error {
		rows, err := settings.GetNodeConfigForUpdate(ctx)
		if err != nil {
			queryErr = err
			return fmt.Errorf("get current node config for update: %w", err)
		}
		parsed, err := system.ParseNodeConfig(rows)
		if err != nil {
			parseErr = err
			return fmt.Errorf("parse current node config: %w", err)
		}
		stored := nodeConfigView(parsed)
		if err := keepOutboundSecrets(req.Outbound, stored.Outbound); err != nil {
			validationErr = err
			return err
		}
		if err := prepareNodeConfigUpdate(req, stored); err != nil {
			validationErr = err
			return err
		}
		change := settingsChange{
			category: "server",
			next:     nodeConfigFields(*req),
			previous: nodeConfigFields(*stored),
		}
		return updateConfigFieldsWithStore(ctx, settings, change)
	})
	if validationErr != nil {
		logger.WithContext(ctx).Errorw("[UpdateNodeConfig] invalid node route settings", logger.Field("error", validationErr.Error()))
		return fmt.Errorf("node route rules are invalid: %w: %w", validationErr, xerr.NewErrCodeMsg(xerr.InvalidParams, validationErr.Error()))
	}
	if queryErr != nil {
		return xerr.Wrapf(queryErr, xerr.DatabaseQueryError, "get current node config for update: %v", queryErr)
	}
	if parseErr != nil {
		return xerr.Wrapf(parseErr, xerr.ERROR, "parse current node config: %v", parseErr)
	}
	if err != nil {
		logger.WithContext(ctx).Errorw("[UpdateNodeConfig] transactional update node config error", logger.Field("error", err.Error()))
		return xerr.Wrapf(err, xerr.DatabaseUpdateError, "update server config: %v", err)
	}
	return s.deps.reinit("node")
}

func nodeConfigFields(req dto.NodeConfig) []configFieldValue {
	fields := convertedConfigFields(req)
	for index := range fields {
		switch fields[index].key {
		case "DNS", "Block", "BlockIPs", "BlockProtocols", "Outbound":
			// NodeDBConfig reads these JSON values as strings before decoding
			// them into runtime types, including when atomic upsert creates a row.
			fields[index].valueType = "string"
		}
	}
	return fields
}

func prepareNodeConfigUpdate(req, current *dto.NodeConfig) error {
	if req == nil {
		return fmt.Errorf("node config request is nil")
	}
	if current == nil {
		current = &dto.NodeConfig{}
	}

	names := make([]string, len(req.Outbound))
	protocols := make([]string, len(req.Outbound))
	for index := range req.Outbound {
		names[index] = req.Outbound[index].Name
		protocols[index] = req.Outbound[index].Protocol
	}
	names, err := appconfig.NormalizeOutboundNames("outbound", names)
	if err != nil {
		return err
	}
	protocols, err = appconfig.NormalizeOutboundProtocols("outbound", protocols)
	if err != nil {
		return err
	}
	for index := range req.Outbound {
		req.Outbound[index].Name = names[index]
		req.Outbound[index].Protocol = protocols[index]
	}

	// A nil presence map means an internal caller supplied a complete value.
	// HTTP JSON binding always records a non-nil map, including for {}.
	if !requestFieldProvided(req.RequestFields, "block_ips") {
		req.BlockIPs = cloneStrings(current.BlockIPs)
	}
	if !requestFieldProvided(req.RequestFields, "block_protocols") {
		req.BlockProtocols = cloneStrings(current.BlockProtocols)
	}
	if req.RequestFields != nil {
		if err := mergePlatformOutboundRuleOmissions(req.Outbound, current.Outbound, req.OutboundFieldSets); err != nil {
			return err
		}
	}

	if req.Block, err = appconfig.NormalizeDomainRules("block", req.Block); err != nil {
		return err
	}
	if req.BlockIPs, err = appconfig.NormalizeIPRules("block_ips", req.BlockIPs); err != nil {
		return err
	}
	if req.BlockProtocols, err = appconfig.NormalizeProtocolRules("block_protocols", req.BlockProtocols); err != nil {
		return err
	}
	totalRules, totalRuleBytes := 0, 0
	for index := range req.Outbound {
		field := fmt.Sprintf("outbound[%d]", index+1)
		if req.Outbound[index].Rules, err = appconfig.NormalizeDomainRules(field+".rules", req.Outbound[index].Rules); err != nil {
			return err
		}
		if req.Outbound[index].IPRules, err = appconfig.NormalizeIPRules(field+".ip_rules", req.Outbound[index].IPRules); err != nil {
			return err
		}
		if req.Outbound[index].ProtocolRules, err = appconfig.NormalizeProtocolRules(field+".protocol_rules", req.Outbound[index].ProtocolRules); err != nil {
			return err
		}
		for _, rules := range [][]string{req.Outbound[index].Rules, req.Outbound[index].IPRules, req.Outbound[index].ProtocolRules} {
			totalRules += len(rules)
			for _, rule := range rules {
				totalRuleBytes += len(rule)
			}
		}
	}
	if err := appconfig.ValidateOutboundAggregate("outbound", len(req.Outbound), totalRules, totalRuleBytes); err != nil {
		return err
	}
	for _, persisted := range []struct {
		field string
		value any
	}{
		{"block", req.Block},
		{"block_ips", req.BlockIPs},
		{"block_protocols", req.BlockProtocols},
		{"outbound", req.Outbound},
	} {
		if err := appconfig.ValidatePersistedJSON(persisted.field, persisted.value); err != nil {
			return err
		}
	}
	return nil
}

func mergePlatformOutboundRuleOmissions(next, current []dto.PlatformNodeOutboundSnapshot, fieldSets []map[string]struct{}) error {
	needsMatch := false
	for index := range next {
		provided := fieldSetAt(fieldSets, index)
		if provided == nil {
			continue
		}
		_, hasIP := provided["ip_rules"]
		_, hasProtocol := provided["protocol_rules"]
		if !hasIP || !hasProtocol {
			needsMatch = true
			break
		}
	}
	if !needsMatch {
		return nil
	}

	nextInputs := make([]appconfig.LegacyOutboundMatchInput, len(next))
	for index := range next {
		nextInputs[index] = appconfig.LegacyOutboundMatchInput{Name: next[index].Name, Value: next[index]}
	}
	currentInputs := make([]appconfig.LegacyOutboundMatchInput, len(current))
	for index := range current {
		currentInputs[index] = appconfig.LegacyOutboundMatchInput{Name: current[index].Name, Value: current[index]}
	}
	matches, err := appconfig.MatchLegacyOutbounds("outbound", nextInputs, currentInputs)
	if err != nil {
		return err
	}
	for index, currentIndex := range matches {
		if currentIndex < 0 {
			continue
		}
		provided := fieldSetAt(fieldSets, index)
		if provided == nil {
			continue
		}
		if _, ok := provided["ip_rules"]; !ok {
			next[index].IPRules = cloneStrings(current[currentIndex].IPRules)
		}
		if _, ok := provided["protocol_rules"]; !ok {
			next[index].ProtocolRules = cloneStrings(current[currentIndex].ProtocolRules)
		}
	}
	return nil
}

func requestFieldProvided(fields map[string]struct{}, field string) bool {
	if fields == nil {
		return true
	}
	_, ok := fields[field]
	return ok
}

func fieldSetAt(fieldSets []map[string]struct{}, index int) map[string]struct{} {
	if index < 0 || index >= len(fieldSets) {
		return nil
	}
	return fieldSets[index]
}

func cloneStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string(nil), values...)
}
