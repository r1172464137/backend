package nodeconfig

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/pkg/errors"
)

func GlobalValues(c config.NodeConfig) dto.ServerNodeConfigValues {
	dns := make([]dto.NodeDNS, 0, len(c.DNS))
	for _, d := range c.DNS {
		dns = append(dns, dto.NodeDNS{
			Proto:      d.Proto,
			Address:    d.Address,
			ServerName: d.ServerName,
			Domains:    normalizeStrings(d.Domains),
		})
	}

	outbound := make([]dto.NodeOutbound, 0, len(c.Outbound))
	for _, o := range c.Outbound {
		outbound = append(outbound, dto.NodeOutbound{
			Name:                 o.Name,
			Protocol:             o.Protocol,
			Address:              o.Address,
			Port:                 o.Port,
			User:                 o.User,
			Password:             o.Password,
			UUID:                 o.UUID,
			Cipher:               o.Cipher,
			Plugin:               o.Plugin,
			PluginOptions:        o.PluginOptions,
			Security:             o.Security,
			SNI:                  o.SNI,
			ALPN:                 normalizeStrings(o.ALPN),
			AllowInsecure:        o.AllowInsecure,
			Fingerprint:          o.Fingerprint,
			Transport:            o.Transport,
			Host:                 o.Host,
			Path:                 o.Path,
			ServiceName:          o.ServiceName,
			XHTTPMode:            o.XHTTPMode,
			XHTTPExtra:           o.XHTTPExtra,
			Flow:                 o.Flow,
			Encryption:           o.Encryption,
			EncryptionMode:       o.EncryptionMode,
			EncryptionRTT:        o.EncryptionRTT,
			EncryptionTicket:     o.EncryptionTicket,
			EncryptionPadding:    o.EncryptionPadding,
			EncryptionPassword:   o.EncryptionPassword,
			Multiplex:            o.Multiplex,
			UoT:                  o.UoT,
			UoTVersion:           o.UoTVersion,
			CongestionController: o.CongestionController,
			UDPStream:            o.UDPStream,
			ReduceRtt:            o.ReduceRtt,
			Heartbeat:            o.Heartbeat,
			RealityPublicKey:     o.RealityPublicKey,
			RealityShortId:       o.RealityShortId,
			SpiderX:              o.SpiderX,
			Settings:             o.Settings,
			StreamSettings:       o.StreamSettings,
			Rules:                normalizeStrings(o.Rules),
			IPRules:              normalizeStrings(o.IPRules),
			ProtocolRules:        normalizeStrings(o.ProtocolRules),
		})
	}

	return dto.ServerNodeConfigValues{
		IPStrategy:     c.IPStrategy,
		DNS:            ensureDNS(dns),
		Block:          normalizeStrings(c.Block),
		BlockIPs:       normalizeStrings(c.BlockIPs),
		BlockProtocols: normalizeStrings(c.BlockProtocols),
		Outbound:       ensureOutbound(outbound),
	}
}

func ApplyOverride(values *dto.ServerNodeConfigValues, override *node.ServerConfigOverride) error {
	if values == nil || override == nil || override.Id == 0 {
		return nil
	}

	if override.IPStrategy != nil {
		values.IPStrategy = *override.IPStrategy
	}
	if override.DNS != nil {
		var dns []dto.NodeDNS
		if err := unmarshalJSONField(*override.DNS, &dns, "dns"); err != nil {
			return err
		}
		values.DNS = ensureDNS(dns)
	}
	if override.Block != nil || override.BlockIPs != nil || override.BlockProtocols != nil {
		values.Block = []string{}
		values.BlockIPs = []string{}
		values.BlockProtocols = []string{}
		if override.Block != nil {
			var block []string
			if err := unmarshalJSONField(*override.Block, &block, "block"); err != nil {
				return err
			}
			values.Block = normalizeStrings(block)
		}
		if override.BlockIPs != nil {
			var blockIPs []string
			if err := unmarshalJSONField(*override.BlockIPs, &blockIPs, "block_ips"); err != nil {
				return err
			}
			values.BlockIPs = normalizeStrings(blockIPs)
		}
		if override.BlockProtocols != nil {
			var blockProtocols []string
			if err := unmarshalJSONField(*override.BlockProtocols, &blockProtocols, "block_protocols"); err != nil {
				return err
			}
			values.BlockProtocols = normalizeStrings(blockProtocols)
		}
	}
	if override.Outbound != nil {
		var outbound []dto.NodeOutbound
		if err := unmarshalJSONField(*override.Outbound, &outbound, "outbound"); err != nil {
			return err
		}
		values.Outbound = ensureOutbound(outbound)
	}
	return nil
}

