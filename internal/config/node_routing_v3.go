package config

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Routing format 3 is the only supported persisted/delivered routing content
// (see .local/rebuild/CONTRACT.md). It adds account scope to rules, per-account
// default outbounds, outbound chains, one DNS policy and the target IP family
// preference. Older formats are rejected, never converted at runtime.
const (
	RoutingVersion           = 3
	MaxRoutingUserIDsPerRule = 1024
	MaxRoutingUserIDs        = 8192
	MaxRoutingUserDefaults   = 8192
	MaxDNSResolvers          = 128
	MaxDNSDomainsPerResolver = 1024
)

// NodeUserDefault sends an account's otherwise unmatched traffic to OutboundID.
type NodeUserDefault struct {
	UserID     int64  `json:"user_id"`
	OutboundID string `json:"outbound_id"`
}

// NodeDNSPolicy is the single DNS model. Resolvers are matched in order by
// domain; the last one is the default and the only one without domains.
type NodeDNSPolicy struct {
	QueryStrategy string            `json:"query_strategy"`
	Resolvers     []NodeDNSResolver `json:"resolvers"`
}

type NodeDNSResolver struct {
	ID          string   `json:"id"`
	Protocol    string   `json:"protocol"`
	Address     string   `json:"address"`
	Domains     []string `json:"domains"`
	OutboundID  string   `json:"outbound_id"`
	BootstrapIP string   `json:"bootstrap_ip,omitempty"`
}

// Outbound protocols that dial a remote peer and can therefore be reached
// through a preceding outbound.
var chainableOutboundProtocols = map[string]bool{
	"socks": true, "http": true, "shadowsocks": true, "vmess": true, "vless": true,
	"trojan": true, "anytls": true, "hysteria": true, "hysteria2": true,
}

func normalizedOutboundProtocol(p string) string {
	p = strings.ToLower(strings.TrimSpace(p))
	switch p {
	case "ss":
		return "shadowsocks"
	case "socks5":
		return "socks"
	}
	return p
}

// normalizeUserIDs validates a rule's account scope: present (not null),
// positive, deduplicated and sorted ascending.
func normalizeUserIDs(field string, ids []int64) ([]int64, error) {
	if ids == nil {
		return nil, errorsWithField(field, "must be an array (empty means all accounts)")
	}
	if len(ids) > MaxRoutingUserIDsPerRule {
		return nil, errorsWithField(field, "too many accounts")
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, errorsWithField(field, "account IDs must be positive")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

// normalizeUserDefaults validates per-account default outbounds.
func normalizeUserDefaults(input []NodeUserDefault, outboundIDs map[string]struct{}) ([]NodeUserDefault, error) {
	if input == nil {
		return nil, errorsWithField("routing.user_defaults", "must be an array")
	}
	if len(input) > MaxRoutingUserDefaults {
		return nil, errorsWithField("routing.user_defaults", "too many entries")
	}
	seen := make(map[int64]struct{}, len(input))
	out := make([]NodeUserDefault, 0, len(input))
	for i, d := range input {
		field := fmt.Sprintf("routing.user_defaults[%d]", i+1)
		if d.UserID <= 0 {
			return nil, errorsWithField(field+".user_id", "must be a positive account ID")
		}
		if _, ok := seen[d.UserID]; ok {
			return nil, errorsWithField(field+".user_id", "duplicate account")
		}
		seen[d.UserID] = struct{}{}
		if _, ok := outboundIDs[d.OutboundID]; !ok {
			return nil, errorsWithField(field+".outbound_id", fmt.Sprintf("unknown outbound %q", d.OutboundID))
		}
		out = append(out, NodeUserDefault{UserID: d.UserID, OutboundID: d.OutboundID})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UserID < out[j].UserID })
	return out, nil
}

// rawDialerProxy returns sockopt.dialerProxy from raw stream_settings. Keys are
// matched case-insensitively because Xray's JSON decoding is case-insensitive.
func rawDialerProxy(stream string) (string, error) {
	if strings.TrimSpace(stream) == "" {
		return "", nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stream), &top); err != nil {
		return "", err
	}
	for k, v := range top {
		if !strings.EqualFold(k, "sockopt") {
			continue
		}
		var sock map[string]json.RawMessage
		if err := json.Unmarshal(v, &sock); err != nil || sock == nil {
			continue
		}
		for sk, sv := range sock {
			if strings.EqualFold(sk, "dialerProxy") {
				var s string
				if err := json.Unmarshal(sv, &s); err != nil {
					return "", fmt.Errorf("sockopt.dialerProxy must be a string")
				}
				return strings.TrimSpace(s), nil
			}
		}
	}
	return "", nil
}

