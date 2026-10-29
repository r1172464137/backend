package config

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestLegacyConversionPreservesOROrderAndConnections(t *testing.T) {
	input := LegacyRoutingInput{BlockProtocols: []string{"bittorrent"}, BlockIPs: []string{"geoip:private"}, Block: []string{"geosite:cn"}, Outbound: []NodeOutbound{{Name: "socks", Protocol: "socks", Address: "127.0.0.1", Port: 1080, Rules: []string{"example.com"}, IPRules: []string{"10.1.2.3/8"}, ProtocolRules: []string{"bittorrent"}}, {Name: "custom-direct", Protocol: "direct", Settings: `{"domainStrategy":"UseIPv4"}`}}}
	got, err := ConvertLegacyRouting(input, "global")
	if err != nil || len(got.Blockers) > 0 || got.Draft == nil {
		t.Fatalf("preview %#v %v", got, err)
	}
	kinds := []string{}
	for _, rule := range got.Draft.Rules {
		kinds = append(kinds, rule.Kind)
	}
	if !reflect.DeepEqual(kinds, []string{"protocol", "ip", "domain", "domain", "ip", "protocol"}) {
		t.Fatalf("order = %v", kinds)
	}
	id := got.Draft.Outbounds[0].ID
	for _, r := range got.Draft.Rules[3:] {
		if r.OutboundID != id {
			t.Fatal("OR categories no longer target same outbound")
		}
	}
	if got.Draft.Outbounds[1].Settings != input.Outbound[1].Settings {
		t.Fatal("custom direct raw settings were not retained")
	}
	repeat, err := ConvertLegacyRouting(input, "global")
	if err != nil || !reflect.DeepEqual(got, repeat) {
		t.Fatal("same cutover input produced different identities")
	}
	other, err := ConvertLegacyRouting(input, "server:7")
	if err != nil || other.Draft.Outbounds[0].ID == id {
		t.Fatal("different scopes reused an outbound identity")
	}
}
func TestLegacyMaximumPersistedPartitionsActuallyConvert(t *testing.T) {
	input := LegacyRoutingInput{Block: make([]string, 1024), BlockIPs: make([]string, 1024), BlockProtocols: []string{"bittorrent"}}
	for i := 0; i < 1024; i++ {
		input.Block[i] = fmt.Sprintf("h%04d.%s.example", i, strings.Repeat("a", 35))
		input.BlockIPs[i] = fmt.Sprintf("10.%d.%d.0/24", i/256, i%256)
	}
	for i := 0; i < 128; i++ {
		ob := NodeOutbound{Name: fmt.Sprintf("out-%03d", i), Protocol: "direct", Rules: make([]string, 30), IPRules: []string{fmt.Sprintf("172.%d.0.0/16", i)}, ProtocolRules: []string{"bittorrent"}}
		for j := range ob.Rules {
			ob.Rules[j] = fmt.Sprintf("h%02d.a", j)
		}
		input.Outbound = append(input.Outbound, ob)
	}
	for _, part := range []struct {
		name  string
		value any
	}{{"block", input.Block}, {"block_ips", input.BlockIPs}, {"block_protocols", input.BlockProtocols}, {"outbound", input.Outbound}} {
		if err := ValidatePersistedJSON(part.name, part.value); err != nil {
			t.Fatalf("fixture was not valid legacy data: %v", err)
		}
	}
	preview, err := ConvertLegacyRouting(input, "global")
	if err != nil || preview.Draft == nil || len(preview.Blockers) != 0 {
		t.Fatalf("conversion %#v %v", preview.Blockers, err)
	}
	if len(preview.Draft.Rules) != 387 || len(preview.Draft.Outbounds) != 128 {
		t.Fatalf("converted shape %d/%d", len(preview.Draft.Rules), len(preview.Draft.Outbounds))
	}
}

func TestLegacyConversionBlocksUnverifiableAndTagConflicts(t *testing.T) {
	for _, o := range []NodeOutbound{
		{Name: "Default", Protocol: "direct"}, {Name: "block", Protocol: "direct"}, {Name: "dns_out", Protocol: "direct"},
		{Name: "unknown", Protocol: "unknown"}, {Name: "unknown-raw", Protocol: "unknown", Settings: `{}`},
		{Name: "wireguard", Protocol: "wireguard"}, {Name: "raw", Protocol: "socks", Settings: `{"foo":true}`},
		{Name: "alpn", Protocol: "direct", ALPN: []string{"h2"}}, {Name: "plugin", Protocol: "direct", Plugin: "plugin"},
		{Name: "uot", Protocol: "shadowsocks", Address: "127.0.0.1", Port: 443, Password: "test", UoT: true},
	} {
		got, err := ConvertLegacyRouting(LegacyRoutingInput{Outbound: []NodeOutbound{o}}, "global")
		if err != nil || got.Draft != nil || len(got.Blockers) == 0 {
			t.Fatalf("accepted %#v: %#v %v", o, got, err)
		}
	}
	got, err := ConvertLegacyRouting(LegacyRoutingInput{Outbound: []NodeOutbound{{Name: "duplicate", Protocol: "direct"}, {Name: "duplicate", Protocol: "direct"}}}, "global")
	if err != nil || got.Draft != nil || len(got.Blockers) == 0 {
		t.Fatal("duplicate names accepted")
	}
}
