package config

import (
	"encoding/json"
	"strings"
	"testing"
)

const testV2 = `{"version":2,"outbounds":[{"id":"ob_11111111-1111-4111-8111-111111111111","name":"A","protocol":"socks","address":"127.0.0.1","port":1080,"password":"","rules":[],"ip_rules":[],"protocol_rules":[]},{"id":"ob_22222222-2222-4222-8222-222222222222","name":"B","protocol":"socks","address":"127.0.0.1","port":1081,"password":"","via_outbound_id":"ob_11111111-1111-4111-8111-111111111111","rules":[],"ip_rules":[],"protocol_rules":[]}],"rules":[{"id":"rr_33333333-3333-4333-8333-333333333333","enabled":true,"remark":"","kind":"domain","values":["domain:example.com"],"user_ids":[3,1,3],"outbound_id":"direct"}],"default_outbound_id":"direct","user_defaults":[{"user_id":3,"outbound_id":"ob_22222222-2222-4222-8222-222222222222"}],"dns_policy":{"query_strategy":"UseIP","resolvers":[{"id":"dns_44444444-4444-4444-8444-444444444444","protocol":"udp","address":"192.0.2.53","domains":[],"outbound_id":"direct"}]}}`

func TestRoutingV2NormalizeAndValidate(t *testing.T) {
	p, err := ParseNodeRouting([]byte(testV2))
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != 2 || len(p.Rules[0].UserIDs) != 2 || p.Rules[0].UserIDs[0] != 1 || p.Outbounds[1].ViaOutboundID == "" || !HasRoutingV2Capabilities(p) {
		t.Fatalf("bad policy: %+v", p)
	}
	body, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseNodeRouting(body); err != nil {
		t.Fatalf("roundtrip: %s: %v", body, err)
	}
	for _, bad := range []string{
		strings.Replace(testV2, `"via_outbound_id":"ob_11111111-1111-4111-8111-111111111111"`, `"via_outbound_id":"ob_22222222-2222-4222-8222-222222222222"`, 1),
		strings.Replace(testV2, `"via_outbound_id":"ob_11111111-1111-4111-8111-111111111111"`, `"via_outbound_id":"ob_99999999-9999-4999-8999-999999999999"`, 1),
		strings.Replace(testV2, `"user_defaults":[{`, `"user_defaults":[{"user_id":3,"outbound_id":"direct"},{`, 1),
		strings.Replace(testV2, `"protocol":"udp"`, `"protocol":"quic"`, 1),
		strings.Replace(testV2, `"dns_policy":{`, `"dns_policy":{"unknown":true,`, 1),
		strings.Replace(testV2, `"user_ids":[3,1,3]`, `"user_ids":null`, 1),
		strings.Replace(testV2, `"address":"192.0.2.53"`, `"address":"dns.example.com"`, 1),
		strings.Replace(testV2, `"address":"192.0.2.53"`, `"address":"192.0.2.53:99999"`, 1),
	} {
		if _, err := ParseNodeRouting([]byte(bad)); err == nil {
			t.Fatalf("accepted invalid: %s", bad)
		}
	}
}

