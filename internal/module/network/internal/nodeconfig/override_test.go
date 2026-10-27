package nodeconfig

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
)

func TestOverrideModelAndApplyOverride(t *testing.T) {
	global := GlobalValues(config.NodeConfig{
		IPStrategy: "prefer_ipv4",
		DNS: []config.NodeDNS{
			{Proto: "udp", Address: "8.8.8.8:53", Domains: []string{"geosite:google"}},
		},
		Block: []string{"geosite:ads"},
		Outbound: []config.NodeOutbound{
			{Name: "global", Protocol: "direct", Rules: []string{"geosite:cn"}},
		},
	})

	override, allInherited, err := OverrideModel(1, dto.ServerNodeConfigOverride{
		InheritIPStrategy: false,
		IPStrategy:        "prefer_ipv6",
		InheritDNS:        false,
		DNS:               []dto.NodeDNS{},
		InheritBlock:      true,
		InheritOutbound:   false,
		Outbound: []dto.NodeOutbound{
			{Name: "node", Protocol: "reject", Rules: []string{"geosite:private"}},
		},
	})
	if err != nil {
		t.Fatalf("OverrideModel() error = %v", err)
	}
	if allInherited {
		t.Fatal("OverrideModel() allInherited = true, want false")
	}
	override.Id = 1

	effective := CloneValues(global)
	if err := ApplyOverride(&effective, override); err != nil {
		t.Fatalf("ApplyOverride() error = %v", err)
	}
	if effective.IPStrategy != "prefer_ipv6" {
		t.Fatalf("IPStrategy = %q, want prefer_ipv6", effective.IPStrategy)
	}
	if len(effective.DNS) != 0 {
		t.Fatalf("DNS len = %d, want 0", len(effective.DNS))
	}
	if len(effective.Block) != 1 || effective.Block[0] != "geosite:ads" {
		t.Fatalf("Block = %#v, want inherited global block", effective.Block)
	}
	if len(effective.Outbound) != 1 || effective.Outbound[0].Name != "node" {
		t.Fatalf("Outbound = %#v, want node override", effective.Outbound)
	}
}

func TestOverrideModelAllInherited(t *testing.T) {
	_, allInherited, err := OverrideModel(1, dto.ServerNodeConfigOverride{
		InheritIPStrategy: true,
		InheritDNS:        true,
		InheritBlock:      true,
		InheritOutbound:   true,
	})
	if err != nil {
		t.Fatalf("OverrideModel() error = %v", err)
	}
	if !allInherited {
		t.Fatal("OverrideModel() allInherited = false, want true")
	}
}