// validateOutboundChains checks via_outbound_id and raw dialerProxy references:
// they must name another custom outbound of the same policy, only dialing
// protocols may use them, and the reference graph must be acyclic.
func validateOutboundChains(outbounds []NodeOutbound) error {
	byID := make(map[string]NodeOutbound, len(outbounds))
	for _, o := range outbounds {
		byID[o.ID] = o
	}
	next := make(map[string]string, len(outbounds))
	for i, o := range outbounds {
		field := fmt.Sprintf("routing.outbounds[%d]", i+1)
		via := strings.TrimSpace(o.ViaOutboundID)
		raw, err := rawDialerProxy(o.StreamSettings)
		if err != nil {
			return errorsWithField(field+".stream_settings", err.Error())
		}
		if via != "" && raw != "" {
			return errorsWithField(field, "via_outbound_id and stream_settings.sockopt.dialerProxy cannot both be set")
		}
		target := via
		if raw != "" {
			if !strings.HasPrefix(raw, "routing_") {
				return errorsWithField(field+".stream_settings", "sockopt.dialerProxy must reference routing_<outbound id> of this policy")
			}
			target = strings.TrimPrefix(raw, "routing_")
		}
		if target == "" {
			continue
		}
		if target == RoutingDirect || target == RoutingBlock {
			return errorsWithField(field+".via_outbound_id", "must reference a custom outbound, not direct or block")
		}
		if target == o.ID {
			return errorsWithField(field+".via_outbound_id", "outbound cannot precede itself")
		}
		if _, ok := byID[target]; !ok {
			return errorsWithField(field+".via_outbound_id", fmt.Sprintf("unknown outbound %q", target))
		}
		if !chainableOutboundProtocols[normalizedOutboundProtocol(o.Protocol)] {
			return errorsWithField(field+".via_outbound_id", "only outbounds that dial a remote peer can use a preceding outbound")
		}
		next[o.ID] = target
	}
	for start := range next {
		seen := map[string]bool{start: true}
		for cur := next[start]; cur != ""; cur = next[cur] {
			if seen[cur] {
				return errorsWithField("routing.outbounds", "outbound chain contains a cycle")
			}
			seen[cur] = true
		}
	}
	return nil
}

