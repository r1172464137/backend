package config

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"

	"github.com/google/uuid"
)

// LegacyRoutingInput is only used by the offline 02179 preflight.
type LegacyRoutingInput struct {
	Block          []string
	BlockIPs       []string
	BlockProtocols []string
	Outbound       []NodeOutbound
}

type LegacyRoutingBlocker struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}
type LegacyRoutingSource struct {
	Field      string   `json:"field"`
	Kind       string   `json:"kind"`
	OutboundID string   `json:"outbound_id"`
	Values     []string `json:"values"`
}
type LegacyRoutingPreview struct {
	Draft    *NodeRouting           `json:"draft"`
	Blockers []LegacyRoutingBlocker `json:"blockers"`
	Sources  []LegacyRoutingSource  `json:"sources"`
}

// ConvertLegacyRouting converts the node's effective legacy inputs in exactly
// the builder's order. A blocker suppresses the whole draft; no object or rule
// is silently renamed, discarded, or replaced with direct.
func ConvertLegacyRouting(legacy LegacyRoutingInput, scope string) (LegacyRoutingPreview, error) {
	if scope == "" {
		return LegacyRoutingPreview{}, fmt.Errorf("cutover scope is required")
	}
	result := LegacyRoutingPreview{Blockers: []LegacyRoutingBlocker{}, Sources: []LegacyRoutingSource{}}
	rawCount := 0
	checkRaw := func(field string, values []string) {
		rawCount += len(values)
		if len(values) > 4096 {
			result.Blockers = append(result.Blockers, LegacyRoutingBlocker{field, "legacy node raw category limit exceeded"})
		}
	}
	checkRaw("block", legacy.Block)
	checkRaw("block_ips", legacy.BlockIPs)
	checkRaw("block_protocols", legacy.BlockProtocols)
	for i, ob := range legacy.Outbound {
		field := fmt.Sprintf("outbound[%d]", i+1)
		checkRaw(field+".rules", ob.Rules)
		checkRaw(field+".ip_rules", ob.IPRules)
		checkRaw(field+".protocol_rules", ob.ProtocolRules)
	}
	if rawCount > 8192 {
		result.Blockers = append(result.Blockers, LegacyRoutingBlocker{"routing", "legacy node raw aggregate limit exceeded"})
	}
	outboundRules, outboundBytes := 0, 0
	policy := &NodeRouting{Version: 1, DefaultOutboundID: RoutingDirect, Outbounds: []NodeOutbound{}, Rules: []NodeRoute{}}
	add := func(field, kind, target string, values []string) {
		for _, raw := range values {
			if len(strings.TrimSpace(raw)) > MaxRouteRuleBytes {
				result.Blockers = append(result.Blockers, LegacyRoutingBlocker{field, "raw match value exceeds node limit"})
				return
			}
		}
		var normalized []string
		var err error
		switch kind {
		case "domain":
			normalized, err = NormalizeDomainRules(field, values)
		case "ip":
			normalized, err = NormalizeIPRules(field, values)
		case "protocol":
			normalized, err = NormalizeProtocolRules(field, values)
		}
		if err != nil {
			result.Blockers = append(result.Blockers, LegacyRoutingBlocker{field, err.Error()})
			return
		}
		if len(normalized) == 0 {
			return
		}
		if strings.HasPrefix(field, "outbound[") {
			outboundRules += len(normalized)
			for _, value := range normalized {
				outboundBytes += len(value)
			}
		}
		id := cutoverUUID(scope, "rule", field)
		enabled := true
		policy.Rules = append(policy.Rules, NodeRoute{ID: "rr_" + id.String(), Enabled: &enabled, Kind: kind, Values: normalized, OutboundID: target})
		result.Sources = append(result.Sources, LegacyRoutingSource{Field: field, Kind: kind, OutboundID: target, Values: normalized})
	}
	add("block_protocols", "protocol", RoutingBlock, legacy.BlockProtocols)
	add("block_ips", "ip", RoutingBlock, legacy.BlockIPs)
	add("block", "domain", RoutingBlock, legacy.Block)
	names := map[string]bool{}
	for i, item := range legacy.Outbound {
		field := fmt.Sprintf("outbound[%d]", i+1)
		name := strings.TrimSpace(item.Name)
		if name == "Default" || name == "block" || name == "dns_out" {
			result.Blockers = append(result.Blockers, LegacyRoutingBlocker{field, "name conflicts with an internal Xray tag"})
			continue
		}
		if name == "" || names[name] {
			result.Blockers = append(result.Blockers, LegacyRoutingBlocker{field, "outbound name is empty or duplicated"})
			continue
		}
		names[name] = true
		if err := validateLegacyOutboundConversion(item); err != nil {
			result.Blockers = append(result.Blockers, LegacyRoutingBlocker{field, err.Error()})
			continue
		}
		id := cutoverUUID(scope, "outbound", field)
		item.ID = "ob_" + id.String()
		add(field+".rules", "domain", item.ID, item.Rules)
		add(field+".ip_rules", "ip", item.ID, item.IPRules)
		add(field+".protocol_rules", "protocol", item.ID, item.ProtocolRules)
		item.Rules, item.IPRules, item.ProtocolRules = []string{}, []string{}, []string{}
		policy.Outbounds = append(policy.Outbounds, item)
	}
	if err := ValidateOutboundAggregate("outbound", len(legacy.Outbound), outboundRules, outboundBytes); err != nil {
		result.Blockers = append(result.Blockers, LegacyRoutingBlocker{"outbound", err.Error()})
	}
	if len(result.Blockers) > 0 {
		return result, nil
	}
	canonical, err := NormalizeNodeRouting(policy)
	if err != nil {
		result.Blockers = append(result.Blockers, LegacyRoutingBlocker{"routing", err.Error()})
		return result, nil
	}
	result.Draft = canonical
	return result, nil
}

