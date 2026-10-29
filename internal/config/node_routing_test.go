package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func routingID(prefix string, n int) string {
	return fmt.Sprintf("%s%08x-0000-4000-8000-%012x", prefix, n, n)
}

func routingBool(v bool) *bool { return &v }

func basicRouting() *NodeRouting {
	return &NodeRouting{
		Version: RoutingVersion1, DefaultOutboundID: RoutingDirect,
		Outbounds: []NodeOutbound{{ID: routingID("ob_", 1), Name: "proxy", Protocol: "socks", Rules: []string{}, IPRules: []string{}, ProtocolRules: []string{}}},
		Rules:     []NodeRoute{{ID: routingID("rr_", 1), Enabled: routingBool(false), Kind: "domain", Values: []string{"example.com.", "full:example.com", "geosite:cn"}, OutboundID: RoutingBlock}},
	}
}

func TestNodeRoutingCanonicalAndPresence(t *testing.T) {
	input := basicRouting()
	got, err := NormalizeNodeRouting(input)
	if err != nil {
		t.Fatal(err)
	}
	if *got.Rules[0].Enabled || !reflect.DeepEqual(got.Rules[0].Values, []string{"full:example.com", "geosite:cn"}) {
		t.Fatalf("canonical rule: %#v", got.Rules[0])
	}
	if input.Rules[0].Values[0] != "example.com." {
		t.Fatal("mutated input")
	}
	body, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseNodeRouting(body)
	if err != nil || !reflect.DeepEqual(parsed, got) {
		t.Fatalf("round trip = %#v, %v", parsed, err)
	}
	for _, edit := range []func(*NodeRouting){
		func(p *NodeRouting) { p.Rules[0].Enabled = nil },
		func(p *NodeRouting) { p.Rules[0].Values = nil },
		func(p *NodeRouting) { p.Rules[0].Values = []string{" "} },
		func(p *NodeRouting) { p.Rules[0].Kind = "unknown" },
		func(p *NodeRouting) { p.Rules[0].OutboundID = "unknown" },
		func(p *NodeRouting) { p.DefaultOutboundID = "unknown" },
		func(p *NodeRouting) { p.Rules[0].ID = routingID("rr_", 1) + "x" },
		func(p *NodeRouting) { p.Outbounds[0].ID = RoutingDirect },
		func(p *NodeRouting) { p.Outbounds[0].Rules = []string{"geosite:cn"} },
		func(p *NodeRouting) { p.Rules = nil },
		func(p *NodeRouting) { p.Outbounds = nil },
		func(p *NodeRouting) { p.Version = 2 },
	} {
		value := basicRouting()
		edit(value)
		if _, err := NormalizeNodeRouting(value); err == nil {
			t.Errorf("unexpectedly accepted %#v", value)
		}
	}
	for _, raw := range []string{`null`, `{}`, `{"version":1,"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct"}`, `{"Version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct"}`, `{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct","future":1}`, `{} {}`, `{"version":1,"outbounds":[],"rules":[{"id":"rr_00000001-0000-4000-8000-000000000001","kind":"domain","values":["example.com"],"outbound_id":"direct"}],"default_outbound_id":"direct"}`} {
		if _, err := ParseNodeRouting([]byte(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}

func TestMissingOutboundReportsEveryReference(t *testing.T) {
	p := basicRouting()
	deleted := p.Outbounds[0].ID
	p.Outbounds = []NodeOutbound{}
	p.DefaultOutboundID = deleted
	p.Rules[0].OutboundID = deleted
	extra := p.Rules[0]
	extra.ID = routingID("rr_", 2)
	p.Rules = append(p.Rules, extra)
	_, err := NormalizeNodeRouting(p)
	if err == nil {
		t.Fatal("missing outbound accepted")
	}
	for _, want := range []string{"default", p.Rules[0].ID, p.Rules[1].ID, deleted} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing reference %s in %v", want, err)
		}
	}
}

func TestNodeRoutingReferencesIDsAndBudgets(t *testing.T) {
	tests := map[string]func(*NodeRouting){
		"disabled bad reference": func(p *NodeRouting) { p.Rules[0].OutboundID = "ob_deleted" },
		"duplicate rule":         func(p *NodeRouting) { p.Rules = append(p.Rules, p.Rules[0]) },
		"duplicate outbound":     func(p *NodeRouting) { p.Outbounds = append(p.Outbounds, p.Outbounds[0]) },
		"duplicate name": func(p *NodeRouting) {
			x := p.Outbounds[0]
			x.ID = routingID("ob_", 2)
			p.Outbounds = append(p.Outbounds, x)
		},
		"duplicate values before dedup": func(p *NodeRouting) {
			p.Rules[0].Values = make([]string, MaxRoutingValuesPerRule+1)
			for i := range p.Rules[0].Values {
				p.Rules[0].Values[i] = "example.com"
			}
		},
		"long remark": func(p *NodeRouting) { p.Rules[0].Remark = strings.Repeat("中", MaxRoutingRemarkRunes+1) },
		"raw XHTTP mode": func(p *NodeRouting) {
			p.Outbounds[0].StreamSettings = `{"network":"xhttp","xhttpSettings":{"mode":"bogus"}}`
		},
		"raw XHTTP xmux": func(p *NodeRouting) {
			p.Outbounds[0].StreamSettings = `{"network":"xhttp","xhttpSettings":{"xmux":"oops"}}`
		},
		"invalid raw":               func(p *NodeRouting) { p.Outbounds[0].Settings = "{" },
		"raw null":                  func(p *NodeRouting) { p.Outbounds[0].StreamSettings = "null" },
		"raw fold collision":        func(p *NodeRouting) { p.Outbounds[0].Settings = `{"security":"tls","Security":"none"}` },
		"xhttp fold collision":      func(p *NodeRouting) { p.Outbounds[0].XHTTPExtra = `{"nested":{"path":"/ok","Path":"/%"}}` },
		"plugin map fold collision": func(p *NodeRouting) { p.Outbounds[0].PluginOptions = map[string]any{"tls": true, "TLS": false} },
	}
	for name, edit := range tests {
		t.Run(name, func(t *testing.T) {
			p := basicRouting()
			edit(p)
			if _, err := NormalizeNodeRouting(p); err == nil {
				t.Fatal("invalid policy accepted")
			}
		})
	}
	p := basicRouting()
	p.Outbounds[0].Settings = ` { "value" : "x" } `
	p.Outbounds[0].PluginOptions = map[string]any{"mode": "ws", "large": json.Number("9007199254740992")}
	got, err := NormalizeNodeRouting(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Outbounds[0].Settings != p.Outbounds[0].Settings {
		t.Fatalf("raw JSON changed: %q", got.Outbounds[0].Settings)
	}
	got.Outbounds[0].PluginOptions.(map[string]any)["mode"] = "changed"
	if p.Outbounds[0].PluginOptions.(map[string]any)["mode"] != "ws" {
		t.Fatal("plugin options aliased")
	}
	if got.Outbounds[0].PluginOptions.(map[string]any)["large"] != json.Number("9007199254740992") {
		t.Fatal("plugin option integer lost precision")
	}
	p.Outbounds[0].PluginOptions = map[string]any{"large": json.Number("9007199254740993")}
	if _, err := NormalizeNodeRouting(p); err == nil {
		t.Fatal("protobuf-lossy plugin option accepted")
	}
	p.Outbounds[0].PluginOptions = nil
	p.Outbounds[0].Address = string([]byte{0xff})
	if _, err := NormalizeNodeRouting(p); err == nil {
		t.Fatal("invalid UTF-8 in a connection accepted")
	}
}

func TestNodeRoutingLegacyMaximumFixture(t *testing.T) {
	// Three block categories at their legacy 1,024-value ceiling, plus
	// 4,096 outbound matches across 128 outbounds and 384 adjacent routes.
	// The real protocol category only accepts bittorrent, so the block protocol
	// category has one value and the other two hold 1,024 each. A separate
	// aggregate test below exercises the full 8,192-v1 boundary.
	p := &NodeRouting{Version: 1, DefaultOutboundID: RoutingDirect, Outbounds: []NodeOutbound{}, Rules: []NodeRoute{}}
	n := 0
	add := func(kind, target string, values []string) {
		n++
		p.Rules = append(p.Rules, NodeRoute{ID: routingID("rr_", n), Enabled: routingBool(true), Kind: kind, Values: values, OutboundID: target})
	}
	blockDomain, blockIP := make([]string, 1024), make([]string, 1024)
	for i := range blockDomain {
		blockDomain[i] = fmt.Sprintf("block-%04d.%s.example", i, strings.Repeat("b", 32))
		blockIP[i] = fmt.Sprintf("10.%d.%d.0/24", i/256, i%256)
	}
	add("protocol", RoutingBlock, []string{"bittorrent"})
	add("ip", RoutingBlock, blockIP)
	add("domain", RoutingBlock, blockDomain)
	for i := 0; i < 128; i++ {
		id := routingID("ob_", i+1)
		p.Outbounds = append(p.Outbounds, NodeOutbound{ID: id, Name: fmt.Sprintf("outbound-%d", i), Protocol: "direct", Rules: []string{}, IPRules: []string{}, ProtocolRules: []string{}})
		// 15 domain + 16 IP + 1 protocol = 32 values per outbound.
		domains, ips := make([]string, 15), make([]string, 16)
		for j := range domains {
			domains[j] = fmt.Sprintf("h%d-%d.example", i, j)
		}
		for j := range ips {
			ips[j] = fmt.Sprintf("172.%d.%d.0/24", i, j)
		}
		add("domain", id, domains)
		add("ip", id, ips)
		add("protocol", id, []string{"bittorrent"})
	}
	got, err := NormalizeNodeRouting(p)
	if err != nil {
		t.Fatalf("maximum legacy fixture: %v", err)
	}
	if len(got.Rules) != 387 || len(got.Outbounds) != 128 {
		t.Fatalf("fixture shape: %d / %d", len(got.Rules), len(got.Outbounds))
	}
}

func TestNodeRoutingInputAndSerializedBoundaries(t *testing.T) {
	p := &NodeRouting{Version: 1, Outbounds: []NodeOutbound{}, Rules: []NodeRoute{}, DefaultOutboundID: RoutingDirect}
	for i := 0; i < 8; i++ {
		values := make([]string, 1024)
		for j := range values {
			values[j] = fmt.Sprintf("h%d-%d.example", i, j)
		}
		p.Rules = append(p.Rules, NodeRoute{ID: routingID("rr_", i+1), Enabled: routingBool(true), Kind: "domain", Values: values, OutboundID: RoutingDirect})
	}
	if _, err := NormalizeNodeRouting(p); err != nil {
		t.Fatalf("8192 values rejected: %v", err)
	}
	p.Rules = append(p.Rules, NodeRoute{ID: routingID("rr_", 9), Enabled: routingBool(false), Kind: "protocol", Values: []string{"bittorrent"}, OutboundID: RoutingBlock})
	if _, err := NormalizeNodeRouting(p); err == nil {
		t.Fatal("8193 input values accepted")
	}

	p = basicRouting()
	p.Rules[0].Values = []string{"full:example.com", "geosite:cn"}
	p.Outbounds[0].Settings = `{"value":""}`
	encoded, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	padding := MaxRoutingJSONBytes - len(encoded)
	p.Outbounds[0].Settings = `{"value":"` + strings.Repeat("x", padding) + `"}`
	encoded, _ = json.Marshal(p)
	if len(encoded) != MaxRoutingJSONBytes {
		t.Fatalf("fixture bytes = %d", len(encoded))
	}
	if _, err := NormalizeNodeRouting(p); err != nil {
		t.Fatalf("1 MiB rejected: %v", err)
	}
	if _, err := ParseNodeRouting(encoded); err != nil {
		t.Fatalf("1 MiB wire policy rejected: %v", err)
	}
	p.Outbounds[0].Settings = `{"value":"` + strings.Repeat("x", padding+1) + `"}`
	if _, err := NormalizeNodeRouting(p); err == nil {
		t.Fatal("over 1 MiB accepted")
	}
}