// normalizeDNSPolicy validates the DNS policy and returns a detached copy.
// valueCount/byteCount accumulate the shared match budget with route values.
func normalizeDNSPolicy(input *NodeDNSPolicy, customOutbounds map[string]struct{}, valueCount, byteCount *int) (*NodeDNSPolicy, error) {
	if input == nil {
		return nil, errorsWithField("routing.dns", "must be an object")
	}
	switch input.QueryStrategy {
	case "UseIP", "UseIPv4", "UseIPv6":
	default:
		return nil, errorsWithField("routing.dns.query_strategy", "must be UseIP, UseIPv4 or UseIPv6")
	}
	if input.Resolvers == nil || len(input.Resolvers) == 0 || len(input.Resolvers) > MaxDNSResolvers {
		return nil, errorsWithField("routing.dns.resolvers", fmt.Sprintf("must contain 1..%d resolvers", MaxDNSResolvers))
	}
	out := &NodeDNSPolicy{QueryStrategy: input.QueryStrategy, Resolvers: make([]NodeDNSResolver, len(input.Resolvers))}
	ids := map[string]struct{}{}
	bootstrapByHost := map[string]string{}
	hostHasBootstrap := map[string]bool{}
	type hostRef struct {
		host, bootstrap, field string
	}
	var hosts []hostRef
	last := len(input.Resolvers) - 1
	for i, r := range input.Resolvers {
		field := fmt.Sprintf("routing.dns.resolvers[%d]", i+1)
		if !validRoutingID(r.ID, "dns_") {
			return nil, errorsWithField(field+".id", "must be dns_ followed by a lowercase UUIDv4")
		}
		if _, ok := ids[r.ID]; ok {
			return nil, errorsWithField(field+".id", "duplicate resolver ID")
		}
		ids[r.ID] = struct{}{}
		if r.Domains == nil {
			return nil, errorsWithField(field+".domains", "must be an array")
		}
		if len(r.Domains) > MaxDNSDomainsPerResolver {
			return nil, errorsWithField(field+".domains", "too many domains")
		}
		domains := []string{}
		if len(r.Domains) > 0 {
			var err error
			domains, err = NormalizeDomainRules(field+".domains", r.Domains)
			if err != nil {
				return nil, err
			}
		}
		if i == last && len(domains) != 0 {
			return nil, errorsWithField(field+".domains", "the last resolver is the default and must have no domains")
		}
		if i != last && len(domains) == 0 {
			return nil, errorsWithField(field+".domains", "only the last (default) resolver may have no domains")
		}
		*valueCount += len(domains)
		for _, d := range domains {
			*byteCount += len(d)
		}
		res := NodeDNSResolver{ID: r.ID, Protocol: r.Protocol, Address: strings.TrimSpace(r.Address), Domains: domains,
			OutboundID: r.OutboundID, BootstrapIP: strings.TrimSpace(r.BootstrapIP)}
		switch r.Protocol {
		case "system":
			if i != last {
				return nil, errorsWithField(field+".protocol", "system can only be the last (default) resolver")
			}
			if res.Address != "" || res.BootstrapIP != "" {
				return nil, errorsWithField(field, "system resolver takes no address or bootstrap_ip")
			}
			if res.OutboundID != RoutingDirect {
				return nil, errorsWithField(field+".outbound_id", "system resolver must use direct")
			}
		case "udp", "tcp", "https":
			host, err := validateResolverAddress(r.Protocol, res.Address)
			if err != nil {
				return nil, errorsWithField(field+".address", err.Error())
			}
			isIP := net.ParseIP(host) != nil
			if res.BootstrapIP != "" {
				if isIP {
					return nil, errorsWithField(field+".bootstrap_ip", "not allowed for an IP address resolver")
				}
				if net.ParseIP(res.BootstrapIP) == nil {
					return nil, errorsWithField(field+".bootstrap_ip", "must be an IP address")
				}
				res.BootstrapIP = net.ParseIP(res.BootstrapIP).String()
			}
			if !isIP {
				h := strings.ToLower(host)
				hosts = append(hosts, hostRef{h, res.BootstrapIP, field})
				if res.BootstrapIP != "" {
					hostHasBootstrap[h] = true
					if prev, ok := bootstrapByHost[h]; ok && prev != res.BootstrapIP {
						return nil, errorsWithField(field+".bootstrap_ip", "conflicts with another resolver for the same host")
					}
					bootstrapByHost[h] = res.BootstrapIP
				}
			}
			if res.OutboundID != RoutingDirect {
				if res.OutboundID == RoutingBlock {
					return nil, errorsWithField(field+".outbound_id", "DNS cannot use block")
				}
				if _, ok := customOutbounds[res.OutboundID]; !ok {
					return nil, errorsWithField(field+".outbound_id", fmt.Sprintf("unknown outbound %q", res.OutboundID))
				}
			}
		default:
			return nil, errorsWithField(field+".protocol", "must be udp, tcp, https or system")
		}
		out.Resolvers[i] = res
	}
	// One host, one connection address: if any resolver for a host names a
	// bootstrap IP, all resolvers for that host must name the same one.
	for _, h := range hosts {
		if hostHasBootstrap[h.host] && h.bootstrap != bootstrapByHost[h.host] {
			return nil, errorsWithField(h.field+".bootstrap_ip", "every resolver for this host must use the same bootstrap_ip")
		}
	}
	return out, nil
}