func cutoverUUID(scope, kind, field string) uuid.UUID {
	sum := sha256.Sum256([]byte("ppanel:ordered-cutover:v1\x00" + scope + "\x00" + kind + "\x00" + field))
	var id uuid.UUID
	copy(id[:], sum[:16])
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return id
}

func validateLegacyOutboundConversion(o NodeOutbound) error {
	protocol := strings.ToLower(strings.TrimSpace(o.Protocol))
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
		return fmt.Errorf("TUIC outbound forwarding is not implemented by the node core")
	default:
		return fmt.Errorf("unsupported legacy protocol %q", o.Protocol)
	}
	if strings.TrimSpace(o.Plugin) != "" || HasNodePluginOptions(o.PluginOptions) {
		return fmt.Errorf("outbound plugin or plugin options are unsupported by ordered builder")
	}
	if err := ValidateNodeURI("spider_x", o.SpiderX, true); err != nil {
		return err
	}
	if err := ValidateNodeURI("path", o.Path, false); err != nil {
		return err
	}
	if o.UoT || o.UoTVersion != 0 {
		return fmt.Errorf("UDP-over-TCP is not consumed by the ordered builder")
	}
	if o.Multiplex != "" && o.Multiplex != "none" {
		return fmt.Errorf("multiplex is not supported by ordered builder")
	}
	if strings.TrimSpace(o.StreamSettings) == "" && len(o.ALPN) > 0 {
		return fmt.Errorf("ALPN changes behavior in ordered builder")
	}
	if strings.TrimSpace(o.StreamSettings) == "" && (o.XHTTPMode != "" || o.XHTTPExtra != "") {
		return fmt.Errorf("XHTTP options change behavior in ordered builder")
	}
	if strings.TrimSpace(o.Settings) == "" && ((o.Encryption != "" && o.Encryption != "none") || o.EncryptionMode != "" || o.EncryptionRTT != "" || o.EncryptionTicket != "" || o.EncryptionPadding != "" || o.EncryptionPassword != "") {
		return fmt.Errorf("encryption is unsupported without raw settings")
	}
	for _, raw := range []string{o.Settings, o.StreamSettings, o.XHTTPExtra} {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if err := rejectUnconsumedRawSemantics(raw); err != nil {
			return err
		}
	}
	if strings.TrimSpace(o.StreamSettings) != "" {
		var stream map[string]json.RawMessage
		if ValidateUniqueJSON([]byte(o.StreamSettings)) != nil || json.Unmarshal([]byte(o.StreamSettings), &stream) != nil || stream == nil || len(stream) != 0 {
			return fmt.Errorf("raw stream settings cannot be verified equivalent")
		}
	}
	if raw := strings.TrimSpace(o.Settings); raw != "" {
		var fields map[string]json.RawMessage
		if ValidateUniqueJSON([]byte(raw)) != nil || json.Unmarshal([]byte(raw), &fields) != nil || fields == nil {
			return fmt.Errorf("raw settings must be a JSON object")
		}
		if err := validateSafeRawSettings(protocol, fields); err != nil {
			return err
		}
	} else if protocol != "freedom" && protocol != "blackhole" {
		if strings.TrimSpace(o.Address) == "" || o.Port <= 0 || o.Port > 65535 {
			return fmt.Errorf("address and port are required for %s", protocol)
		}
		identity := strings.TrimSpace(o.UUID)
		if identity == "" {
			identity = strings.TrimSpace(o.Password)
		}
		switch protocol {
		case "vmess", "vless":
			if _, err := uuid.Parse(identity); err != nil {
				return fmt.Errorf("%s outbound identity is not a UUID", protocol)
			}
			if protocol == "vless" && o.Flow != "" && o.Flow != "none" && o.Flow != "xtls-rprx-vision" && o.Flow != "xtls-rprx-vision-udp443" {
				return fmt.Errorf("unsupported VLESS flow")
			}
		case "trojan", "anytls":
			if o.Password == "" {
				return fmt.Errorf("%s requires a password", protocol)
			}
		case "shadowsocks":
			if o.Password == "" {
				return fmt.Errorf("shadowsocks requires a password")
			}
			cipher := strings.ToLower(strings.TrimSpace(o.Cipher))
			if cipher == "" {
				cipher = "chacha20-ietf-poly1305"
			}
			switch cipher {
			case "aes-128-gcm", "aead_aes_128_gcm", "aes-256-gcm", "aead_aes_256_gcm", "chacha20-poly1305", "aead_chacha20_poly1305", "chacha20-ietf-poly1305", "xchacha20-poly1305", "aead_xchacha20_poly1305", "xchacha20-ietf-poly1305", "2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305":
			default:
				return fmt.Errorf("unsupported shadowsocks cipher")
			}
		}
	}
	security := strings.ToLower(strings.TrimSpace(o.Security))
	if security != "" && security != "none" && security != "tls" && security != "reality" {
		return fmt.Errorf("unsupported security %q", o.Security)
	}
	if (protocol == "vless" || protocol == "trojan") && (security == "" || security == "none") {
		address := strings.TrimSpace(o.Address)
		ip, err := netip.ParseAddr(address)
		if address != "localhost" && (err != nil || !(ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())) {
			return fmt.Errorf("public %s without transport security cannot be verified", protocol)
		}
	}
	if (security == "tls" || security == "reality") && strings.TrimSpace(o.StreamSettings) == "" {
		fingerprint := strings.ToLower(strings.TrimSpace(o.Fingerprint))
		if fingerprint == "" {
			fingerprint = "chrome"
		}
		known := map[string]bool{"chrome": true, "firefox": true, "safari": true, "ios": true, "android": true, "edge": true, "360": true, "qq": true, "random": true, "randomized": true, "randomizednoalpn": true, "unsafe": true, "hellogolang": true, "hellorandomized": true, "hellorandomizedalpn": true, "hellorandomizednoalpn": true, "hellofirefox_120": true, "hellofirefox_148": true, "hellochrome_120": true, "hellochrome_131": true, "hellochrome_133": true, "helloios_13": true, "helloios_14": true, "helloedge_106": true, "hellosafari_26_3": true, "hello360_11_0": true, "helloqq_11_1": true}
		if !known[fingerprint] || (security == "reality" && (fingerprint == "unsafe" || fingerprint == "hellogolang")) {
			return fmt.Errorf("unsupported or unverifiable TLS fingerprint")
		}
	}
	if security == "reality" && strings.TrimSpace(o.StreamSettings) == "" {
		key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(o.RealityPublicKey))
		if err != nil || len(key) != 32 {
			return fmt.Errorf("invalid REALITY public key")
		}
		short := strings.TrimSpace(o.RealityShortId)
		if len(short) > 16 {
			return fmt.Errorf("invalid REALITY short id")
		}
		if _, err := hex.DecodeString(short); err != nil {
			return fmt.Errorf("invalid REALITY short id")
		}
		if spider := strings.TrimSpace(o.SpiderX); spider != "" && !strings.HasPrefix(spider, "/") {
			return fmt.Errorf("invalid REALITY spider path")
		}
	}
	transport := strings.ToLower(strings.TrimSpace(o.Transport))
	if security == "reality" && strings.TrimSpace(o.StreamSettings) == "" {
		effective := transport
		if effective == "" {
			switch protocol {
			case "tuic":
				effective = "tuic"
			case "hysteria":
				effective = "hysteria"
			default:
				effective = "tcp"
			}
		}
		switch effective {
		case "tcp", "raw", "grpc", "xhttp", "splithttp":
		default:
			return fmt.Errorf("REALITY transport cannot be built")
		}
	}
	switch transport {
	case "", "tcp", "raw", "ws", "websocket", "grpc", "httpupgrade", "splithttp", "xhttp", "hysteria":
	case "tuic":
		return fmt.Errorf("TUIC outbound transport dialer is not implemented")
	default:
		return fmt.Errorf("unsupported transport %q", o.Transport)
	}
	if protocol == "hysteria" && transport != "" && transport != "hysteria" {
		return fmt.Errorf("Hysteria outbound requires Hysteria transport")
	}
	if protocol != "hysteria" && transport == "hysteria" {
		return fmt.Errorf("Hysteria transport requires Hysteria outbound")
	}
	return nil
}