func OverrideResponse(override *node.ServerConfigOverride) (dto.ServerNodeConfigOverride, error) {
	resp := dto.ServerNodeConfigOverride{
		InheritIPStrategy: true,
		InheritDNS:        true,
		InheritBlock:      true,
		InheritOutbound:   true,
		DNS:               []dto.NodeDNS{},
		Block:             []string{},
		BlockIPs:          []string{},
		BlockProtocols:    []string{},
		Outbound:          []dto.NodeOutbound{},
	}
	if override == nil || override.Id == 0 {
		return resp, nil
	}

	if override.IPStrategy != nil {
		resp.InheritIPStrategy = false
		resp.IPStrategy = *override.IPStrategy
	}
	if override.DNS != nil {
		resp.InheritDNS = false
		var dns []dto.NodeDNS
		if err := unmarshalJSONField(*override.DNS, &dns, "dns"); err != nil {
			return resp, err
		}
		resp.DNS = ensureDNS(dns)
	}
	if override.Block != nil || override.BlockIPs != nil || override.BlockProtocols != nil {
		resp.InheritBlock = false
		if override.Block != nil {
			var block []string
			if err := unmarshalJSONField(*override.Block, &block, "block"); err != nil {
				return resp, err
			}
			resp.Block = normalizeStrings(block)
		}
		if override.BlockIPs != nil {
			var blockIPs []string
			if err := unmarshalJSONField(*override.BlockIPs, &blockIPs, "block_ips"); err != nil {
				return resp, err
			}
			resp.BlockIPs = normalizeStrings(blockIPs)
		}
		if override.BlockProtocols != nil {
			var blockProtocols []string
			if err := unmarshalJSONField(*override.BlockProtocols, &blockProtocols, "block_protocols"); err != nil {
				return resp, err
			}
			resp.BlockProtocols = normalizeStrings(blockProtocols)
		}
	}
	if override.Outbound != nil {
		resp.InheritOutbound = false
		var outbound []dto.NodeOutbound
		if err := unmarshalJSONField(*override.Outbound, &outbound, "outbound"); err != nil {
			return resp, err
		}
		resp.Outbound = ensureOutbound(outbound)
	}

	return resp, nil
}

// MergeRouteRuleOmissions preserves additive fields hidden from older admin
// clients. Explicitly submitted empty arrays remain empty.
func MergeRouteRuleOmissions(req *dto.UpdateServerNodeConfigRequest, existing dto.ServerNodeConfigOverride, inheritedOutbound []dto.NodeOutbound) error {
	if req == nil {
		return errors.New("server node config request is nil")
	}
	if !req.InheritOutbound {
		if err := normalizeOutboundIdentities(req.Outbound); err != nil {
			return err
		}
	}
	if req.RequestFields == nil {
		return nil
	}
	if !req.InheritBlock {
		if _, ok := req.RequestFields["block_ips"]; !ok {
			req.BlockIPs = cloneStrings(existing.BlockIPs)
		}
		if _, ok := req.RequestFields["block_protocols"]; !ok {
			req.BlockProtocols = cloneStrings(existing.BlockProtocols)
		}
	}
	if req.InheritOutbound {
		return nil
	}
	current := existing.Outbound
	if existing.InheritOutbound {
		current = inheritedOutbound
	}
	return mergeOutboundRuleOmissions(req.Outbound, current, req.OutboundFieldSets)
}

