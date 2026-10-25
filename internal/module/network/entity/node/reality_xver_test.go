package node

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestRealityXverStorageAndDistribution(t *testing.T) {
	for _, xver := range []int{0, 1, 2} {
		t.Run(fmt.Sprint(xver), func(t *testing.T) {
			for _, kind := range []string{"vless", "vmess", "trojan", "anytls"} {
				p, err := NormalizeProtocolForStorage(Protocol{Type: kind, Port: 20100, Enable: true,
					Security: "reality", Transport: "tcp", SNI: "example.com", RealityServerAddr: "127.0.0.1",
					RealityServerPort: 8443, RealityPrivateKey: "key", RealityShortId: "0123abcd",
					RealityXver: xver, AcceptProxyProtocol: true})
				if err != nil {
					t.Fatal(err)
				}
				var server Server
				if err := server.MarshalProtocols([]Protocol{p}); err != nil {
					t.Fatal(err)
				}
				if (xver != 0) != strings.Contains(server.Protocols, `"reality_xver"`) {
					t.Fatalf("stored JSON = %s", server.Protocols)
				}
				stored, err := server.UnmarshalProtocols()
				if err != nil {
					t.Fatal(err)
				}
				distributed := SanitizeProtocolsForNodeDistribution(stored)
				if len(distributed) != 1 || distributed[0].RealityXver != xver || !distributed[0].AcceptProxyProtocol || distributed[0].Port != 20100 || distributed[0].RealityServerPort != 8443 {
					t.Fatalf("distribution = %+v", distributed)
				}
			}
		})
	}
	var old Protocol
	if err := json.Unmarshal([]byte(`{"type":"vless","security":"reality","accept_proxy_protocol":true}`), &old); err != nil {
		t.Fatal(err)
	}
	if old.RealityXver != 0 {
		t.Fatal("omitted xver must default to zero, not inferred from reception")
	}
}

func TestRealityXverValidationAndCleanup(t *testing.T) {
	for _, xver := range []int{-1, 3, 256} {
		// Invalid REALITY settings are rejected even when disabled.
		if _, err := NormalizeProtocolForStorage(Protocol{Type: "vless", Security: "reality", RealityXver: xver}); err == nil {
			t.Fatalf("accepted %d", xver)
		}
	}
	for _, raw := range []string{`1.5`, `"2"`, `true`, `18446744073709551616`} {
		var p Protocol
		if err := json.Unmarshal([]byte(`{"reality_xver":`+raw+`}`), &p); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, security := range []string{"", "none", "tls"} {
		p, err := NormalizeProtocolForStorage(Protocol{Type: "vless", Security: security, RealityXver: 2, AcceptProxyProtocol: true})
		if err != nil {
			t.Fatal(err)
		}
		if p.RealityXver != 0 || !p.AcceptProxyProtocol {
			t.Fatalf("cleanup = %+v", p)
		}
	}
	if _, err := NormalizeProtocolForStorage(Protocol{Type: "nowhere", Enable: true, RealityXver: 1}); err == nil {
		t.Fatal("Nowhere must reject unrelated options before cleanup")
	}
}
