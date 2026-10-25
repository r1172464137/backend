package serverapi

import (
	"testing"

	"github.com/perfect-panel/server/internal/module/network/entity/node"
)

func TestRealityXverLegacyConfig(t *testing.T) {
	for _, kind := range []string{Vless, Vmess, Trojan, AnyTLS} {
		for _, xver := range []int{0, 1, 2} {
			config := compatible(node.Protocol{Type: kind, Security: "reality", RealityXver: xver})
			security := config["security_config"].(map[string]interface{})
			got, present := security["reality_xver"]
			if xver == 0 {
				if present {
					t.Fatalf("zero should preserve old JSON shape: %v", security)
				}
			} else if got != xver {
				t.Fatalf("%s: xver = %v, want %d", kind, got, xver)
			}
		}
		config := compatible(node.Protocol{Type: kind, Security: "tls", RealityXver: 2})
		if _, present := config["security_config"].(map[string]interface{})["reality_xver"]; present {
			t.Fatalf("non-REALITY legacy shape changed: %v", config)
		}
	}
}