func TestSanitizeNodeConfigValues(t *testing.T) {
	global := GlobalValues(config.NodeConfig{
		IPStrategy: "prefer_ipv4",
		DNS: []config.NodeDNS{
			{Proto: " udp ", Address: " 8.8.8.8:53 ", ServerName: " dns.google ", Domains: []string{" geosite:google ", "", "geosite:google"}},
			{Proto: "", Address: "1.1.1.1:53", Domains: []string{"geosite:cloudflare"}},
		},
		Block: []string{" geosite:ads ", "", "geosite:ads", "   "},
		Outbound: []config.NodeOutbound{
			{
				Name:             " node ",
				Protocol:         " socks ",
				Address:          " 127.0.0.1 ",
				User:             " user ",
				Transport:        " websocket ",
				Host:             " example.com ",
				Plugin:           " v2ray-plugin ",
				PluginOptions:    map[string]any{"mode": "websocket"},
				ALPN:             []string{" h2 ", "", "h2"},
				XHTTPMode:        " packet-up ",
				Encryption:       " mlkem768x25519plus ",
				EncryptionMode:   " native ",
				Multiplex:        " medium ",
				Settings:         " {\"address\":\"127.0.0.1\",\"port\":1080} ",
				StreamSettings:   " {\"network\":\"tcp\"} ",
				RealityPublicKey: " public-key ",
				Rules:            []string{" geosite:private ", "", "geosite:private"},
			},
			{Name: "empty-rules", Protocol: "direct", Rules: []string{" "}},
			{Name: "", Protocol: "direct", Rules: []string{"geosite:cn"}},
		},
	})

	if len(global.DNS) != 1 {
		t.Fatalf("DNS len = %d, want 1", len(global.DNS))
	}
	if global.DNS[0].Proto != "udp" || global.DNS[0].Address != "8.8.8.8:53" {
		t.Fatalf("DNS = %#v, want trimmed valid DNS", global.DNS[0])
	}
	if global.DNS[0].ServerName != "dns.google" {
		t.Fatalf("DNS server name = %q, want dns.google", global.DNS[0].ServerName)
	}
	if len(global.DNS[0].Domains) != 1 || global.DNS[0].Domains[0] != "geosite:google" {
		t.Fatalf("DNS domains = %#v, want sanitized domains", global.DNS[0].Domains)
	}
	if len(global.Block) != 1 || global.Block[0] != "geosite:ads" {
		t.Fatalf("Block = %#v, want sanitized block rules", global.Block)
	}
	if len(global.Outbound) != 2 {
		t.Fatalf("Outbound len = %d, want 2", len(global.Outbound))
	}
	if global.Outbound[0].Name != "node" || global.Outbound[0].Protocol != "socks" {
		t.Fatalf("Outbound = %#v, want trimmed outbound", global.Outbound[0])
	}
	if global.Outbound[0].User != "user" || global.Outbound[0].Transport != "websocket" || global.Outbound[0].Host != "example.com" {
		t.Fatalf("Outbound extra fields = %#v, want trimmed extra fields", global.Outbound[0])
	}
	if global.Outbound[0].Plugin != "v2ray-plugin" || global.Outbound[0].XHTTPMode != "packet-up" ||
		global.Outbound[0].Encryption != "mlkem768x25519plus" || global.Outbound[0].EncryptionMode != "native" ||
		global.Outbound[0].Multiplex != "medium" {
		t.Fatalf("Outbound extended fields = %#v, want preserved fields", global.Outbound[0])
	}
	if len(global.Outbound[0].ALPN) != 1 || global.Outbound[0].ALPN[0] != "h2" {
		t.Fatalf("Outbound ALPN = %#v, want [h2]", global.Outbound[0].ALPN)
	}
	if global.Outbound[0].Settings != "{\"address\":\"127.0.0.1\",\"port\":1080}" || global.Outbound[0].StreamSettings != "{\"network\":\"tcp\"}" {
		t.Fatalf("Outbound raw JSON fields = %#v, want trimmed raw JSON fields", global.Outbound[0])
	}
	if len(global.Outbound[0].Rules) != 1 || global.Outbound[0].Rules[0] != "geosite:private" {
		t.Fatalf("Outbound rules = %#v, want sanitized rules", global.Outbound[0].Rules)
	}
	if global.Outbound[1].Name != "empty-rules" || len(global.Outbound[1].Rules) != 0 {
		t.Fatalf("Outbound without rules = %#v, want preserved with empty rules", global.Outbound[1])
	}
}

