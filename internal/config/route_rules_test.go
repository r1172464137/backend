package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeDomainRules(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    []string
		wantErr string
	}{
		{
			name: "canonical forms and stable deduplication",
			input: []string{
				" example.com. ", "full:example.com", "FuLl:Example.COM.", "KEYWORD:tracker", "SuFfIx:example.org.", "DOMAIN:example.net.",
				`ReGeX:(?i)(^|\.)tracker\.`, `REGEXP:^announce[0-9]+\.example$`, "GeoSite:category-pt",
			},
			want: []string{
				"full:example.com", "full:Example.COM", "keyword:tracker", "domain:example.org", "domain:example.net",
				`regexp:(?i)(^|\.)tracker\.`, `regexp:^announce[0-9]+\.example$`, "geosite:category-pt",
			},
		},
		{name: "unknown prefix", input: []string{"geoip:private"}, wantErr: `block line 1: unknown domain rule prefix "geoip"`},
		{name: "lookahead is not RE2", input: []string{`regex:tracker(?=\.example)`}, wantErr: "block line 1: regular expression is not supported by RE2"},
		{name: "lookbehind is not RE2", input: []string{`regexp:(?<=tracker)\.example`}, wantErr: "block line 1: regular expression is not supported by RE2"},
		{name: "backreference is not RE2", input: []string{`regex:(tracker)\1`}, wantErr: "block line 1: regular expression is not supported by RE2"},
		{name: "CIDR belongs in its own field", input: []string{"10.0.0.0/8"}, wantErr: "block line 1:"},
		{name: "address belongs in its own field", input: []string{"127.0.0.1"}, wantErr: "block line 1: \"127.0.0.1\" is an IP address"},
		{name: "repeated root dots", input: []string{"example.com.."}, wantErr: "block line 1: \"example.com..\" has repeated trailing DNS root dots"},
		{name: "keyword whitespace", input: []string{"keyword:two words"}, wantErr: "block line 1: keyword must not contain whitespace or control characters"},
		{name: "embedded newline", input: []string{"example.com\nother.example"}, wantErr: "block line 1: contains a control character"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeDomainRules("block", tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("NormalizeDomainRules() error = %v, want containing %q", err, tt.wantErr)
				}
				var ruleErr *RouteRuleError
				if !errors.As(err, &ruleErr) || ruleErr.Field != "block" || ruleErr.Line != 1 {
					t.Fatalf("error = %#v, want block line 1 RouteRuleError", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeDomainRules() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("NormalizeDomainRules() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestNormalizeIPRules(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    []string
		wantErr string
	}{
		{
			name:  "GeoIP IPv4 IPv6 and masked CIDR",
			input: []string{" GEOIP:private ", "127.0.0.1", "10.2.3.4/8", "fc00::1/7", "2001:db8::1"},
			want:  []string{"geoip:private", "127.0.0.1", "10.0.0.0/8", "fc00::/7", "2001:db8::1"},
		},
		{name: "invalid mask", input: []string{"10.0.0.0/33"}, wantErr: `block_ips line 1: "10.0.0.0/33" is not a valid CIDR prefix`},
		{name: "port is rejected", input: []string{"127.0.0.1:443"}, wantErr: `block_ips line 1: unknown IP rule prefix "127.0.0.1"`},
		{name: "empty GeoIP", input: []string{"geoip:"}, wantErr: "block_ips line 1: GeoIP label"},
		{name: "unsafe GeoIP", input: []string{"geoip:cn/../private"}, wantErr: "block_ips line 1: GeoIP label"},
		{name: "unknown prefix", input: []string{"geosite:cn"}, wantErr: `block_ips line 1: unknown IP rule prefix "geosite"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeIPRules("block_ips", tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("NormalizeIPRules() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeIPRules() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("NormalizeIPRules() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestNormalizeProtocolRules(t *testing.T) {
	got, err := NormalizeProtocolRules("block_protocols", []string{" BitTorrent ", "BITTORRENT"})
	if err != nil {
		t.Fatalf("NormalizeProtocolRules() error = %v", err)
	}
	if !reflect.DeepEqual(got, []string{"bittorrent"}) {
		t.Fatalf("NormalizeProtocolRules() = %#v, want [bittorrent]", got)
	}

	_, err = NormalizeProtocolRules("outbound[2].protocol_rules", []string{"http"})
	if err == nil || !strings.Contains(err.Error(), `outbound[2].protocol_rules line 1: unsupported protocol "http"`) {
		t.Fatalf("NormalizeProtocolRules() error = %v, want contextual allowlist error", err)
	}
}

func TestCrosswireCanonicalVectors(t *testing.T) {
	domain, err := NormalizeDomainRules("block", []string{
		"Example.COM.", "FULL:tracker.example.", "keyword:bittorrent", "suffix:example.org.", "domain:canonical.example.",
		`regex:(?i)(^|\.)tracker\.`, `regexp:^announce\.`, "geosite:category-ai-!cn", "geosite:cn@ads", "geosite:cn@!ads@foo_bar",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantDomain := []string{
		"full:Example.COM", "full:tracker.example", "keyword:bittorrent", "domain:example.org", "domain:canonical.example",
		`regexp:(?i)(^|\.)tracker\.`, `regexp:^announce\.`, "geosite:category-ai-!cn", "geosite:cn@ads", "geosite:cn@!ads@foo_bar",
	}
	if !reflect.DeepEqual(domain, wantDomain) {
		t.Fatalf("domain crosswire vector = %#v, want %#v", domain, wantDomain)
	}
	ips, err := NormalizeIPRules("block_ips", []string{"geoip:private", "geoip:cn_test-1", "10.2.3.4/8", "2001:db8::1"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ips, []string{"geoip:private", "geoip:cn_test-1", "10.0.0.0/8", "2001:db8::1"}) {
		t.Fatalf("IP crosswire vector = %#v", ips)
	}
	for _, invalid := range []string{"geoip:cn.test", "geoip:cn!test", "geoip:cn@test"} {
		if _, err := NormalizeIPRules("block_ips", []string{invalid}); err == nil {
			t.Errorf("NormalizeIPRules(%q) unexpectedly succeeded", invalid)
		}
	}
}

func TestDomainRulesRejectCrossCategoryAndMalformedExactValues(t *testing.T) {
	for _, value := range []string{"bittorrent", "bittorrent.", "full:BitTorrent", "full:BitTorrent.", "suffix:bittorrent", "domain:bittorrent."} {
		if _, err := NormalizeDomainRules("block", []string{value}); err == nil || !strings.Contains(err.Error(), "belongs in a protocol_rules/block_protocols field") {
			t.Errorf("NormalizeDomainRules(%q) error = %v, want protocol-category error", value, err)
		}
	}
	for _, value := range []string{"127.0.0.1", "full:2001:db8::1", "10.0.0.0/8"} {
		if _, err := NormalizeDomainRules("block", []string{value}); err == nil || !strings.Contains(err.Error(), "belongs in an IP rule field") {
			t.Errorf("NormalizeDomainRules(%q) error = %v, want IP-category error", value, err)
		}
	}
	for _, value := range []string{"-bad.example", "bad-.example", "bad..example", "bad_label.example", ".example"} {
		if _, err := NormalizeDomainRules("block", []string{value}); err == nil || !strings.Contains(err.Error(), "malformed") {
			t.Errorf("NormalizeDomainRules(%q) error = %v, want malformed-label error", value, err)
		}
	}
}

type matchOutbound struct {
	Name          string   `json:"name"`
	Protocol      string   `json:"protocol"`
	Address       string   `json:"address"`
	Rules         []string `json:"rules"`
	IPRules       []string `json:"ip_rules"`
	ProtocolRules []string `json:"protocol_rules"`
}

func matchInputs(values ...matchOutbound) []LegacyOutboundMatchInput {
	result := make([]LegacyOutboundMatchInput, len(values))
	for index := range values {
		result[index] = LegacyOutboundMatchInput{Name: values[index].Name, Value: values[index]}
	}
	return result
}

func TestMatchLegacyOutboundsDeterministicScenarios(t *testing.T) {
	a := matchOutbound{Name: "a", Protocol: "direct", Address: "a.example", Rules: []string{"geosite:cn"}, IPRules: []string{"geoip:cn"}}
	b := matchOutbound{Name: "b", Protocol: "socks", Address: "b.example", Rules: []string{"full:b.example"}, ProtocolRules: []string{"bittorrent"}}
	c := matchOutbound{Name: "c", Protocol: "direct", Address: "c.example"}

	t.Run("rename and reorder uses unique legacy fingerprint", func(t *testing.T) {
		renamedA := a
		renamedA.Name = "renamed-a"
		matches, err := MatchLegacyOutbounds("outbound", matchInputs(b, renamedA), matchInputs(a, b))
		if err != nil || !reflect.DeepEqual(matches, []int{1, 0}) {
			t.Fatalf("matches = %#v, error = %v", matches, err)
		}
	})
	t.Run("insertion is new and existing name remains stable", func(t *testing.T) {
		inserted := matchOutbound{Name: "inserted", Protocol: "vless", Address: "new.example"}
		matches, err := MatchLegacyOutbounds("outbound", matchInputs(inserted, a), matchInputs(a))
		if err != nil || !reflect.DeepEqual(matches, []int{-1, 0}) {
			t.Fatalf("matches = %#v, error = %v", matches, err)
		}
	})
	t.Run("rename plus edit is positional only with unchanged anchored sequence", func(t *testing.T) {
		editedB := b
		editedB.Name = "renamed-b"
		editedB.Address = "edited.example"
		matches, err := MatchLegacyOutbounds("outbound", matchInputs(a, editedB), matchInputs(a, b))
		if err != nil || !reflect.DeepEqual(matches, []int{0, 1}) {
			t.Fatalf("matches = %#v, error = %v", matches, err)
		}
	})
	t.Run("rename plus edit and reorder is ambiguous", func(t *testing.T) {
		editedB := b
		editedB.Name = "renamed-b"
		editedB.Address = "edited.example"
		if _, err := MatchLegacyOutbounds("outbound", matchInputs(editedB, a), matchInputs(a, b)); err == nil || !strings.Contains(err.Error(), "upgraded client") {
			t.Fatalf("error = %v, want client-upgrade ambiguity", err)
		}
	})
	t.Run("single rename plus edit has no positional evidence", func(t *testing.T) {
		editedA := a
		editedA.Name = "renamed-a"
		editedA.Address = "edited.example"
		if _, err := MatchLegacyOutbounds("outbound", matchInputs(editedA), matchInputs(a)); err == nil {
			t.Fatal("single ambiguous replacement unexpectedly matched")
		}
	})
	t.Run("deletion plus reorder never shifts hidden fields", func(t *testing.T) {
		matches, err := MatchLegacyOutbounds("outbound", matchInputs(c, a), matchInputs(a, b, c))
		if err != nil || !reflect.DeepEqual(matches, []int{2, 0}) {
			t.Fatalf("matches = %#v, error = %v", matches, err)
		}
	})
}

func TestNormalizeOutboundNamesRejectsBlankDuplicateAndCount(t *testing.T) {
	for _, names := range [][]string{{""}, {"a", " a "}} {
		if _, err := NormalizeOutboundNames("outbound", names); err == nil {
			t.Fatalf("NormalizeOutboundNames(%q) unexpectedly succeeded", names)
		}
	}
	names := make([]string, MaxNodeOutbounds+1)
	for index := range names {
		names[index] = fmt.Sprintf("outbound-%d", index)
	}
	if _, err := NormalizeOutboundNames("outbound", names); err == nil || !strings.Contains(err.Error(), "more than 128") {
		t.Fatalf("outbound count error = %v", err)
	}
	if _, err := NormalizeOutboundProtocols("outbound", []string{"direct", " "}); err == nil || !strings.Contains(err.Error(), "line 2: outbound protocol must not be blank") {
		t.Fatalf("outbound protocol error = %v", err)
	}
}

func TestPersistedJSONSizeUsesFinalEscapedBytes(t *testing.T) {
	if err := ValidateSerializedJSON("block", strings.Repeat("x", MaxPersistedJSONBytes-1)); err != nil {
		t.Fatalf("below-limit serialized JSON rejected: %v", err)
	}
	if err := ValidateSerializedJSON("block", strings.Repeat("x", MaxPersistedJSONBytes)); err == nil {
		t.Fatal("at-limit serialized JSON unexpectedly accepted")
	}

	quotes := strings.Repeat(`"`, (MaxPersistedJSONBytes-2)/2)
	encoded, err := json.Marshal(quotes)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != MaxPersistedJSONBytes {
		t.Fatalf("escaped fixture length = %d, want %d", len(encoded), MaxPersistedJSONBytes)
	}
	if err := ValidatePersistedJSON("outbound", quotes); err == nil || !strings.Contains(err.Error(), "serialized JSON") {
		t.Fatalf("escaped boundary error = %v", err)
	}
	if err := ValidatePersistedJSON("outbound", quotes[:len(quotes)-1]); err != nil {
		t.Fatalf("escaped below-boundary JSON rejected: %v", err)
	}
}

func TestOutboundAggregateLimits(t *testing.T) {
	if err := ValidateOutboundAggregate("outbound", MaxNodeOutbounds, MaxOutboundAggregateRules, MaxOutboundAggregateRuleBytes); err != nil {
		t.Fatalf("limits at boundary rejected: %v", err)
	}
	for _, test := range []struct{ count, rules, bytes int }{
		{MaxNodeOutbounds + 1, 0, 0}, {0, MaxOutboundAggregateRules + 1, 0}, {0, 0, MaxOutboundAggregateRuleBytes + 1},
	} {
		if err := ValidateOutboundAggregate("outbound", test.count, test.rules, test.bytes); err == nil {
			t.Fatalf("aggregate limits %#v unexpectedly accepted", test)
		}
	}
}

func TestNormalizeRouteRuleBounds(t *testing.T) {
	// regex: gains one byte when canonicalized to regexp:, so the limit is
	// measured on the final value rather than the submitted spelling.
	atLimit := "regex:" + strings.Repeat("x", MaxRouteRuleBytes-len("regexp:"))
	got, err := NormalizeDomainRules("block", []string{atLimit})
	if err != nil {
		t.Fatalf("canonical boundary rule error=%v", err)
	}
	if len(got) != 1 || len(got[0]) != MaxRouteRuleBytes {
		t.Fatalf("canonical boundary rule = %#v", got)
	}
	tooLong := atLimit + "x"
	if _, err := NormalizeDomainRules("block", []string{tooLong}); err == nil || !strings.Contains(err.Error(), "canonical rule exceeds 4096 bytes") {
		t.Fatalf("long canonical rule error = %v", err)
	}
	canonicalDuplicates, err := NormalizeDomainRules("block", []string{"example.com", "example.com.", "full:example.com"})
	if err != nil || !reflect.DeepEqual(canonicalDuplicates, []string{"full:example.com"}) {
		t.Fatalf("canonical deduplication = %#v, error = %v", canonicalDuplicates, err)
	}

	values := make([]string, MaxRouteRules+1)
	for index := range values {
		values[index] = fmt.Sprintf("host-%d.example", index)
	}
	if _, err := NormalizeDomainRules("block", values); err == nil || !strings.Contains(err.Error(), "contains more than 1024 rules") {
		t.Fatalf("rule count error = %v", err)
	}
}
