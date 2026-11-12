package delivery

import (
	"encoding/base64"
	"testing"

	"github.com/perfect-panel/server/internal/module/platform/entity/client"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

func apps() []*client.SubscribeApplication {
	return []*client.SubscribeApplication{
		{Id: 1, Name: "Clash", UserAgent: "clash", OutputFormat: "yaml"},
		{Id: 2, Name: "Default", UserAgent: "", OutputFormat: "base64", IsDefault: true},
		{Id: 3, Name: "Mihomo", UserAgent: "clash.meta", OutputFormat: "yaml"},
		{Id: 4, Name: "Stash", UserAgent: "stash", OutputFormat: "yaml"},
		{Id: 5, Name: "Surge", UserAgent: "surge", OutputFormat: "conf"},
		{Id: 6, Name: "Surge dup", UserAgent: "SURGE", OutputFormat: "conf"},
		{Id: 7, Name: "v2rayN", UserAgent: "v2rayn", OutputFormat: "base64"},
	}
}

// CONTRACT 5.1: app id, then the longest case-insensitive UA keyword (ties
// to the smallest id; empty keywords and empty UAs never match), then the
// single default.
func TestSelectApplication(t *testing.T) {
	cases := []struct {
		app, ua string
		want    int64
	}{
		{"", "v2rayN/7.0", 7},
		{"", "ClashMetaForAndroid clash.meta", 3}, // longest keyword wins
		{"", "clash-verge/2.0", 1},
		{"", "Stash/2.4 clash", 4}, // Stash also names clash
		{"", "Surge iOS/3000", 5},  // equal keywords: smallest id
		{"", "", 2},                // empty UA: default, not the empty keyword
		{"", "curl/8", 2},
		{"5", "v2rayN/7.0", 5}, // explicit app beats UA
	}
	for _, c := range cases {
		got, err := selectApplication(apps(), c.app, c.ua)
		if err != nil || got.Id != c.want {
			t.Errorf("app=%q ua=%q: got %+v %v, want %d", c.app, c.ua, got, err, c.want)
		}
	}
	for _, bad := range []string{"99", "0", "-1", "x"} {
		if _, err := selectApplication(apps(), bad, "v2rayN"); !isCode(err, xerr.InvalidParams) {
			t.Errorf("app=%s: %v, want InvalidParams without fallback", bad, err)
		}
	}
	noDefault := apps()
	noDefault[1].IsDefault = false
	if _, err := selectApplication(noDefault, "", "curl/8"); !isCode(err, xerr.InvalidParams) {
		t.Errorf("no default: %v", err)
	}
}

func isCode(err error, code uint32) bool {
	var c *xerr.CodeError
	return errors.As(errors.Cause(err), &c) && c.GetErrCode() == code
}

func TestValidateOutput(t *testing.T) {
	b64 := func(s string) []byte { return []byte(base64.StdEncoding.EncodeToString([]byte(s))) }
	good := map[string][]byte{
		"base64": b64("REMARKS=自用面板\nSTATUS=ok\nvless://u@h:443?security=reality#a\ntrojan://p@h:443#b\n"),
		"yaml":   []byte("proxies:\n  - name: a\n"),
		"json":   []byte(`{"outbounds":[]}`),
		"conf":   []byte("[Proxy]\na = trojan, h, 443\n"),
	}
	for f, body := range good {
		if err := validateOutput(f, body); err != nil {
			t.Errorf("%s rejected: %v", f, err)
		}
	}
	bad := []struct {
		name, format string
		body         []byte
	}{
		{"not base64", "base64", []byte("vless://plain")},
		{"not a share URI", "base64", b64("[Proxy]\na = trojan\n")},
		{"empty", "base64", b64("\n")},
		{"header only", "base64", b64("REMARKS=x\n")},
		{"lowercase key", "base64", b64("remarks=x\nvless://u@h:1#a\n")},
		{"list", "yaml", []byte("- a\n- b\n")},
		{"template marker", "yaml", []byte("proxies: {{ .Proxies }}\n")},
		{"array", "json", []byte(`[1]`)},
		{"no value", "conf", []byte("a = <no value>\n")},
	}
	for _, c := range bad {
		if err := validateOutput(c.format, c.body); err == nil {
			t.Errorf("%s %s accepted", c.format, c.name)
		}
	}
}

func TestAcceptableToken(t *testing.T) {
	for _, tok := range []string{"", "short", "with space1", "tab\tinside1", "nul\x00inside"} {
		if acceptableToken(tok) {
			t.Errorf("%q accepted", tok)
		}
	}
	if !acceptableToken("abcdefgh12345678abcdefgh12345678") {
		t.Error("issued-shape token refused")
	}
}