// validateResolverAddress returns the host of a resolver address.
func validateResolverAddress(protocol, address string) (string, error) {
	if address == "" {
		return "", fmt.Errorf("must not be empty")
	}
	if strings.Contains(address, "+local") {
		return "", fmt.Errorf("+local resolvers are not supported")
	}
	if protocol == "https" {
		u, err := url.Parse(address)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.Opaque != "" {
			return "", fmt.Errorf("must be an https:// URL")
		}
		if u.User != nil || u.Fragment != "" || strings.Contains(address, "#") {
			return "", fmt.Errorf("must not contain credentials or a fragment")
		}
		host := u.Hostname()
		if err := validResolverHost(host); err != nil {
			return "", err
		}
		if p := u.Port(); p != "" {
			if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
				return "", fmt.Errorf("invalid port")
			}
		}
		return host, nil
	}
	if strings.Contains(address, "://") || strings.ContainsAny(address, "/?#@") {
		return "", fmt.Errorf("must be host or host:port")
	}
	host, port := address, ""
	if h, p, err := net.SplitHostPort(address); err == nil {
		host, port = h, p
	} else if strings.Count(address, ":") > 1 && net.ParseIP(address) != nil {
		host = address // bare IPv6 literal
	} else if strings.Contains(address, ":") {
		return "", fmt.Errorf("must be host or host:port")
	}
	if port != "" {
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("invalid port")
		}
	}
	if err := validResolverHost(host); err != nil {
		return "", err
	}
	return host, nil
}

func validResolverHost(host string) error {
	if net.ParseIP(host) != nil {
		return nil
	}
	if host == "" || len(host) > 253 {
		return fmt.Errorf("invalid host")
	}
	for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
		if label == "" || len(label) > 63 {
			return fmt.Errorf("invalid host")
		}
		for i, c := range label {
			ok := c == '-' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
			if !ok || ((i == 0 || i == len(label)-1) && c == '-') {
				return fmt.Errorf("invalid host")
			}
		}
	}
	return nil
}

func validTargetIPPreference(v string) bool {
	return v == "none" || v == "prefer_ipv4" || v == "prefer_ipv6"
}

// AccountIDs returns every account referenced by rule scopes and account
// defaults.
func (p *NodeRouting) AccountIDs() map[int64]struct{} {
	ids := map[int64]struct{}{}
	if p == nil {
		return ids
	}
	for _, r := range p.Rules {
		for _, id := range r.UserIDs {
			ids[id] = struct{}{}
		}
	}
	for _, d := range p.UserDefaults {
		ids[d.UserID] = struct{}{}
	}
	return ids
}

