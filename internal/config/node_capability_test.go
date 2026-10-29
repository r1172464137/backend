package config

import (
	"strings"
	"testing"
)

func TestNodeSpiderURIAndNumericBounds(t *testing.T) {
	for _, value := range []string{"/%", "/?p=%", "//attacker.example/path", "/path#frag", "/?p=-1", "/?p=1-2-3", "/?p=4097", "/?c=9", "/?t=17", "/?i=30001", "/?r=5001", "/?c=8&t=9", "/?p=2-1", "/?P=1", "/?p=1&p=2", "/?x=1&x=2", "/?x=%00", "/" + strings.Repeat("x", 4096)} {
		if err := ValidateNodeURI("spider_x", value, true); err == nil {
			t.Errorf("accepted spider URI %q", value)
		}
	}
	for _, value := range []string{"", "/", "/path?p=1-2&x=value", "/?p=4096&c=8&t=8&i=30000&r=5000"} {
		if err := ValidateNodeURI("spider_x", value, true); err != nil {
			t.Errorf("rejected valid spider URI %q: %v", value, err)
		}
	}
}
func TestSpiderBoundsApplyToConversionAndManagementPolicies(t *testing.T) {
	cases := []struct {
		value string
		valid bool
	}{
		{"/", true}, {"/?p=4096&c=8&t=8&i=30000&r=5000", true}, {"/?p=0-2&c=0-8", true},
		{"/?p=4097", false}, {"/?c=9", false}, {"/?t=17", false}, {"/?i=30001", false}, {"/?r=5001", false},
		{"/?c=8&t=9", false}, {"/?p=2-1", false}, {"/?P=1", false}, {"/?p=-1", false}, {"/?p=1-2-3", false}, {"/?p=1&p=2", false}, {"/?x=1&x=2", false}, {"/?p=18446744073709551616", false},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			outbound := NodeOutbound{Name: "direct-custom", Protocol: "direct", SpiderX: tc.value}
			preview, err := ConvertLegacyRouting(LegacyRoutingInput{Outbound: []NodeOutbound{outbound}}, "global")
			if err != nil || (preview.Draft != nil) != tc.valid {
				t.Fatalf("conversion valid=%v: %#v %v", tc.valid, preview, err)
			}
			outbound.ID = routingID("ob_", 1)
			policy := &NodeRouting{Version: 1, Outbounds: []NodeOutbound{outbound}, Rules: []NodeRoute{}, DefaultOutboundID: "direct"}
			_, err = NormalizeNodeRouting(policy)
			if (err == nil) != tc.valid {
				t.Fatalf("management valid=%v: %v", tc.valid, err)
			}
		})
	}
}

func TestNestedRealitySpiderBoundsCannotBypassManagementValidation(t *testing.T) {
	for _, tc := range []struct {
		field, raw string
		valid      bool
	}{
		{"stream_settings", `{"realitySettings":{"spiderX":"/?p=4096"}}`, true},
		{"stream_settings", `{"realitySettings":{"spiderX":"/?p=4097"}}`, false},
		{"stream_settings", `{"xhttpSettings":{"downloadSettings":{"realitySettings":{"spiderX":"/?c=9"}}}}`, false},
		{"xhttp_extra", `{"downloadSettings":{"realitySettings":{"spiderX":"/?r=5001"}}}`, false},
		{"xhttp_extra", `{"downloadSettings":{"realitySettings":{"spiderX":"/?r=5000"}}}`, true},
		{"stream_settings", `{"realitySettings":{"spiderX":12}}`, false},
		{"stream_settings", `{"security":"xtls"}`, false},
		{"stream_settings", `{"security":"unknown"}`, false},
		{"xhttp_extra", `{"downloadSettings":{"security":"xtls"}}`, false},
		{"xhttp_extra", `{"headers":{"security":"arbitrary-header"}}`, true},
	} {
		t.Run(tc.field+"/"+tc.raw, func(t *testing.T) {
			p := basicRouting()
			if tc.field == "stream_settings" {
				p.Outbounds[0].StreamSettings = tc.raw
			} else {
				p.Outbounds[0].XHTTPExtra = tc.raw
			}
			_, err := NormalizeNodeRouting(p)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
		})
	}
}

func TestCutoverBlocksNodeUnsupportedOutboundCapabilities(t *testing.T) {
	for _, ob := range []NodeOutbound{
		{Name: "tuic", Protocol: "tuic", Address: "127.0.0.1", Port: 443, UUID: "00000000-0000-0000-0000-000000000001", Password: "secret"},
		{Name: "tuic-transport", Protocol: "direct", Transport: "tuic"},
		{Name: "plugin-opts", Protocol: "direct", PluginOptions: map[string]any{"mode": "ws"}},
		{Name: "spider", Protocol: "direct", SpiderX: "/%"},
		{Name: "hysteria-stream", Protocol: "hysteria2", Address: "127.0.0.1", Port: 443, Transport: "tcp"},
		{Name: "bad-path", Protocol: "direct", Path: "/%"},
	} {
		p, err := ConvertLegacyRouting(LegacyRoutingInput{Outbound: []NodeOutbound{ob}}, "global")
		if err != nil || p.Draft != nil || len(p.Blockers) == 0 {
			t.Fatalf("unsupported %s accepted: %#v %v", ob.Name, p, err)
		}
	}
	// The normal direct/SOCKS configuration used by migration remains valid.
	p, err := ConvertLegacyRouting(LegacyRoutingInput{Outbound: []NodeOutbound{{Name: "proxy", Protocol: "socks", Address: "127.0.0.1", Port: 1080, Rules: []string{"example.com"}}}}, "global")
	if err != nil || p.Draft == nil {
		t.Fatalf("valid SOCKS rejected: %v %#v", err, p.Blockers)
	}
	if _, err := ParseNodeRouting([]byte(`{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct","default_outbound_id":"block"}`)); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("ambiguous JSON error %v", err)
	}
}
