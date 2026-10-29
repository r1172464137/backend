package bootstrap

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/perfect-panel/server/internal/module/network/entity/node"
)

func TestCutoverInventoryRejectsUnsupportedEnabledListeners(t *testing.T) {
	for name, protocol := range map[string]node.Protocol{
		"unsupported-nowhere": {Type: "nowhere", Port: 443, Enable: true},
		"unsupported-naive":   {Type: "naive", Port: 443, Enable: true},
		"unsupported-ssr":     {Type: "shadowsocksr", Port: 443, Enable: true},
		"ss-plugin":           {Type: "shadowsocks", Port: 443, Enable: true, Cipher: "aes-128-gcm", Plugin: "obfs"},
		"ss-plugin-options":   {Type: "shadowsocks", Port: 443, Enable: true, Cipher: "aes-128-gcm", PluginOptions: map[string]any{"mode": "http"}},
		"ss-uot":              {Type: "shadowsocks", Port: 443, Enable: true, Cipher: "aes-128-gcm", UoT: true},
		"ss-uot-version":      {Type: "shadowsocks", Port: 443, Enable: true, Cipher: "aes-128-gcm", UoTVersion: 2},
		"vless-uot-version":   {Type: "vless", Port: 443, Enable: true, Transport: "tcp", UoTVersion: 2},
		"malformed-path":      {Type: "vless", Port: 443, Enable: true, Transport: "ws", Path: "/%"},
		"bad-reality":         {Type: "vless", Port: 443, Enable: true, Transport: "tcp", Security: "reality", SNI: "example.com", RealityServerPort: 443, RealityPrivateKey: "bad", RealityShortId: "aa"},
	} {
		t.Run(name, func(t *testing.T) {
			body, err := json.Marshal([]node.Protocol{protocol})
			if err != nil {
				t.Fatal(err)
			}
			if err := node.ValidateNodeProtocols(&node.Server{Id: 1, Protocols: string(body)}); err == nil {
				t.Fatal("unsupported listener accepted")
			}
		})
	}
	for _, protocol := range []node.Protocol{{Type: "vless", Port: 443, Enable: true, Transport: "tcp"}, {Type: "vless", Port: 443, Enable: true, Transport: "tcp", Security: "reality", SNI: "example.com", RealityServerAddr: "example.com", RealityServerPort: 443, RealityPrivateKey: base64.RawURLEncoding.EncodeToString(make([]byte, 32)), RealityShortId: "0123456789abcdef", RealityXver: 1}, {Type: "shadowsocks", Port: 8443, Enable: true, Cipher: "aes-128-gcm"}, {Type: "nowhere", Port: 5555, Enable: false}} {
		body, _ := json.Marshal([]node.Protocol{protocol})
		if err := node.ValidateNodeProtocols(&node.Server{Protocols: string(body)}); err != nil {
			t.Errorf("ordinary %s blocked: %v", protocol.Type, err)
		}
	}
}
func TestCutoverInventoryBlocksEnabledBeforeDDL(t *testing.T) {
	store := cutoverFakeStore{system: cutoverFakeSystem{}, nodes: cutoverFakeNodes{overrides: map[int64]*node.ServerConfigOverride{}, protocols: map[int64]string{3: `[{"type":"naive","port":443,"enable":true}]`}}}
	_, err := preflightOrderedCutover(t.Context(), store, false)
	if err == nil || !strings.Contains(err.Error(), "server 3 protocol preflight blocked") {
		t.Fatalf("error %v", err)
	}
}