// Inspect raw objects recursively before equivalence checks: unknown nested
// UoT/plugin semantics must not disappear inside otherwise accepted settings.
func rejectUnconsumedRawSemantics(raw string) error {
	if err := ValidateUniqueJSON([]byte(raw)); err != nil {
		return fmt.Errorf("invalid raw settings: %w", err)
	}
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

// These raw forms are the small subset whose build behavior can be verified
// without linking the Xray runtime into the panel. Complex or other-protocol
// raw objects are explicitly blocked instead of guessed to be equivalent.
func validateSafeRawSettings(protocol string, fields map[string]json.RawMessage) error {
	if len(fields) == 0 && (protocol == "freedom" || protocol == "blackhole") {
		return nil
	}
	switch protocol {
	case "freedom":
		for k, raw := range fields {
			switch k {
			case "domainStrategy", "targetStrategy":
				var v string
				if json.Unmarshal(raw, &v) != nil {
					return fmt.Errorf("invalid %s", k)
				}
				switch strings.ToLower(v) {
				case "", "asis", "useip", "useipv4", "useipv6", "useipv4v6", "useipv6v4", "forceip", "forceipv4", "forceipv6", "forceipv4v6", "forceipv6v4":
				default:
					return fmt.Errorf("unsupported %s", k)
				}
			case "userLevel", "proxyProtocol":
				var v uint32
				if json.Unmarshal(raw, &v) != nil {
					return fmt.Errorf("invalid %s", k)
				}
			default:
				return fmt.Errorf("raw freedom setting %q cannot be verified", k)
			}
		}
		return nil
	case "blackhole":
		for k, raw := range fields {
			if k != "response" {
				return fmt.Errorf("raw blackhole setting %q cannot be verified", k)
			}
			var v struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(raw, &v) != nil || (v.Type != "none" && v.Type != "http") {
				return fmt.Errorf("unsupported blackhole response")
			}
		}
		return nil
	default:
		return fmt.Errorf("raw %s settings cannot be verified equivalent", protocol)
	}
}