func OverrideModel(serverID int64, req dto.ServerNodeConfigOverride) (*node.ServerConfigOverride, bool, error) {
	data := &node.ServerConfigOverride{
		ServerId: serverID,
	}

	if !req.InheritIPStrategy {
		if len(req.IPStrategy) > 32 {
			return nil, false, &config.RouteRuleError{Field: "ip_strategy", Message: "exceeds the 32-byte database limit"}
		}
		data.IPStrategy = stringPtr(req.IPStrategy)
	}
	if !req.InheritDNS {
		value, err := marshalJSONField(ensureDNS(req.DNS), "dns")
		if err != nil {
			return nil, false, err
		}
		if err := config.ValidateSerializedJSON("dns", value); err != nil {
			return nil, false, err
		}
		data.DNS = &value
	}
	if !req.InheritBlock {
		block, err := config.NormalizeDomainRules("block", req.Block)
		if err != nil {
			return nil, false, err
		}
		blockIPs, err := config.NormalizeIPRules("block_ips", req.BlockIPs)
		if err != nil {
			return nil, false, err
		}
		blockProtocols, err := config.NormalizeProtocolRules("block_protocols", req.BlockProtocols)
		if err != nil {
			return nil, false, err
		}
		blockValue, err := marshalJSONField(block, "block")
		if err != nil {
			return nil, false, err
		}
		blockIPValue, err := marshalJSONField(blockIPs, "block_ips")
		if err != nil {
			return nil, false, err
		}
		blockProtocolValue, err := marshalJSONField(blockProtocols, "block_protocols")
		if err != nil {
			return nil, false, err
		}
		for _, persisted := range []struct{ field, value string }{
			{"block", blockValue}, {"block_ips", blockIPValue}, {"block_protocols", blockProtocolValue},
		} {
			if err := config.ValidateSerializedJSON(persisted.field, persisted.value); err != nil {
				return nil, false, err
			}
		}
		data.Block = &blockValue
		data.BlockIPs = &blockIPValue
		data.BlockProtocols = &blockProtocolValue
	}
	if !req.InheritOutbound {
		outbound, err := normalizeOutboundRulesForWrite(req.Outbound)
		if err != nil {
			return nil, false, err
		}
		value, err := marshalJSONField(outbound, "outbound")
		if err != nil {
			return nil, false, err
		}
		if err := config.ValidateSerializedJSON("outbound", value); err != nil {
			return nil, false, err
		}
		data.Outbound = &value
	}

	allInherited := data.IPStrategy == nil && data.DNS == nil && data.Block == nil && data.BlockIPs == nil && data.BlockProtocols == nil && data.Outbound == nil
	return data, allInherited, nil
}

func CloneValues(values dto.ServerNodeConfigValues) dto.ServerNodeConfigValues {
	dns := make([]dto.NodeDNS, 0, len(values.DNS))
	for _, d := range values.DNS {
		dns = append(dns, dto.NodeDNS{
			Proto:      d.Proto,
			Address:    d.Address,
			ServerName: d.ServerName,
			Domains:    normalizeStrings(d.Domains),
		})
	}

	outbound := make([]dto.NodeOutbound, 0, len(values.Outbound))
	for _, o := range values.Outbound {
		outbound = append(outbound, dto.NodeOutbound{
			Name:                 o.Name,
			Protocol:             o.Protocol,
			Address:              o.Address,
			Port:                 o.Port,
			User:                 o.User,
			Password:             o.Password,
			UUID:                 o.UUID,
			Cipher:               o.Cipher,
			Plugin:               o.Plugin,
			PluginOptions:        o.PluginOptions,
			Security:             o.Security,
			SNI:                  o.SNI,
			ALPN:                 normalizeStrings(o.ALPN),
			AllowInsecure:        o.AllowInsecure,
			Fingerprint:          o.Fingerprint,
			Transport:            o.Transport,
			Host:                 o.Host,
			Path:                 o.Path,
			ServiceName:          o.ServiceName,
			XHTTPMode:            o.XHTTPMode,
			XHTTPExtra:           o.XHTTPExtra,
			Flow:                 o.Flow,
			Encryption:           o.Encryption,
			EncryptionMode:       o.EncryptionMode,
			EncryptionRTT:        o.EncryptionRTT,
			EncryptionTicket:     o.EncryptionTicket,
			EncryptionPadding:    o.EncryptionPadding,
			EncryptionPassword:   o.EncryptionPassword,
			Multiplex:            o.Multiplex,
			UoT:                  o.UoT,
			UoTVersion:           o.UoTVersion,
			CongestionController: o.CongestionController,
			UDPStream:            o.UDPStream,
			ReduceRtt:            o.ReduceRtt,
			Heartbeat:            o.Heartbeat,
			RealityPublicKey:     o.RealityPublicKey,
			RealityShortId:       o.RealityShortId,
			SpiderX:              o.SpiderX,
			Settings:             o.Settings,
			StreamSettings:       o.StreamSettings,
			Rules:                normalizeStrings(o.Rules),
			IPRules:              normalizeStrings(o.IPRules),
			ProtocolRules:        normalizeStrings(o.ProtocolRules),
		})
	}

	return dto.ServerNodeConfigValues{
		IPStrategy:     values.IPStrategy,
		DNS:            ensureDNS(dns),
		Block:          normalizeStrings(values.Block),
		BlockIPs:       normalizeStrings(values.BlockIPs),
		BlockProtocols: normalizeStrings(values.BlockProtocols),
		Outbound:       ensureOutbound(outbound),
	}
}

