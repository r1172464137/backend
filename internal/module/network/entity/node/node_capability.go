package node

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/perfect-panel/server/internal/config"
)

// ValidateNodeProtocols inventories every enabled stored protocol before the
// distribution sanitizer can silently omit it. The allowlist is pinned to the
// same-batch ppnode builder, not the panel's broader subscription formats.
func ValidateNodeProtocols(server *Server) error {
	if server == nil {
		return fmt.Errorf("missing server")
	}
	var protocols []Protocol
	if server.Protocols != "" {
		if strings.TrimSpace(server.Protocols) == "null" {
			return fmt.Errorf("protocol inventory must be an array")
		}
		if err := config.ValidateUniqueJSON([]byte(server.Protocols)); err != nil {
			return fmt.Errorf("invalid protocols JSON: %w", err)
		}
		decoder := json.NewDecoder(strings.NewReader(server.Protocols))
		decoder.UseNumber()
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&protocols); err != nil {
			return fmt.Errorf("invalid protocols JSON: %w", err)
		}
		var entries []map[string]json.RawMessage
		if err := json.Unmarshal([]byte(server.Protocols), &entries); err != nil {
			return fmt.Errorf("invalid protocol inventory")
		}
		for i, entry := range entries {
			var enabled *bool
			raw, ok := entry["enable"]
			if !ok || json.Unmarshal(raw, &enabled) != nil || enabled == nil {
				return fmt.Errorf("protocol[%d] requires explicit enable", i+1)
			}
		}
	}
	seenType, seenTCP, seenUDP := map[string]bool{}, map[uint16]bool{}, map[uint16]bool{}
	var blockers []string
	for i, raw := range protocols {
		if !raw.Enable {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(raw.Type))
		if name == "hysteria2" {
			name = "hysteria"
		}
		switch name {
		case "vless", "vmess", "trojan", "shadowsocks", "hysteria", "tuic", "anytls":
		default:
			blockers = append(blockers, fmt.Sprintf("protocol[%d] %q is enabled but unsupported by ppnode", i+1, raw.Type))
			continue
		}
		if seenType[name] {
			blockers = append(blockers, fmt.Sprintf("protocol[%d] duplicates enabled protocol %q", i+1, name))
		}
		seenType[name] = true
		if raw.Port == 0 {
			blockers = append(blockers, fmt.Sprintf("protocol[%d] requires a nonzero port", i+1))
		} else {
			usesTCP, usesUDP := true, false
			switch name {
			case "hysteria", "tuic":
				usesTCP, usesUDP = false, true
			case "shadowsocks":
				usesUDP = true
			}
			if (usesTCP && seenTCP[raw.Port]) || (usesUDP && seenUDP[raw.Port]) {
				blockers = append(blockers, fmt.Sprintf("protocol[%d] conflicts with an enabled listener port", i+1))
			}
			if usesTCP {
				seenTCP[raw.Port] = true
			}
			if usesUDP {
				seenUDP[raw.Port] = true
			}
		}
		if err := validateEnabledNodeProtocol(raw, name); err != nil {
			blockers = append(blockers, fmt.Sprintf("protocol[%d]: %v", i+1, err))
		}
	}
	if len(blockers) > 0 {
		return fmt.Errorf("%s", strings.Join(blockers, "; "))
	}
	return nil
}

