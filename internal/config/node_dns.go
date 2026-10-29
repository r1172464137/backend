package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"reflect"
	"strconv"
	"strings"
)

// ParseNodeDNSJSON rejects ambiguous or unknown fields before normalizing.
func ParseNodeDNSJSON(raw []byte) ([]NodeDNS, error) {
	if err := ValidateUniqueJSON(raw); err != nil {
		return nil, err
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, err
	}
	for _, element := range elements {
		if _, err := exactJSONFields(element, reflect.TypeOf(NodeDNS{})); err != nil {
			return nil, err
		}
	}
	var input []NodeDNS
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return nil, err
	}
	return NormalizeNodeDNS(input)
}

// NormalizeNodeDNS implements the supported node DNS contract: UDP only,
// optional literal port, no scheme, userinfo, path, or TLS server name.
func NormalizeNodeDNS(input []NodeDNS) ([]NodeDNS, error) {
	out := make([]NodeDNS, 0, len(input))
	for i, item := range input {
		field := fmt.Sprintf("dns[%d]", i+1)
		proto := strings.ToLower(strings.TrimSpace(item.Proto))
		if proto != "" && proto != "udp" {
			return nil, fmt.Errorf("%s: only UDP is supported", field)
		}
		if item.ServerName != "" {
			return nil, fmt.Errorf("%s: server_name is unsupported", field)
		}
		address := item.Address
		if address == "" || address != strings.TrimSpace(address) || strings.ContainsAny(address, "/?#@\\% \t\r\n") {
			return nil, fmt.Errorf("%s: invalid resolver address", field)
		}
		host, port := address, ""
		if strings.HasPrefix(address, "[") {
			end := strings.IndexByte(address, ']')
			if end < 0 {
				return nil, fmt.Errorf("%s: invalid IPv6", field)
			}
			host = address[1:end]
			tail := address[end+1:]
			if tail != "" {
				if !strings.HasPrefix(tail, ":") || len(tail) == 1 {
					return nil, fmt.Errorf("%s: invalid port", field)
				}
				port = tail[1:]
			}
			ip, err := netip.ParseAddr(host)
			if err != nil || !ip.Is6() || ip.Zone() != "" {
				return nil, fmt.Errorf("%s: invalid IPv6", field)
			}
			host = ip.String()
		} else if ip, err := netip.ParseAddr(address); err == nil {
			if ip.Zone() != "" {
				return nil, fmt.Errorf("%s: invalid IP zone", field)
			}
			host = ip.String()
		} else if strings.Contains(address, ":") {
			var err error
			host, port, err = net.SplitHostPort(address)
			if err != nil || port == "" {
				return nil, fmt.Errorf("%s: invalid address or port", field)
			}
		}
		if port != "" {
			for _, r := range port {
				if r < '0' || r > '9' {
					return nil, fmt.Errorf("%s: invalid port", field)
				}
			}
			n, err := strconv.ParseUint(port, 10, 16)
			if err != nil || n == 0 {
				return nil, fmt.Errorf("%s: invalid port", field)
			}
			port = strconv.FormatUint(n, 10)
		}
		if ip, err := netip.ParseAddr(host); err == nil {
			if ip.Zone() != "" {
				return nil, fmt.Errorf("%s: invalid IP zone", field)
			}
			host = ip.String()
		} else {
			if strings.HasSuffix(host, "..") {
				return nil, fmt.Errorf("%s: invalid hostname", field)
			}
			trailing := strings.HasSuffix(host, ".")
			base := strings.TrimSuffix(host, ".")
			if strings.Count(base, ".") == 3 && strings.Trim(base, "0123456789.") == "" {
				if _, err := netip.ParseAddr(base); err != nil {
					return nil, fmt.Errorf("%s: invalid IP address", field)
				}
			}
			if len(base) == 0 || len(base) > 253 {
				return nil, fmt.Errorf("%s: invalid hostname", field)
			}
			for _, label := range strings.Split(base, ".") {
				if !domainLabel.MatchString(label) {
					return nil, fmt.Errorf("%s: invalid hostname", field)
				}
			}
			host = strings.ToLower(base)
			if trailing || host == "localhost" || host == "fakedns" {
				host += "."
			}
		}
		if port != "" {
			address = net.JoinHostPort(host, port)
		} else {
			address = host
		}
		if len(item.Domains) > 4096 {
			return nil, fmt.Errorf("%s: too many DNS rules", field)
		}
		for _, raw := range item.Domains {
			if len(strings.TrimSpace(raw)) > MaxRouteRuleBytes {
				return nil, fmt.Errorf("%s: DNS rule too long", field)
			}
		}
		domains, err := NormalizeDomainRules(field+".domains", item.Domains)
		if err != nil {
			return nil, err
		}
		out = append(out, NodeDNS{Proto: proto, Address: address, ServerName: "", Domains: domains})
	}
	if err := ValidatePersistedJSON("dns", out); err != nil {
		return nil, err
	}
	return out, nil
}
