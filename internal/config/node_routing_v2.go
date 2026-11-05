package config

import (
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"sort"
	"strings"
)

// HasRoutingV2Capabilities reports whether version downgrade would discard configured semantics.
func HasRoutingV2Capabilities(p *NodeRouting) bool {
	if p == nil || p.Version != RoutingVersion2 {
		return false
	}
	if len(p.UserDefaults) > 0 || p.DNSPolicy != nil {
		return true
	}
	for _, r := range p.Rules {
		// Node v2 uses IPIfNonMatch; v1 AsIs loses domain-to-IP matches.
		if len(r.UserIDs) > 0 || (r.Kind == "ip" && r.Enabled != nil && *r.Enabled && len(r.Values) > 0) {
			return true
		}
	}
	for _, o := range p.Outbounds {
		if o.ViaOutboundID != "" {
			return true
		}
	}
	return false
}

func normalizeRoutingUsers(in []int64) ([]int64, error) {
	if len(in) > MaxRoutingValuesPerRule {
		return nil, fmt.Errorf("too many user IDs")
	}
	if len(in) == 0 {
		return nil, nil
	}
	seen := make(map[int64]bool, len(in))
	result := make([]int64, 0, len(in))
	for _, id := range in {
		if id <= 0 {
			return nil, fmt.Errorf("user IDs must be positive")
		}
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func rejectDNSSystemDialer(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var root any
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return err
	}
	var walk func(any) error
	walk = func(value any) error {
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				if err := rejectDNSSystemSocket(key, child); err != nil {
					return err
				}
				if err := walk(child); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range v {
				if err := walk(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(root)
}

func rejectDNSSystemSocket(key string, child any) error {
	if strings.EqualFold(key, "sockopt") {
		if socket, ok := child.(map[string]any); ok {
			for name, setting := range socket {
				if strings.EqualFold(name, "addressPortStrategy") {
					strategy, ok := setting.(string)
					if !ok || (strategy != "" && !strings.EqualFold(strategy, "none")) {
						return fmt.Errorf("addressPortStrategy cannot use system DNS with explicit DNS policy")
					}
				}
			}
		}
	}
	return nil
}

func normalizeRoutingV2(input, result *NodeRouting, ids map[string]struct{}) error {
	byID := make(map[string]NodeOutbound, len(result.Outbounds))
	for _, out := range result.Outbounds {
		byID[out.ID] = out
	}
	links, err := normalizeRoutingChains(input, result, byID)
	if err != nil {
		return err
	}
	if err := normalizeRoutingDefaults(input, result, ids); err != nil {
		return err
	}
	return normalizeRoutingDNS(input, result, ids, byID, links)
}

func normalizeRoutingChains(input, result *NodeRouting, byID map[string]NodeOutbound) (map[string]string, error) {
	links := make(map[string]string, len(byID))
	for _, out := range result.Outbounds {
		if input.DNSPolicy != nil {
			for _, raw := range []string{out.StreamSettings, out.XHTTPExtra} {
				if err := rejectDNSSystemDialer(raw); err != nil {
					return nil, errorsWithField("routing.outbounds", err.Error())
				}
			}
		}
		via, err := routingOutboundVia(out)
		if err != nil {
			return nil, err
		}
		if via != "" {
			if _, ok := byID[via]; !ok {
				return nil, errorsWithField("routing.outbounds", fmt.Sprintf("unknown via outbound %q", via))
			}
		}
		links[out.ID] = via
	}
	for _, out := range result.Outbounds {
		visited := make(map[string]bool)
		for current := out.ID; current != ""; current = links[current] {
			if visited[current] {
				return nil, errorsWithField("routing.outbounds", "outbound chain contains a cycle")
			}
			visited[current] = true
		}
	}
	return links, nil
}

// routingOutboundVia checks the raw dialer before resolving the resulting link.
func routingOutboundVia(out NodeOutbound) (string, error) {
	via := out.ViaOutboundID
	if out.StreamSettings == "" {
		return via, nil
	}
	var value map[string]json.RawMessage
	_ = json.Unmarshal([]byte(out.StreamSettings), &value)
	for k, raw := range value {
		if !strings.EqualFold(k, "sockopt") {
			continue
		}
		var socket map[string]json.RawMessage
		if err := json.Unmarshal(raw, &socket); err != nil && string(raw) != "null" {
			return "", errorsWithField("routing.outbounds", "invalid raw sockopt")
		}
		for name, value := range socket {
			if !strings.EqualFold(name, "dialerProxy") {
				continue
			}
			var tag string
			if err := json.Unmarshal(value, &tag); err != nil {
				return "", errorsWithField("routing.outbounds", "invalid raw dialerProxy")
			}
			if tag == "" {
				continue
			}
			if via != "" {
				return "", errorsWithField("routing.outbounds", "raw dialerProxy conflicts with via_outbound_id")
			}
			if tag == "Default" || tag == "block" {
				continue
			}
			if !strings.HasPrefix(tag, "routing_") {
				return "", errorsWithField("routing.outbounds", "raw dialerProxy must reference an existing outbound")
			}
			via = strings.TrimPrefix(tag, "routing_")
		}
	}
	return via, nil
}

func normalizeRoutingDefaults(input, result *NodeRouting, ids map[string]struct{}) error {
	if len(input.UserDefaults) > MaxRoutingValues {
		return errorsWithField("routing.user_defaults", "too many entries")
	}
	result.UserDefaults = make([]NodeUserDefault, 0, len(input.UserDefaults))
	seenUsers := make(map[int64]bool, len(input.UserDefaults))
	for _, d := range input.UserDefaults {
		if d.UserID <= 0 || seenUsers[d.UserID] {
			return errorsWithField("routing.user_defaults", "invalid or duplicate user")
		}
		if _, ok := ids[d.OutboundID]; !ok {
			return errorsWithField("routing.user_defaults", "unknown outbound")
		}
		seenUsers[d.UserID] = true
		result.UserDefaults = append(result.UserDefaults, d)
	}
	sort.Slice(result.UserDefaults, func(i, j int) bool { return result.UserDefaults[i].UserID < result.UserDefaults[j].UserID })
	return nil
}

func normalizeRoutingDNS(input, result *NodeRouting, ids map[string]struct{}, byID map[string]NodeOutbound, links map[string]string) error {
	if input.DNSPolicy == nil {
		return nil
	}
	policy := input.DNSPolicy
	switch policy.QueryStrategy {
	case "UseIP", "UseIPv4", "UseIPv6":
	default:
		return errorsWithField("routing.dns_policy.query_strategy", "unsupported query strategy")
	}
	if len(policy.Resolvers) == 0 || len(policy.Resolvers) > 128 {
		return errorsWithField("routing.dns_policy.resolvers", "must contain 1..128 resolvers")
	}
	result.DNSPolicy = &NodeDNSPolicy{QueryStrategy: policy.QueryStrategy, Resolvers: make([]NodeDNSResolver, 0, len(policy.Resolvers))}
	seenDNS := make(map[string]bool)
	bootstrapHosts := make(map[string]string)
	for i, resolver := range policy.Resolvers {
		field := fmt.Sprintf("routing.dns_policy.resolvers[%d]", i+1)
		if !validRoutingID(resolver.ID, "dns_") || seenDNS[resolver.ID] {
			return errorsWithField(field+".id", "invalid or duplicate resolver ID")
		}
		seenDNS[resolver.ID] = true
		if _, ok := ids[resolver.OutboundID]; !ok || resolver.OutboundID == RoutingBlock {
			return errorsWithField(field+".outbound_id", "unknown or block outbound")
		}
		if err := validateResolverOutbound(field, resolver, byID, links); err != nil {
			return err
		}
		if resolver.Domains == nil || len(resolver.Domains) > MaxRoutingValuesPerRule || (i == len(policy.Resolvers)-1) != (len(resolver.Domains) == 0) {
			return errorsWithField(field+".domains", "only the last resolver must have empty domains")
		}
		domains, err := NormalizeDomainRules(field+".domains", resolver.Domains)
		if err != nil {
			return err
		}
		if (i == len(policy.Resolvers)-1) != (len(domains) == 0) {
			return errorsWithField(field+".domains", "only the last resolver may have empty normalized domains")
		}
		resolver.Domains = append([]string{}, domains...)
		if resolver.BootstrapIP != "" {
			ip, err := netip.ParseAddr(resolver.BootstrapIP)
			if err != nil || ip.Zone() != "" {
				return errorsWithField(field+".bootstrap_ip", "invalid IP address")
			}
			resolver.BootstrapIP = ip.String()
		}
		host, err := normalizeResolverAddress(field, &resolver)
		if err != nil {
			return err
		}
		if err := validateResolverBootstrap(field, host, resolver, len(policy.Resolvers), bootstrapHosts); err != nil {
			return err
		}
		result.DNSPolicy.Resolvers = append(result.DNSPolicy.Resolvers, resolver)
	}
	return nil
}

func validateResolverBootstrap(field, host string, resolver NodeDNSResolver, count int, bootstrapHosts map[string]string) error {
	host = strings.Trim(host, "[]")
	_, literalErr := netip.ParseAddr(host)
	if resolver.BootstrapIP != "" {
		if literalErr == nil {
			return errorsWithField(field+".bootstrap_ip", "cannot bootstrap an IP literal resolver")
		}
		canonicalHost := strings.ToLower(strings.TrimSuffix(host, "."))
		if previous, ok := bootstrapHosts[canonicalHost]; ok && previous != resolver.BootstrapIP {
			return errorsWithField(field+".bootstrap_ip", "conflicting bootstrap IPs for hostname")
		}
		bootstrapHosts[canonicalHost] = resolver.BootstrapIP
	}
	// A single direct resolver hostname otherwise asks itself for its address.
	if count == 1 && resolver.OutboundID == RoutingDirect && resolver.BootstrapIP == "" {
		if literalErr != nil {
			return errorsWithField(field+".bootstrap_ip", "required for the resolver's direct self-resolution cycle")
		}
	}
	return nil
}

func validateResolverOutbound(field string, resolver NodeDNSResolver, byID map[string]NodeOutbound, links map[string]string) error {
	if resolver.Protocol == "udp" {
		if out, ok := byID[resolver.OutboundID]; ok {
			if strings.EqualFold(out.Protocol, "http") {
				return errorsWithField(field+".outbound_id", "HTTP outbound does not support UDP DNS")
			}
			if strings.EqualFold(out.Protocol, "socks") {
				if via, ok := byID[links[out.ID]]; ok && strings.EqualFold(via.Protocol, "http") {
					return errorsWithField(field+".outbound_id", "SOCKS UDP through an HTTP predecessor is unsupported")
				}
			}
		}
	}
	return nil
}

func normalizeResolverAddress(field string, resolver *NodeDNSResolver) (string, error) {
	switch resolver.Protocol {
	case "udp", "tcp":
		normalized, err := NormalizeNodeDNS([]NodeDNS{{Proto: "udp", Address: resolver.Address, Domains: []string{}}})
		if err != nil {
			return "", errorsWithField(field+".address", err.Error())
		}
		resolver.Address = normalized[0].Address
		host := resolver.Address
		if parsed, _, err := net.SplitHostPort(host); err == nil {
			host = parsed
		}
		return host, nil
	case "https":
		u, err := url.Parse(resolver.Address)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
			return "", errorsWithField(field+".address", "expected HTTPS URL without userinfo or fragment")
		}
		if _, err := NormalizeNodeDNS([]NodeDNS{{Proto: "udp", Address: u.Host, Domains: []string{}}}); err != nil {
			return "", errorsWithField(field+".address", "invalid HTTPS host or port")
		}
		return u.Hostname(), nil
	default:
		return "", errorsWithField(field+".protocol", "unsupported resolver protocol")
	}
}