func validateEnabledNodeProtocol(raw Protocol, name string) error {
	if err := config.ValidateNodeURI("path", raw.Path, false); err != nil {
		return err
	}
	if err := config.ValidateNodeURI("obfs_path", raw.ObfsPath, false); err != nil {
		return err
	}
	if raw.UoT || raw.UoTVersion != 0 {
		return fmt.Errorf("inbound UDP-over-TCP is not consumed by ppnode")
	}
	if !config.ValidXHTTPMode(raw.XhttpMode) {
		return fmt.Errorf("unsupported XHTTP mode")
	}
	if raw.XhttpExtra != "" {
		if err := config.ValidateXHTTPExtra(raw.XhttpExtra, raw.XhttpMode); err != nil {
			return err
		}
	}
	if name == "shadowsocks" {
		plugin := strings.ToLower(strings.TrimSpace(raw.Plugin))
		switch plugin {
		case "", "none", "false", "off", "disabled":
		default:
			return fmt.Errorf("shadowsocks inbound plugin is unsupported")
		}
		if config.HasNodePluginOptions(raw.PluginOptions) {
			return fmt.Errorf("shadowsocks plugin options are unsupported")
		}
		if !supportedInboundCipher(raw.Cipher) {
			return fmt.Errorf("shadowsocks cipher is unsupported")
		}
		if strings.Contains(strings.ToLower(raw.Cipher), "2022") {
			length := 32
			if strings.EqualFold(strings.TrimSpace(raw.Cipher), "2022-blake3-aes-128-gcm") {
				length = 16
			}
			if len(raw.ServerKey) != length {
				return fmt.Errorf("shadowsocks 2022 server key has invalid length")
			}
		}
	}
	normalized, err := NormalizeProtocolForStorage(raw)
	if err != nil {
		return fmt.Errorf("cannot be distributed: %w", err)
	}
	projected := SanitizeProtocolsForNodeDistribution([]Protocol{normalized})
	if len(projected) != 1 {
		return fmt.Errorf("would be omitted by node distribution")
	}
	p := projected[0]
	if name == "vless" || name == "vmess" || name == "trojan" {
		switch p.Security {
		case "", "none", "tls", "reality":
		default:
			return fmt.Errorf("unsupported listener security")
		}
		switch p.Transport {
		case "tcp", "ws", "websocket", "grpc", "httpupgrade", "splithttp", "xhttp":
		default:
			return fmt.Errorf("unsupported or missing transport")
		}
	}
	if p.Security == "reality" {
		key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(p.RealityPrivateKey))
		if err != nil || len(key) != 32 {
			return fmt.Errorf("invalid REALITY private key")
		}
		short := strings.TrimSpace(p.RealityShortId)
		if short == "" || len(short) > 16 {
			return fmt.Errorf("invalid REALITY short id")
		}
		if _, err := hex.DecodeString(short); err != nil {
			return fmt.Errorf("invalid REALITY short id")
		}
		if strings.TrimSpace(p.SNI) == "" || p.RealityServerPort <= 0 || p.RealityServerPort > 65535 {
			return fmt.Errorf("incomplete REALITY target")
		}
		host := p.RealityServerAddr
		if host == "" {
			host = p.SNI
		}
		if _, _, err := net.SplitHostPort(fmt.Sprintf("%s:%d", host, p.RealityServerPort)); err != nil {
			return fmt.Errorf("invalid REALITY target")
		}
		if p.RealityXver < 0 || p.RealityXver > 2 {
			return fmt.Errorf("invalid REALITY xver")
		}
		switch strings.ToLower(p.Transport) {
		case "tcp", "grpc", "xhttp", "splithttp":
		default:
			if name != "anytls" {
				return fmt.Errorf("REALITY transport is unsupported")
			}
		}
	}
	if name == "vless" && p.Encryption != "" && p.Encryption != "none" {
		if p.Encryption != "mlkem768x25519plus" {
			return fmt.Errorf("unsupported VLESS encryption")
		}
		switch p.EncryptionMode {
		case "native", "xorpub", "random":
		default:
			return fmt.Errorf("invalid VLESS encryption mode")
		}
		if strings.TrimSpace(p.EncryptionTicket) == "" || strings.TrimSpace(p.EncryptionPrivateKey) == "" {
			return fmt.Errorf("incomplete VLESS encryption")
		}
		// The Xray decryption builder consumes a numeric lifetime and 32/64
		// byte base64url keys. Unverifiable noncanonical forms fail closed.
		ticket := strings.TrimSuffix(strings.TrimSpace(p.EncryptionTicket), "s")
		parts := strings.Split(ticket, "-")
		if len(parts) < 1 || len(parts) > 2 {
			return fmt.Errorf("invalid VLESS encryption ticket")
		}
		for _, part := range parts {
			if part == "" {
				return fmt.Errorf("invalid VLESS encryption ticket")
			}
			if _, err := strconv.ParseUint(part, 10, 31); err != nil {
				return fmt.Errorf("invalid VLESS encryption ticket")
			}
		}
		for _, part := range strings.Split(p.EncryptionPrivateKey, ".") {
			key, err := base64.RawURLEncoding.DecodeString(part)
			if err != nil || (len(key) != 32 && len(key) != 64) {
				return fmt.Errorf("invalid VLESS encryption private key")
			}
		}
		for _, part := range strings.Split(p.EncryptionServerPadding, ".") {
			if len(part) >= 20 {
				key, err := base64.RawURLEncoding.DecodeString(part)
				if err != nil || (len(key) != 32 && len(key) != 64) {
					return fmt.Errorf("invalid VLESS encryption padding")
				}
			}
		}
	}
	if p.Security == "tls" || name == "hysteria" || name == "tuic" {
		switch p.CertMode {
		case "file", "self", "dns", "http":
		default:
			return fmt.Errorf("unsupported node TLS certificate mode")
		}
	}
	if name == "hysteria" && (p.DownMbps < 0 || p.UpMbps < 0) {
		return fmt.Errorf("invalid Hysteria bandwidth")
	}
	if name == "hysteria" && p.DownMbps > 0 && p.UpMbps <= 0 {
		return fmt.Errorf("Hysteria down bandwidth requires up bandwidth")
	}
	if name == "hysteria" && p.Obfs != "" && p.ObfsPassword != "" {
		if p.Obfs != "salamander" {
			return fmt.Errorf("unsupported Hysteria obfuscation")
		}
		if !json.Valid([]byte(fmt.Sprintf(`{"password":"%s"}`, p.ObfsPassword))) {
			return fmt.Errorf("invalid Hysteria obfuscation password")
		}
	}
	if name == "hysteria" || name == "tuic" {
		mode := strings.ToLower(strings.TrimSpace(p.CertMode))
		if mode == "" || mode == "none" {
			return fmt.Errorf("requires a node TLS certificate")
		}
	}
	return nil
}

func supportedInboundCipher(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "aes-128-gcm", "aead_aes_128_gcm", "aes-256-gcm", "aead_aes_256_gcm", "chacha20-poly1305", "aead_chacha20_poly1305", "chacha20-ietf-poly1305", "xchacha20-poly1305", "aead_xchacha20_poly1305", "xchacha20-ietf-poly1305", "2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305":
		return true
	default:
		return false
	}
}