func unmarshalJSONField[T any](value string, target *T, field string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(value), target); err != nil {
		return errors.Wrapf(err, "unmarshal server node config %s", field)
	}
	return nil
}

func marshalJSONField(value any, field string) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", errors.Wrapf(err, "marshal server node config %s", field)
	}
	return string(data), nil
}

func normalizeStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func ensureDNS(values []dto.NodeDNS) []dto.NodeDNS {
	if values == nil {
		return []dto.NodeDNS{}
	}
	result := make([]dto.NodeDNS, 0, len(values))
	for _, item := range values {
		proto := strings.TrimSpace(item.Proto)
		address := strings.TrimSpace(item.Address)
		if proto == "" || address == "" {
			continue
		}
		result = append(result, dto.NodeDNS{
			Proto:      proto,
			Address:    address,
			ServerName: strings.TrimSpace(item.ServerName),
			Domains:    normalizeStrings(item.Domains),
		})
	}
	return result
}

func ensureOutbound(values []dto.NodeOutbound) []dto.NodeOutbound {
	if values == nil {
		return []dto.NodeOutbound{}
	}
	result := make([]dto.NodeOutbound, 0, len(values))
	for _, item := range values {
		name := strings.TrimSpace(item.Name)
		protocol := strings.TrimSpace(item.Protocol)
		rules := normalizeStrings(item.Rules)
		ipRules := normalizeStrings(item.IPRules)
		protocolRules := normalizeStrings(item.ProtocolRules)
		if name == "" || protocol == "" {
			continue
		}
		result = append(result, dto.NodeOutbound{
			Name:                 name,
			Protocol:             protocol,
			Address:              strings.TrimSpace(item.Address),
			Port:                 item.Port,
			User:                 strings.TrimSpace(item.User),
			Password:             item.Password,
			UUID:                 strings.TrimSpace(item.UUID),
			Cipher:               strings.TrimSpace(item.Cipher),
			Plugin:               strings.TrimSpace(item.Plugin),
			PluginOptions:        item.PluginOptions,
			Security:             strings.TrimSpace(item.Security),
			SNI:                  strings.TrimSpace(item.SNI),
			ALPN:                 normalizeStrings(item.ALPN),
			AllowInsecure:        item.AllowInsecure,
			Fingerprint:          strings.TrimSpace(item.Fingerprint),
			Transport:            strings.TrimSpace(item.Transport),
			Host:                 strings.TrimSpace(item.Host),
			Path:                 strings.TrimSpace(item.Path),
			ServiceName:          strings.TrimSpace(item.ServiceName),
			XHTTPMode:            strings.TrimSpace(item.XHTTPMode),
			XHTTPExtra:           strings.TrimSpace(item.XHTTPExtra),
			Flow:                 strings.TrimSpace(item.Flow),
			Encryption:           strings.TrimSpace(item.Encryption),
			EncryptionMode:       strings.TrimSpace(item.EncryptionMode),
			EncryptionRTT:        strings.TrimSpace(item.EncryptionRTT),
			EncryptionTicket:     strings.TrimSpace(item.EncryptionTicket),
			EncryptionPadding:    strings.TrimSpace(item.EncryptionPadding),
			EncryptionPassword:   item.EncryptionPassword,
			Multiplex:            strings.TrimSpace(item.Multiplex),
			UoT:                  item.UoT,
			UoTVersion:           item.UoTVersion,
			CongestionController: strings.TrimSpace(item.CongestionController),
			UDPStream:            item.UDPStream,
			ReduceRtt:            item.ReduceRtt,
			Heartbeat:            item.Heartbeat,
			RealityPublicKey:     strings.TrimSpace(item.RealityPublicKey),
			RealityShortId:       strings.TrimSpace(item.RealityShortId),
			SpiderX:              strings.TrimSpace(item.SpiderX),
			Settings:             strings.TrimSpace(item.Settings),
			StreamSettings:       strings.TrimSpace(item.StreamSettings),
			Rules:                rules,
			IPRules:              ipRules,
			ProtocolRules:        protocolRules,
		})
	}
	return result
}

