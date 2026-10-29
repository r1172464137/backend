package nodeconfig

import (
	"encoding/json"
	"fmt"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
)

// GlobalValues contains only current non-routing settings. Legacy Block and
// Outbound columns are reserved for the offline cutover/backup, never read by
// the ordered runtime.
func GlobalValues(c config.NodeConfig) dto.ServerNodeConfigValues {
	dns := make([]dto.NodeDNS, 0, len(c.DNS))
	for _, d := range c.DNS {
		dns = append(dns, dto.NodeDNS{Proto: d.Proto, Address: d.Address, ServerName: d.ServerName, Domains: cloneStrings(d.Domains)})
	}
	return dto.ServerNodeConfigValues{IPStrategy: c.IPStrategy, DNS: ensureDNS(dns), RoutingRevision: c.RoutingRevision, RoutingMode: "global-v1"}
}
func ApplyOverride(values *dto.ServerNodeConfigValues, override *node.ServerConfigOverride) error {
	if values == nil || override == nil {
		return nil
	}
	if override.IPStrategy != nil {
		values.IPStrategy = *override.IPStrategy
	}
	if override.DNS != nil {
		dns, err := config.ParseNodeDNSJSON([]byte(*override.DNS))
		if err != nil {
			return fmt.Errorf("invalid DNS override: %w", err)
		}
		input := make([]dto.NodeDNS, 0, len(dns))
		for _, d := range dns {
			input = append(input, dto.NodeDNS{Proto: d.Proto, Address: d.Address, ServerName: d.ServerName, Domains: d.Domains})
		}
		normalized, err := normalizeDNS(input)
		if err != nil {
			return fmt.Errorf("invalid DNS override: %w", err)
		}
		values.DNS = normalized
	}
	return nil
}
func OverrideResponse(override *node.ServerConfigOverride) (dto.ServerNodeConfigOverride, error) {
	resp := dto.ServerNodeConfigOverride{InheritIPStrategy: true, InheritDNS: true, InheritRouting: true, DNS: []dto.NodeDNS{}}
	if override == nil {
		return resp, nil
	}
	resp.RoutingRevision = override.RoutingRevision
	resp.InheritRouting = override.Routing == nil
	if override.Routing != nil {
		policy, err := config.ParseNodeRouting([]byte(*override.Routing))
		if err != nil {
			return resp, err
		}
		resp.Routing = policy
	}
	if override.IPStrategy != nil {
		resp.InheritIPStrategy = false
		resp.IPStrategy = *override.IPStrategy
	}
	if override.DNS != nil {
		resp.InheritDNS = false
		dns, err := config.ParseNodeDNSJSON([]byte(*override.DNS))
		if err != nil {
			return resp, fmt.Errorf("invalid DNS override: %w", err)
		}
		input := make([]dto.NodeDNS, 0, len(dns))
		for _, d := range dns {
			input = append(input, dto.NodeDNS{Proto: d.Proto, Address: d.Address, ServerName: d.ServerName, Domains: d.Domains})
		}
		normalized, err := normalizeDNS(input)
		if err != nil {
			return resp, fmt.Errorf("invalid DNS override: %w", err)
		}
		resp.DNS = normalized
	}
	return resp, nil
}
func OverrideModel(serverID int64, req dto.ServerNodeConfigOverride) (*node.ServerConfigOverride, error) {
	data := &node.ServerConfigOverride{ServerId: serverID}
	if !req.InheritIPStrategy {
		if len(req.IPStrategy) > 32 {
			return nil, fmt.Errorf("ip_strategy exceeds 32 bytes")
		}
		v := req.IPStrategy
		data.IPStrategy = &v
	}
	if !req.InheritDNS {
		dns, err := normalizeDNS(req.DNS)
		if err != nil {
			return nil, err
		}
		if err := config.ValidatePersistedJSON("dns", dns); err != nil {
			return nil, err
		}
		b, err := json.Marshal(dns)
		if err != nil {
			return nil, err
		}
		v := string(b)
		data.DNS = &v
	}
	return data, nil
}
func CloneValues(values dto.ServerNodeConfigValues) dto.ServerNodeConfigValues {
	copy := values
	copy.DNS = ensureDNS(values.DNS)
	return copy
}
func normalizeDNS(values []dto.NodeDNS) ([]dto.NodeDNS, error) {
	input := make([]config.NodeDNS, 0, len(values))
	for _, d := range values {
		input = append(input, config.NodeDNS{Proto: d.Proto, Address: d.Address, ServerName: d.ServerName, Domains: d.Domains})
	}
	normal, err := config.NormalizeNodeDNS(input)
	if err != nil {
		return nil, err
	}
	result := make([]dto.NodeDNS, 0, len(normal))
	for _, d := range normal {
		result = append(result, dto.NodeDNS{Proto: d.Proto, Address: d.Address, ServerName: d.ServerName, Domains: d.Domains})
	}
	return result, nil
}
func ensureDNS(values []dto.NodeDNS) []dto.NodeDNS {
	result := make([]dto.NodeDNS, 0, len(values))
	for _, d := range values {
		d.Domains = cloneStrings(d.Domains)
		result = append(result, d)
	}
	return result
}
func cloneStrings(values []string) []string { return append([]string{}, values...) }