func TestOverrideModelPreservesOutboundWithoutRules(t *testing.T) {
	override, allInherited, err := OverrideModel(1, dto.ServerNodeConfigOverride{
		InheritIPStrategy: true,
		InheritDNS:        true,
		InheritBlock:      true,
		InheritOutbound:   false,
		Outbound: []dto.NodeOutbound{
			{
				Name:     " warp ",
				Protocol: " socks ",
				Address:  " 127.0.0.1 ",
				Port:     1080,
				Rules:    []string{},
			},
		},
	})
	if err != nil {
		t.Fatalf("OverrideModel() error = %v", err)
	}
	if allInherited {
		t.Fatal("OverrideModel() allInherited = true, want false")
	}
	if override.Outbound == nil {
		t.Fatal("OverrideModel() Outbound = nil, want override value")
	}
	override.Id = 1

	resp, err := OverrideResponse(override)
	if err != nil {
		t.Fatalf("OverrideResponse() error = %v", err)
	}
	if resp.InheritOutbound {
		t.Fatal("OverrideResponse() InheritOutbound = true, want false")
	}
	if len(resp.Outbound) != 1 {
		t.Fatalf("OverrideResponse() Outbound len = %d, want 1", len(resp.Outbound))
	}
	got := resp.Outbound[0]
	if got.Name != "warp" || got.Protocol != "socks" || got.Address != "127.0.0.1" || got.Port != 1080 {
		t.Fatalf("OverrideResponse() Outbound = %#v, want preserved trimmed warp outbound", got)
	}
	if len(got.Rules) != 0 {
		t.Fatalf("OverrideResponse() Outbound rules = %#v, want empty rules", got.Rules)
	}
}

func TestLegacyDomainOverrideMakesNewBlockCategoriesExplicitlyEmpty(t *testing.T) {
	legacyBlock := `["geosite:private"]`
	override := &node.ServerConfigOverride{Id: 1, ServerId: 7, Block: &legacyBlock}

	resp, err := OverrideResponse(override)
	if err != nil {
		t.Fatalf("OverrideResponse() error = %v", err)
	}
	if resp.InheritBlock || len(resp.Block) != 1 || len(resp.BlockIPs) != 0 || len(resp.BlockProtocols) != 0 {
		t.Fatalf("legacy override response = %#v, want domain override and empty new categories", resp)
	}

	effective := serverNodeConfigValuesForTest()
	if err := ApplyOverride(&effective, override); err != nil {
		t.Fatalf("ApplyOverride() error = %v", err)
	}
	if len(effective.Block) != 1 || len(effective.BlockIPs) != 0 || len(effective.BlockProtocols) != 0 {
		t.Fatalf("effective blocks = %#v/%#v/%#v, want legacy domain plus explicit empty categories", effective.Block, effective.BlockIPs, effective.BlockProtocols)
	}
}

func serverNodeConfigValuesForTest() dto.ServerNodeConfigValues {
	return dto.ServerNodeConfigValues{
		Block:          []string{"geosite:global"},
		BlockIPs:       []string{"geoip:private"},
		BlockProtocols: []string{"bittorrent"},
	}
}

func TestOverrideModelStoresAllBlockCategoriesAndCanonicalizes(t *testing.T) {
	override, allInherited, err := OverrideModel(9, dto.ServerNodeConfigOverride{
		InheritIPStrategy: true,
		InheritDNS:        true,
		InheritBlock:      false,
		Block:             []string{"example.com", `regex:(^|\.)tracker\.`},
		BlockIPs:          []string{"10.1.2.3/8"},
		BlockProtocols:    []string{"BitTorrent"},
		InheritOutbound:   true,
	})
	if err != nil {
		t.Fatalf("OverrideModel() error = %v", err)
	}
	if allInherited || override.Block == nil || override.BlockIPs == nil || override.BlockProtocols == nil {
		t.Fatalf("override = %#v, allInherited = %v; want all block columns stored", override, allInherited)
	}
	override.Id = 1
	resp, err := OverrideResponse(override)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resp.Block, []string{"full:example.com", `regexp:(^|\.)tracker\.`}) ||
		!reflect.DeepEqual(resp.BlockIPs, []string{"10.0.0.0/8"}) ||
		!reflect.DeepEqual(resp.BlockProtocols, []string{"bittorrent"}) {
		t.Fatalf("canonical block response = %#v", resp)
	}
}

