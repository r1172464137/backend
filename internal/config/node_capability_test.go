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
func TestSpiderBoundsApplyToManagementPolicies(t *testing.T) {
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
			outbound.ID = routingID("ob_", 1)
			policy := DefaultNodeRouting("test")
			policy.Outbounds = []NodeOutbound{outbound}
			_, err := NormalizeNodeRouting(policy)
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

func TestDuplicateJSONKeysAreRejected(t *testing.T) {
	if _, err := ParseNodeRouting([]byte(`{"version":3,"outbounds":[],"rules":[],"default_outbound_id":"direct","user_defaults":[],"dns":{"query_strategy":"UseIP","resolvers":[{"id":"dns_00000000-0000-4000-8000-000000000001","protocol":"system","address":"","domains":[],"outbound_id":"direct"}]},"target_ip_preference":"none","default_outbound_id":"block","user_defaults":[],"dns":{"query_strategy":"UseIP","resolvers":[{"id":"dns_00000000-0000-4000-8000-000000000001","protocol":"system","address":"","domains":[],"outbound_id":"direct"}]},"target_ip_preference":"none"}`)); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("ambiguous JSON error %v", err)
	}
}
