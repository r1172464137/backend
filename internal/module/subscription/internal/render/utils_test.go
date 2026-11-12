package render

import (
	"reflect"
	"strings"
	"testing"

	"github.com/perfect-panel/server/internal/module/network/entity/node"
)

func TestAdapterProxy(t *testing.T) {
	servers := getServers()
	if len(servers) == 0 {
		t.Fatal("no servers found")
	}

	proxies, err := NewAdapter(tpl).Proxies(servers)
	if err != nil {
		t.Fatalf("failed to adapt servers: %v", err)
	}
	if len(proxies) != 2 {
		t.Fatalf("proxies len = %d, want 2", len(proxies))
	}
	if proxies[0].Name != "TestShadowSocks" || proxies[0].Type != "shadowsocks" {
		t.Fatalf("first proxy = %#v, want shadowsocks proxy", proxies[0])
	}
	if proxies[0].Method != "aes-256-gcm" {
		t.Fatalf("first proxy method = %q, want aes-256-gcm", proxies[0].Method)
	}
	if proxies[1].Name != "TestTrojan" || proxies[1].SNI != "tls.example.com" {
		t.Fatalf("second proxy = %#v, want trojan proxy with SNI", proxies[1])
	}
}

func TestAdapterProxyMatchesCanonicalProtocolAliases(t *testing.T) {
	srv := &node.Server{
		Id:      1,
		Name:    "AliasServer",
		Address: "example.com",
	}
	if err := srv.MarshalProtocols([]node.Protocol{
		{
			Type:        "shadowsocksr",
			Port:        8389,
			Enable:      true,
			Cipher:      "aes-256-cfb",
			ServerKey:   "server-password",
			SSRProtocol: "auth_aes128_md5",
			Obfs:        "tls1.2_ticket_auth",
			ObfsParam:   "example.com",
		},
		{
			Type:   "hysteria",
			Port:   443,
			Enable: true,
			SNI:    "tls.example.com",
		},
	}); err != nil {
		t.Fatalf("marshal protocols: %v", err)
	}

	enabled := true
	proxies, err := NewAdapter(tpl).Proxies([]*node.Node{
		{
			Id:       1,
			Name:     "SSR Alias",
			Port:     8389,
			Address:  "ssr.example.com",
			ServerId: srv.Id,
			Server:   srv,
			Protocol: "ssr",
			Enabled:  &enabled,
		},
		{
			Id:       2,
			Name:     "Hysteria Alias",
			Port:     443,
			Address:  "hy.example.com",
			ServerId: srv.Id,
			Server:   srv,
			Protocol: "hysteria2",
			Enabled:  &enabled,
		},
	})
	if err != nil {
		t.Fatalf("failed to adapt servers: %v", err)
	}
	if len(proxies) != 2 {
		t.Fatalf("proxies len = %d, want 2", len(proxies))
	}
	if proxies[0].Type != "shadowsocksr" || proxies[0].SSRProtocol != "auth_aes128_md5" || proxies[0].ServerKey != "server-password" {
		t.Fatalf("first proxy = %#v, want canonical shadowsocksr fields", proxies[0])
	}
	if proxies[1].Type != "hysteria" || proxies[1].SNI != "tls.example.com" {
		t.Fatalf("second proxy = %#v, want canonical hysteria fields", proxies[1])
	}
}

func getServers() []*node.Node {
	srv := &node.Server{
		Id:      1,
		Name:    "TestServer",
		Address: "example.com",
	}
	if err := srv.MarshalProtocols([]node.Protocol{
		{
			Type:   "shadowsocks",
			Port:   1234,
			Enable: true,
			Cipher: "aes-256-gcm",
		},
		{
			Type:      "trojan",
			Port:      443,
			Enable:    true,
			Security:  "tls",
			SNI:       "tls.example.com",
			Transport: "tcp",
		},
	}); err != nil {
		panic(err)
	}

	enabled := true
	return []*node.Node{
		{
			Id:       1,
			Name:     "TestShadowSocks",
			Tags:     "stable,asia",
			Port:     1234,
			Address:  "ss.example.com",
			ServerId: srv.Id,
			Server:   srv,
			Protocol: "shadowsocks",
			Enabled:  &enabled,
			Sort:     1,
		},
		{
			Id:       2,
			Name:     "TestTrojan",
			Tags:     "tls",
			Port:     443,
			Address:  "trojan.example.com",
			ServerId: srv.Id,
			Server:   srv,
			Protocol: "trojan",
			Enabled:  &enabled,
			Sort:     2,
		},
	}
}