func TestMergeRouteRuleOmissionsAndExplicitEmpty(t *testing.T) {
	existing := dto.ServerNodeConfigOverride{
		InheritBlock:    false,
		BlockIPs:        []string{"geoip:private"},
		BlockProtocols:  []string{"bittorrent"},
		InheritOutbound: false,
		Outbound: []dto.NodeOutbound{
			{Name: "first", IPRules: []string{"geoip:cn"}, ProtocolRules: []string{"bittorrent"}},
			{Name: "second", IPRules: []string{"192.0.2.0/24"}, ProtocolRules: []string{"bittorrent"}},
		},
	}

	var oldClient dto.UpdateServerNodeConfigRequest
	if err := json.Unmarshal([]byte(`{
		"server_id":1,"inherit_block":false,"block":["example.com"],
		"inherit_outbound":false,"outbound":[
			{"name":"second","protocol":"direct","rules":["geosite:cn"]},
			{"name":"first","protocol":"direct","rules":[]}
		]
	}`), &oldClient); err != nil {
		t.Fatal(err)
	}
	if err := MergeRouteRuleOmissions(&oldClient, existing, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(oldClient.BlockIPs, existing.BlockIPs) || !reflect.DeepEqual(oldClient.BlockProtocols, existing.BlockProtocols) {
		t.Fatalf("old client block omission cleared hidden rules: %#v", oldClient.ServerNodeConfigOverride)
	}
	if !reflect.DeepEqual(oldClient.Outbound[0].IPRules, []string{"192.0.2.0/24"}) ||
		!reflect.DeepEqual(oldClient.Outbound[1].IPRules, []string{"geoip:cn"}) {
		t.Fatalf("reordered outbound omission did not match by name: %#v", oldClient.Outbound)
	}

	var newClient dto.UpdateServerNodeConfigRequest
	if err := json.Unmarshal([]byte(`{
		"server_id":1,"inherit_block":false,"block":[],"block_ips":[],"block_protocols":[],
		"inherit_outbound":false,"outbound":[{"name":"first","protocol":"direct","rules":[],"ip_rules":[],"protocol_rules":[]}]
	}`), &newClient); err != nil {
		t.Fatal(err)
	}
	if err := MergeRouteRuleOmissions(&newClient, existing, nil); err != nil {
		t.Fatal(err)
	}
	if newClient.BlockIPs == nil || newClient.BlockProtocols == nil || newClient.Outbound[0].IPRules == nil || newClient.Outbound[0].ProtocolRules == nil {
		t.Fatalf("explicit empty arrays lost presence: %#v", newClient)
	}
	if len(newClient.BlockIPs) != 0 || len(newClient.BlockProtocols) != 0 || len(newClient.Outbound[0].IPRules) != 0 || len(newClient.Outbound[0].ProtocolRules) != 0 {
		t.Fatalf("explicit empty arrays were not retained: %#v", newClient)
	}
}

func TestMergeRouteRuleOmissionsUsesInheritedOutboundWhenCreatingOverride(t *testing.T) {
	req := dto.UpdateServerNodeConfigRequest{
		ServerID: 1,
		ServerNodeConfigOverride: dto.ServerNodeConfigOverride{
			InheritBlock:    true,
			InheritOutbound: false,
			Outbound:        []dto.NodeOutbound{{Name: "global", Protocol: "direct"}},
		},
		RequestFields:     map[string]struct{}{"outbound": {}},
		OutboundFieldSets: []map[string]struct{}{{"name": {}, "protocol": {}}},
	}
	if err := MergeRouteRuleOmissions(&req, dto.ServerNodeConfigOverride{InheritOutbound: true}, []dto.NodeOutbound{{
		Name: "global", Protocol: "direct", IPRules: []string{"geoip:cn"}, ProtocolRules: []string{"bittorrent"},
	}}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(req.Outbound[0].IPRules, []string{"geoip:cn"}) || !reflect.DeepEqual(req.Outbound[0].ProtocolRules, []string{"bittorrent"}) {
		t.Fatalf("inherited hidden outbound rules were not preserved: %#v", req.Outbound[0])
	}
}

func TestOverrideOutboundOmissionMatchingScenarios(t *testing.T) {
	existing := dto.ServerNodeConfigOverride{InheritOutbound: false, Outbound: []dto.NodeOutbound{
		{Name: "a", Protocol: "direct", Address: "a.example", Rules: []string{}, IPRules: []string{"geoip:a"}},
		{Name: "b", Protocol: "socks", Address: "b.example", Rules: []string{}, IPRules: []string{"geoip:b"}},
		{Name: "c", Protocol: "direct", Address: "c.example", Rules: []string{}, IPRules: []string{"geoip:c"}},
	}}
	bind := func(t *testing.T, outbound string) dto.UpdateServerNodeConfigRequest {
		t.Helper()
		var req dto.UpdateServerNodeConfigRequest
		body := `{"server_id":1,"inherit_block":true,"inherit_outbound":false,"outbound":` + outbound + `}`
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatal(err)
		}
		return req
	}

	t.Run("rename and reorder", func(t *testing.T) {
		req := bind(t, `[{"name":"b","protocol":"socks","address":"b.example","rules":[]},{"name":"renamed-a","protocol":"direct","address":"a.example","rules":[]}]`)
		if err := MergeRouteRuleOmissions(&req, existing, nil); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(req.Outbound[0].IPRules, []string{"geoip:b"}) || !reflect.DeepEqual(req.Outbound[1].IPRules, []string{"geoip:a"}) {
			t.Fatalf("hidden rules swapped: %#v", req.Outbound)
		}
	})
	t.Run("insertion", func(t *testing.T) {
		req := bind(t, `[{"name":"new","protocol":"vless","address":"new.example","rules":[]},{"name":"a","protocol":"direct","address":"a.example","rules":[]},{"name":"b","protocol":"socks","address":"b.example","rules":[]},{"name":"c","protocol":"direct","address":"c.example","rules":[]}]`)
		if err := MergeRouteRuleOmissions(&req, existing, nil); err != nil {
			t.Fatal(err)
		}
		if len(req.Outbound[0].IPRules) != 0 || !reflect.DeepEqual(req.Outbound[1].IPRules, []string{"geoip:a"}) {
			t.Fatalf("insertion inherited unrelated hidden rules: %#v", req.Outbound)
		}
	})
	t.Run("rename plus edit with unchanged anchored sequence", func(t *testing.T) {
		req := bind(t, `[{"name":"a","protocol":"direct","address":"a.example","rules":[]},{"name":"renamed-b","protocol":"socks","address":"edited.example","rules":[]},{"name":"c","protocol":"direct","address":"c.example","rules":[]}]`)
		if err := MergeRouteRuleOmissions(&req, existing, nil); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(req.Outbound[1].IPRules, []string{"geoip:b"}) {
			t.Fatalf("safe positional preservation failed: %#v", req.Outbound)
		}
	})
	t.Run("rename plus edit and reorder is rejected", func(t *testing.T) {
		req := bind(t, `[{"name":"renamed-b","protocol":"socks","address":"edited.example","rules":[]},{"name":"a","protocol":"direct","address":"a.example","rules":[]},{"name":"c","protocol":"direct","address":"c.example","rules":[]}]`)
		if err := MergeRouteRuleOmissions(&req, existing, nil); err == nil || !strings.Contains(err.Error(), "upgraded client") {
			t.Fatalf("error = %v, want ambiguity rejection", err)
		}
	})
	t.Run("deletion and reorder", func(t *testing.T) {
		req := bind(t, `[{"name":"c","protocol":"direct","address":"c.example","rules":[]},{"name":"a","protocol":"direct","address":"a.example","rules":[]}]`)
		if err := MergeRouteRuleOmissions(&req, existing, nil); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(req.Outbound[0].IPRules, []string{"geoip:c"}) || !reflect.DeepEqual(req.Outbound[1].IPRules, []string{"geoip:a"}) {
			t.Fatalf("deletion/reorder hidden rules = %#v", req.Outbound)
		}
	})
	for name, outbound := range map[string]string{
		"blank":     `[{"name":" ","protocol":"direct","rules":[]}]`,
		"duplicate": `[{"name":"a","protocol":"direct","rules":[]},{"name":" a ","protocol":"socks","rules":[]}]`,
	} {
		t.Run(name, func(t *testing.T) {
			req := bind(t, outbound)
			if err := MergeRouteRuleOmissions(&req, existing, nil); err == nil {
				t.Fatalf("%s outbound names unexpectedly accepted", name)
			}
		})
	}
}

func TestOverrideRejectsBlankProtocolBeforeOutboundNormalization(t *testing.T) {
	outbound := []dto.NodeOutbound{
		{Name: "first", Protocol: "   ", IPRules: []string{"geoip:first"}},
		{Name: "second", Protocol: "direct", IPRules: []string{"geoip:second"}},
	}
	_, _, err := OverrideModel(1, dto.ServerNodeConfigOverride{
		InheritIPStrategy: true, InheritDNS: true, InheritBlock: true,
		InheritOutbound: false, Outbound: outbound,
	})
	if err == nil || !strings.Contains(err.Error(), "outbound line 1: outbound protocol must not be blank") {
		t.Fatalf("blank protocol error = %v", err)
	}
	if outbound[1].Name != "second" || !reflect.DeepEqual(outbound[1].IPRules, []string{"geoip:second"}) {
		t.Fatalf("invalid first outbound shifted the second identity: %#v", outbound)
	}
}

func TestOverrideModelEnforcesSerializedTextLimit(t *testing.T) {
	rules := make([]string, 900)
	for index := range rules {
		rules[index] = fmt.Sprintf("host-%04d.%s.example", index, strings.Repeat("a", 55))
	}
	_, _, err := OverrideModel(1, dto.ServerNodeConfigOverride{
		InheritIPStrategy: true, InheritDNS: true, InheritBlock: false, Block: rules, InheritOutbound: true,
	})
	if err == nil || !strings.Contains(err.Error(), "serialized JSON") {
		t.Fatalf("oversize block error = %v", err)
	}
	_, _, err = OverrideModel(1, dto.ServerNodeConfigOverride{
		InheritIPStrategy: true, InheritDNS: true, InheritBlock: true, InheritOutbound: false,
		Outbound: []dto.NodeOutbound{{Name: "large", Protocol: "direct", Settings: strings.Repeat("x", config.MaxPersistedJSONBytes)}},
	})
	if err == nil || !strings.Contains(err.Error(), "outbound: serialized JSON") {
		t.Fatalf("oversize outbound error = %v", err)
	}
	_, _, err = OverrideModel(1, dto.ServerNodeConfigOverride{
		InheritIPStrategy: true, InheritDNS: false,
		DNS:          []dto.NodeDNS{{Proto: "https", Address: strings.Repeat("x", config.MaxPersistedJSONBytes)}},
		InheritBlock: true, InheritOutbound: true,
	})
	if err == nil || !strings.Contains(err.Error(), "dns: serialized JSON") {
		t.Fatalf("oversize DNS error = %v", err)
	}
}

func TestOverrideModelRejectsMisclassifiedRules(t *testing.T) {
	_, _, err := OverrideModel(1, dto.ServerNodeConfigOverride{
		InheritIPStrategy: true,
		InheritDNS:        true,
		InheritBlock:      false,
		Block:             []string{"geoip:private"},
		InheritOutbound:   true,
	})
	if err == nil || !strings.Contains(err.Error(), `block line 1: unknown domain rule prefix "geoip"`) {
		t.Fatalf("OverrideModel() error = %v, want contextual domain-category error", err)
	}
}