func normalizeOutboundIdentities(values []dto.NodeOutbound) error {
	names := make([]string, len(values))
	protocols := make([]string, len(values))
	for index := range values {
		names[index] = values[index].Name
		protocols[index] = values[index].Protocol
	}
	names, err := config.NormalizeOutboundNames("outbound", names)
	if err != nil {
		return err
	}
	protocols, err = config.NormalizeOutboundProtocols("outbound", protocols)
	if err != nil {
		return err
	}
	for index := range values {
		values[index].Name = names[index]
		values[index].Protocol = protocols[index]
	}
	return nil
}

func normalizeOutboundRulesForWrite(values []dto.NodeOutbound) ([]dto.NodeOutbound, error) {
	if err := normalizeOutboundIdentities(values); err != nil {
		return nil, err
	}
	values = ensureOutbound(values)
	totalRules, totalRuleBytes := 0, 0
	for index := range values {
		field := fmt.Sprintf("outbound[%d]", index+1)
		var err error
		if values[index].Rules, err = config.NormalizeDomainRules(field+".rules", values[index].Rules); err != nil {
			return nil, err
		}
		if values[index].IPRules, err = config.NormalizeIPRules(field+".ip_rules", values[index].IPRules); err != nil {
			return nil, err
		}
		if values[index].ProtocolRules, err = config.NormalizeProtocolRules(field+".protocol_rules", values[index].ProtocolRules); err != nil {
			return nil, err
		}
		for _, rules := range [][]string{values[index].Rules, values[index].IPRules, values[index].ProtocolRules} {
			totalRules += len(rules)
			for _, rule := range rules {
				totalRuleBytes += len(rule)
			}
		}
	}
	if err := config.ValidateOutboundAggregate("outbound", len(values), totalRules, totalRuleBytes); err != nil {
		return nil, err
	}
	return values, nil
}

func mergeOutboundRuleOmissions(next, current []dto.NodeOutbound, fieldSets []map[string]struct{}) error {
	needsMatch := false
	for index := range next {
		if index >= len(fieldSets) || fieldSets[index] == nil {
			continue
		}
		_, hasIP := fieldSets[index]["ip_rules"]
		_, hasProtocol := fieldSets[index]["protocol_rules"]
		if !hasIP || !hasProtocol {
			needsMatch = true
			break
		}
	}
	if !needsMatch {
		return nil
	}
	nextInputs := make([]config.LegacyOutboundMatchInput, len(next))
	for index := range next {
		nextInputs[index] = config.LegacyOutboundMatchInput{Name: next[index].Name, Value: next[index]}
	}
	currentInputs := make([]config.LegacyOutboundMatchInput, len(current))
	for index := range current {
		currentInputs[index] = config.LegacyOutboundMatchInput{Name: current[index].Name, Value: current[index]}
	}
	matches, err := config.MatchLegacyOutbounds("outbound", nextInputs, currentInputs)
	if err != nil {
		return err
	}
	for index, currentIndex := range matches {
		if currentIndex < 0 || index >= len(fieldSets) || fieldSets[index] == nil {
			continue
		}
		if _, ok := fieldSets[index]["ip_rules"]; !ok {
			next[index].IPRules = cloneStrings(current[currentIndex].IPRules)
		}
		if _, ok := fieldSets[index]["protocol_rules"]; !ok {
			next[index].ProtocolRules = cloneStrings(current[currentIndex].ProtocolRules)
		}
	}
	return nil
}

func cloneStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string(nil), values...)
}

func stringPtr(value string) *string {
	return &value
}