func TestV2DNSBootstrapAndRawChain(t *testing.T) {
	for _, protocol := range []string{"udp", "tcp", "https"} {
		p, err := ParseNodeRouting([]byte(testV2))
		if err != nil {
			t.Fatal(err)
		}
		r := &p.DNSPolicy.Resolvers[0]
		r.Protocol = protocol
		r.Address = "resolver.example"
		if protocol == "https" {
			r.Address = "https://resolver.example/dns-query"
		}
		r.BootstrapIP = "192.0.2.1"
		if _, err = NormalizeNodeRouting(p); err != nil {
			t.Fatalf("%s: %v", protocol, err)
		}
	}
	p, err := ParseNodeRouting([]byte(testV2))
	if err != nil {
		t.Fatal(err)
	}
	p.DNSPolicy.Resolvers[0].BootstrapIP = "192.0.2.11"
	if _, err = NormalizeNodeRouting(p); err == nil {
		t.Fatal("literal resolver accepted bootstrap")
	}
	p.DNSPolicy.Resolvers = []NodeDNSResolver{
		{ID: "dns_44444444-4444-4444-8444-444444444444", Protocol: "udp", Address: "RESOLVER.example", BootstrapIP: "192.0.2.1", Domains: []string{"full:example.com"}, OutboundID: "direct"},
		{ID: "dns_55555555-5555-4555-8555-555555555555", Protocol: "tcp", Address: "resolver.example.", BootstrapIP: "192.0.2.2", Domains: []string{}, OutboundID: "direct"},
	}
	if _, err = NormalizeNodeRouting(p); err == nil {
		t.Fatal("conflicting bootstrap accepted")
	}
	p.DNSPolicy.Resolvers[1].BootstrapIP = "192.0.2.1"
	if _, err = NormalizeNodeRouting(p); err != nil {
		t.Fatalf("shared consistent bootstrap rejected: %v", err)
	}
	p, err = ParseNodeRouting([]byte(testV2))
	if err != nil {
		t.Fatal(err)
	}
	p.Outbounds[0].StreamSettings = `{"sockopt":{"dialerProxy":"routing_ob_22222222-2222-4222-8222-222222222222"}}`
	if _, err = NormalizeNodeRouting(p); err == nil {
		t.Fatal("raw/structured cycle accepted")
	}
	p.Outbounds[0].StreamSettings = `{"sockopt":{"dialerProxy":"routing_ob_99999999-9999-4999-8999-999999999999"}}`
	if _, err = NormalizeNodeRouting(p); err == nil {
		t.Fatal("dangling raw chain accepted")
	}
	p.Outbounds[0].StreamSettings = `{"sockopt":{"tcpNoDelay":true}}`
	if _, err = NormalizeNodeRouting(p); err != nil {
		t.Fatalf("valid raw rejected: %v", err)
	}
	p.Outbounds[0].StreamSettings = `{"sockopt":{"addressPortStrategy":"srvPortOnly"}}`
	if _, err = NormalizeNodeRouting(p); err == nil {
		t.Fatal("system SRV accepted with explicit DNS")
	}
	p.DNSPolicy = nil
	if _, err = NormalizeNodeRouting(p); err != nil {
		t.Fatalf("old DNS raw setting rejected: %v", err)
	}
}

func TestHasRoutingV2CapabilitiesDetectsEnabledIPRules(t *testing.T) {
	enabled, disabled := true, false
	cases := []struct {
		name   string
		policy *NodeRouting
		want   bool
	}{
		{"nil policy", nil, false},
		{"v1 IP rule", &NodeRouting{Version: 1, Rules: []NodeRoute{{Kind: "ip", Enabled: &enabled, Values: []string{"198.51.100.0/24"}}}}, false},
		{"v2 enabled IP", &NodeRouting{Version: 2, Rules: []NodeRoute{{Kind: "ip", Enabled: &enabled, Values: []string{"198.51.100.0/24"}}}}, true},
		{"v2 disabled IP", &NodeRouting{Version: 2, Rules: []NodeRoute{{Kind: "ip", Enabled: &disabled, Values: []string{"198.51.100.0/24"}}}}, false},
		{"v2 missing enabled", &NodeRouting{Version: 2, Rules: []NodeRoute{{Kind: "ip", Values: []string{"198.51.100.0/24"}}}}, false},
		{"v2 empty IP", &NodeRouting{Version: 2, Rules: []NodeRoute{{Kind: "ip", Enabled: &enabled}}}, false},
		{"v2 domain", &NodeRouting{Version: 2, Rules: []NodeRoute{{Kind: "domain", Enabled: &enabled, Values: []string{"full:example.com"}}}}, false},
		{"v2 disabled user rule unchanged", &NodeRouting{Version: 2, Rules: []NodeRoute{{Kind: "domain", Enabled: &disabled, UserIDs: []int64{1}}}}, true},
		{"v2 DNS unchanged", &NodeRouting{Version: 2, DNSPolicy: &NodeDNSPolicy{}}, true},
		{"v2 via unchanged", &NodeRouting{Version: 2, Outbounds: []NodeOutbound{{ViaOutboundID: "ob_example"}}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasRoutingV2Capabilities(tc.policy); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRoutingVersion1WireDoesNotGrow(t *testing.T) {
	p := NodeRouting{Version: 1, Outbounds: []NodeOutbound{}, Rules: []NodeRoute{}, DefaultOutboundID: "direct"}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "user_defaults") || strings.Contains(string(b), "dns_policy") {
		t.Fatal(string(b))
	}
	if _, err := ParseNodeRouting(b); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseNodeRouting([]byte(strings.Replace(string(b), `"version":1`, `"version":1,"user_defaults":[]`, 1))); err == nil {
		t.Fatal("v1 accepted v2 field")
	}
	p.Version = 2
	p.UserDefaults = []NodeUserDefault{}
	b, err = json.Marshal(p)
	if err != nil || !strings.Contains(string(b), `"user_defaults":[]`) {
		t.Fatalf("v2 empty defaults: %s %v", b, err)
	}
}