func TestAdapterProxyCertPinForcesCertVerification(t *testing.T) {
	const pin = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	srv := &node.Server{Id: 1, Name: "PinServer", Address: "example.com"}
	if err := srv.MarshalProtocols([]node.Protocol{
		{
			Type: "vmess", Port: 443, Enable: true, Security: "tls",
			SNI: "pin.example.com", CertMode: "self",
			AllowInsecure: true, CertPinSHA256: pin,
		},
		{
			Type: "trojan", Port: 444, Enable: true, Security: "tls",
			SNI: "pin.example.com", CertMode: "http",
			AllowInsecure: true,
		},
	}); err != nil {
		t.Fatalf("marshal protocols: %v", err)
	}

	enabled := true
	nodes := []*node.Node{
		{Id: 1, Name: "Pinned", Port: 443, Address: "a.example.com", ServerId: srv.Id, Server: srv, Protocol: "vmess", Enabled: &enabled},
		{Id: 2, Name: "Unpinned", Port: 444, Address: "b.example.com", ServerId: srv.Id, Server: srv, Protocol: "trojan", Enabled: &enabled},
	}
	proxies, err := NewAdapter(tpl).Proxies(nodes)
	if err != nil {
		t.Fatalf("proxies: %v", err)
	}
	if len(proxies) != 2 {
		t.Fatalf("len(proxies) = %d, want 2", len(proxies))
	}
	pinned, unpinned := proxies[0], proxies[1]
	if pinned.CertPinSHA256 != pin {
		t.Fatalf("pinned CertPinSHA256 = %q, want %q", pinned.CertPinSHA256, pin)
	}
	if pinned.AllowInsecure {
		t.Fatal("pinned proxy AllowInsecure = true, want false (pin must force cert verification)")
	}
	if !unpinned.AllowInsecure {
		t.Fatal("unpinned proxy AllowInsecure = false, want true (no pin, keep configured value)")
	}
}

func TestAdapterPreservesNowhereContractAndUserSharedKey(t *testing.T) {
	const pin = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	const sharedKey = "550e8400-e29b-41d4-a716-446655440000"
	srv := &node.Server{Id: 1, Name: "NowhereServer", Address: "example.com"}
	if err := srv.MarshalProtocols([]node.Protocol{{
		Type: "nowhere", Port: 443, Version: 1, Enable: true, Security: "tls", Network: "mix",
		SNI: "nowhere.example", ALPN: []string{"now/1"}, CertMode: "self", CertPinSHA256: pin,
	}}); err != nil {
		t.Fatalf("marshal protocols: %v", err)
	}

	enabled := true
	client, err := NewAdapter(
		`{{ (index .Proxies 0).Type }}|{{ (index .Proxies 0).Network }}|{{ .UserInfo.Password }}`,
		WithServers([]*node.Node{{
			Id: 1, Name: "Nowhere", Port: 443, Address: "nowhere.example", ServerId: srv.Id,
			Server: srv, Protocol: "nowhere", Enabled: &enabled,
		}}),
		WithUserInfo(User{Password: sharedKey}),
		WithOutputFormat("text"),
	).Client()
	if err != nil {
		t.Fatalf("Client() error = %v", err)
	}
	if len(client.Proxies) != 1 {
		t.Fatalf("len(Proxies) = %d, want 1", len(client.Proxies))
	}
	proxy := client.Proxies[0]
	if proxy.Type != "nowhere" || proxy.Version != 1 || proxy.Network != "mix" || proxy.Security != "tls" ||
		proxy.SNI != "nowhere.example" || len(proxy.ALPN) != 1 || proxy.ALPN[0] != "now/1" ||
		proxy.CertPinSHA256 != pin {
		t.Fatalf("Nowhere proxy fields were not preserved: %#v", proxy)
	}
	built, err := client.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got, want := string(built), "nowhere|mix|"+sharedKey; got != want {
		t.Fatalf("Build() = %q, want %q", got, want)
	}
}

// CONTRACT 5.5: server-only settings never reach the template view, so
// neither field access nor toJson can leak them.
func TestProxyCarriesNoServerOnlyFields(t *testing.T) {
	typ := reflect.TypeOf(Proxy{})
	for _, name := range []string{"RealityServerAddr", "RealityServerPort", "RealityPrivateKey", "AcceptProxyProtocol",
		"CertMode", "CertDNSProvider", "CertDNSEnv", "EncryptionTicket", "EncryptionPrivateKey", "EncryptionServerPadding"} {
		if _, ok := typ.FieldByName(name); ok {
			t.Errorf("render.Proxy exposes server-only field %s", name)
		}
	}
	srv := &node.Server{Id: 1, Name: "S", Address: "example.com"}
	if err := srv.MarshalProtocols([]node.Protocol{{
		Type: "vless", Port: 443, Enable: true, Security: "reality", SNI: "www.example.com",
		RealityServerAddr: "secret-target.example", RealityServerPort: 8443, RealityPrivateKey: "PRIVATE-KEY-VALUE",
		RealityPublicKey: "PUBLIC", RealityShortId: "ab", CertDNSEnv: "CF_TOKEN=SECRET-ENV",
	}}); err != nil {
		t.Fatal(err)
	}
	enabled := true
	client, err := NewAdapter(`{{ range .Proxies }}{{ toJson . }}{{ end }}`,
		WithServers([]*node.Node{{Id: 1, Name: "N", Port: 443, Address: "example.com", ServerId: 1, Server: srv, Protocol: "vless", Enabled: &enabled}}),
		WithOutputFormat("conf"),
	).Client()
	if err != nil {
		t.Fatal(err)
	}
	out, err := client.Build()
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"secret-target.example", "8443", "PRIVATE-KEY-VALUE", "SECRET-ENV"} {
		if strings.Contains(string(out), secret) {
			t.Errorf("rendered template leaks %q: %s", secret, out)
		}
	}
	if !strings.Contains(string(out), "PUBLIC") {
		t.Errorf("client field missing from template: %s", out)
	}
}