// NewAccountIDs returns accounts referenced by next but not by previous. Only
// these must exist when saving: an account deleted after it was referenced
// stays in the policy and matches no identity, so deleting an account can
// never widen a rule to everyone or make the rest of the policy uneditable.
func NewAccountIDs(next, previous *NodeRouting) []int64 {
	old := previous.AccountIDs()
	var out []int64
	for id := range next.AccountIDs() {
		if _, ok := old[id]; !ok {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// DefaultResolverID derives a stable system-resolver ID for a scope.
func DefaultResolverID(scope string) string {
	sum := sha256.Sum256([]byte("ppanel-routing-v3-default-resolver/" + scope))
	b := sum[:16]
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("dns_%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// DefaultDNSPolicy is the explicit system default: query both families,
// one system resolver, no domain-specific resolvers.
func DefaultDNSPolicy(scope string) *NodeDNSPolicy {
	return &NodeDNSPolicy{QueryStrategy: "UseIP", Resolvers: []NodeDNSResolver{{
		ID: DefaultResolverID(scope), Protocol: "system", Address: "", Domains: []string{}, OutboundID: RoutingDirect}}}
}

// DefaultNodeRouting is an empty format 3 policy: everything goes direct.
func DefaultNodeRouting(scope string) *NodeRouting {
	return &NodeRouting{Version: RoutingVersion, Outbounds: []NodeOutbound{}, Rules: []NodeRoute{},
		DefaultOutboundID: RoutingDirect, UserDefaults: []NodeUserDefault{}, DNS: DefaultDNSPolicy(scope),
		TargetIPPreference: "none"}
}

// validateOutboundCapability mirrors the node's capability check
// (node core/outbound/capability.go): an outbound the node core cannot build
// or forward through is rejected when the administrator saves, not later as a
// failed candidate on the node. The cross-repository capability fixtures keep
// both sides in agreement.
func validateOutboundCapability(field string, o NodeOutbound) error {
	protocol := normalizedOutboundProtocol(o.Protocol)
	switch protocol {
	case "direct":
		protocol = "freedom"
	case "reject", "block":
		protocol = "blackhole"
	case "hysteria2":
		protocol = "hysteria"
	}
	switch protocol {
	case "freedom", "blackhole", "http", "socks", "shadowsocks", "trojan", "vmess", "vless", "anytls", "hysteria":
	case "tuic":
		return errorsWithField(field+".protocol", "TUIC outbound is not implemented by the node core")
	default:
		return errorsWithField(field+".protocol", fmt.Sprintf("unsupported outbound protocol %q", o.Protocol))
	}
	if o.UoT || o.UoTVersion != 0 {
		return errorsWithField(field, "UDP-over-TCP is unsupported")
	}
	if strings.TrimSpace(o.Plugin) != "" || HasNodePluginOptions(o.PluginOptions) {
		return errorsWithField(field, "outbound plugins are unsupported")
	}
	for _, raw := range []struct{ name, value string }{{"settings", o.Settings}, {"stream_settings", o.StreamSettings}, {"xhttp_extra", o.XHTTPExtra}} {
		if strings.TrimSpace(raw.value) == "" {
			continue
		}
		if err := rejectRawUoTAndPlugins(raw.value); err != nil {
			return errorsWithField(field+"."+raw.name, err.Error())
		}
	}
	if err := ValidateNodeURI("spider_x", o.SpiderX, true); err != nil {
		return errorsWithField(field+".spider_x", err.Error())
	}
	if err := ValidateNodeURI("path", o.Path, false); err != nil {
		return errorsWithField(field+".path", err.Error())
	}
	transport := strings.ToLower(strings.TrimSpace(o.Transport))
	if err := validateOutboundTransportName(transport); err != nil {
		return errorsWithField(field+".transport", err.Error())
	}
	network := transport
	if raw := strings.TrimSpace(o.StreamSettings); raw != "" {
		var stream map[string]json.RawMessage
		if json.Unmarshal([]byte(raw), &stream) == nil {
			network = "tcp"
			for k, v := range stream {
				if strings.EqualFold(k, "network") || strings.EqualFold(k, "method") {
					var s string
					if json.Unmarshal(v, &s) == nil {
						network = strings.ToLower(strings.TrimSpace(s))
					}
				}
			}
			if err := validateOutboundTransportName(network); err != nil {
				return errorsWithField(field+".stream_settings", err.Error())
			}
		}
	}
	if network == "" {
		network = "tcp"
		if protocol == "hysteria" {
			network = "hysteria"
		}
	}
	if protocol == "hysteria" && network != "hysteria" {
		return errorsWithField(field, "Hysteria outbound requires Hysteria transport")
	}
	if protocol != "hysteria" && network == "hysteria" {
		return errorsWithField(field, "Hysteria transport requires Hysteria outbound")
	}
	return nil
}

func validateOutboundTransportName(value string) error {
	switch value {
	case "", "raw", "tcp", "ws", "websocket", "grpc", "httpupgrade", "xhttp", "splithttp", "kcp", "mkcp", "hysteria":
		return nil
	case "tuic":
		return fmt.Errorf("TUIC outbound transport dialer is not implemented")
	default:
		return fmt.Errorf("unsupported outbound transport %q", value)
	}
}

// rejectRawUoTAndPlugins walks raw JSON: UDP-over-TCP and plugin semantics
// must not be hidden inside otherwise accepted raw settings.
func rejectRawUoTAndPlugins(raw string) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("invalid raw settings: %w", err)
	}
	var walk func(any) error
	walk = func(value any) error {
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				switch strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(key)) {
				case "uot", "uotversion", "udpovertcp", "udpovertcpversion":
					if child == false || child == json.Number("0") {
						continue
					}
					return fmt.Errorf("raw UDP-over-TCP semantics are unsupported")
				case "plugin", "pluginopts", "pluginoptions":
					if HasNodePluginOptions(child) {
						return fmt.Errorf("raw outbound plugins are unsupported")
					}
					continue
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
	return walk(value)
}
