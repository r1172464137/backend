package bootstrap

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/perfect-panel/server/internal/config"
)

// A synthetic policy with the shape of the production seven rules: private
// IP block, BitTorrent block, domain block, two node-direct exceptions, CN
// domain block and CN IP block; default node-direct.
const sevenRulesV1 = `{"version":1,"outbounds":[],"rules":[` +
	`{"id":"rr_10000000-0000-4000-8000-000000000001","enabled":true,"remark":"private","kind":"ip","values":["geoip:private"],"outbound_id":"block"},` +
	`{"id":"rr_10000000-0000-4000-8000-000000000002","enabled":true,"remark":"bt","kind":"protocol","values":["bittorrent"],"outbound_id":"block"},` +
	`{"id":"rr_10000000-0000-4000-8000-000000000003","enabled":true,"remark":"pt","kind":"domain","values":["domain:tracker.example"],"outbound_id":"block"},` +
	`{"id":"rr_10000000-0000-4000-8000-000000000004","enabled":true,"remark":"google-cn","kind":"domain","values":["geosite:google-cn"],"outbound_id":"direct"},` +
	`{"id":"rr_10000000-0000-4000-8000-000000000005","enabled":false,"remark":"apple","kind":"domain","values":["geosite:apple"],"outbound_id":"direct"},` +
	`{"id":"rr_10000000-0000-4000-8000-000000000006","enabled":true,"remark":"cn-domain","kind":"domain","values":["geosite:cn"],"outbound_id":"block"},` +
	`{"id":"rr_10000000-0000-4000-8000-000000000007","enabled":true,"remark":"cn-ip","kind":"ip","values":["geoip:cn"],"outbound_id":"block"}` +
	`],"default_outbound_id":"direct"}`

func TestConvertRoutingV1KeepsRulesAndAddsFormat3Defaults(t *testing.T) {
	out, err := convertRoutingV1(sevenRulesV1, "global", "prefer_ipv4")
	if err != nil {
		t.Fatal(err)
	}
	p, err := config.ParseNodeRouting([]byte(out))
	if err != nil {
		t.Fatalf("converted policy does not parse as format 3: %v", err)
	}
	var v1 routingV1
	if err := json.Unmarshal([]byte(sevenRulesV1), &v1); err != nil {
		t.Fatal(err)
	}
	if len(p.Rules) != 7 || p.DefaultOutboundID != "direct" || len(p.Outbounds) != 0 {
		t.Fatalf("shape changed: %d rules, default %q", len(p.Rules), p.DefaultOutboundID)
	}
	for i, r := range p.Rules {
		want := v1.Rules[i]
		if r.ID != want.ID || r.Kind != want.Kind || r.OutboundID != want.OutboundID || *r.Enabled != *want.Enabled || r.Remark != want.Remark {
			t.Fatalf("rule %d changed: %#v", i+1, r)
		}
		if r.UserIDs == nil || len(r.UserIDs) != 0 {
			t.Fatalf("rule %d account scope = %v, want [] (all accounts)", i+1, r.UserIDs)
		}
	}
	if p.UserDefaults == nil || len(p.UserDefaults) != 0 {
		t.Fatalf("account defaults = %v", p.UserDefaults)
	}
	if p.TargetIPPreference != "prefer_ipv4" {
		t.Fatalf("target preference = %q", p.TargetIPPreference)
	}
	if p.DNS == nil || p.DNS.QueryStrategy != "UseIP" || len(p.DNS.Resolvers) != 1 || p.DNS.Resolvers[0].Protocol != "system" {
		t.Fatalf("dns = %#v", p.DNS)
	}
	again, err := convertRoutingV1(sevenRulesV1, "global", "prefer_ipv4")
	if err != nil || again != out {
		t.Fatal("conversion is not deterministic")
	}
	if other, _ := convertRoutingV1(sevenRulesV1, "server-7", "prefer_ipv4"); other == out {
		t.Fatal("different scopes produced the same resolver ID")
	}
}

func TestConvertRoutingV1RejectsOtherInputs(t *testing.T) {
	for name, raw := range map[string]string{
		"format 2":      strings.Replace(sevenRulesV1, `"version":1`, `"version":2`, 1),
		"format 3":      strings.Replace(sevenRulesV1, `"version":1`, `"version":3`, 1),
		"unknown field": strings.Replace(sevenRulesV1, `"default_outbound_id"`, `"dns_policy":{},"default_outbound_id"`, 1),
		"duplicate key": strings.Replace(sevenRulesV1, `"version":1`, `"version":1,"version":1`, 1),
		"trailing data": sevenRulesV1 + `{}`,
		"bad reference": strings.Replace(sevenRulesV1, `"outbound_id":"block"}`, `"outbound_id":"ob_ffffffff-ffff-4fff-8fff-ffffffffffff"}`, 1),
		"empty":         ``,
	} {
		if _, err := convertRoutingV1(raw, "global", "none"); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestLegacyDNSAndIPStrategyMapping(t *testing.T) {
	for _, raw := range []string{"", "null", "[]", " [ ] "} {
		if empty, err := legacyDNSEmpty(raw); err != nil || !empty {
			t.Errorf("legacy DNS %q: empty=%v err=%v", raw, empty, err)
		}
	}
	if empty, err := legacyDNSEmpty(`[{"proto":"udp","address":"8.8.8.8:53"}]`); err != nil || empty {
		t.Error("non-empty legacy DNS reported empty")
	}
	if _, err := legacyDNSEmpty(`{`); err == nil {
		t.Error("unreadable legacy DNS accepted")
	}
	for in, want := range map[string]string{"": "none", "prefer_ipv4": "prefer_ipv4", "prefer_ipv6": "prefer_ipv6"} {
		if got, err := targetPreferenceFromLegacy(in); err != nil || got != want {
			t.Errorf("ip_strategy %q -> %q %v", in, got, err)
		}
	}
	for _, in := range []string{"UseIPv4", "ipv4_only", "prefer_ipv4v6"} {
		if _, err := targetPreferenceFromLegacy(in); err == nil {
			t.Errorf("ip_strategy %q accepted", in)
		}
	}
}

func TestFreshInstallationDefaultIsValidFormat3(t *testing.T) {
	body, err := json.Marshal(DefaultRoutingV3())
	if err != nil {
		t.Fatal(err)
	}
	p, err := config.ParseNodeRouting(body)
	if err != nil {
		t.Fatal(err)
	}
	if p.DefaultOutboundID != "direct" || len(p.Rules) != 0 || p.TargetIPPreference != "none" {
		t.Fatalf("fresh default %#v", p)
	}
}
