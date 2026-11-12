package bootstrap

import (
	"testing"

	"github.com/perfect-panel/server/internal/module/platform/entity/client"
)

func app(id int64, name, ua, format string, def bool) *client.SubscribeApplication {
	return &client.SubscribeApplication{Id: id, Name: name, UserAgent: ua, OutputFormat: format, IsDefault: def}
}

func defaultsOf(apps []*client.SubscribeApplication) (out []*client.SubscribeApplication) {
	for _, a := range apps {
		if a.IsDefault {
			out = append(out, a)
		}
	}
	return out
}

// MIGRATION 5.2: the production shape (five applications, all default)
// keeps Default/base64; ambiguous data stops instead of picking by order.
func TestChooseDefaultApplication(t *testing.T) {
	cases := []struct {
		name string
		apps []*client.SubscribeApplication
		want int64
	}{
		{"production five defaults", []*client.SubscribeApplication{
			app(1, "Clash", "clash", "yaml", true), app(2, "Default", "", "base64", true),
			app(3, "Surge", "surge", "conf", true), app(4, "sing-box", "sing-box", "json", true),
			app(5, "Shadowrocket", "shadowrocket", "base64", true)}, 2},
		{"v2ray among defaults", []*client.SubscribeApplication{
			app(1, "v2rayN", "v2rayn", "base64", true), app(2, "Rocket", "rocket", "base64", true)}, 1},
		{"zero defaults single base64", []*client.SubscribeApplication{
			app(1, "Clash", "clash", "yaml", false), app(2, "Links", "x", "base64", false)}, 2},
		{"ambiguous", []*client.SubscribeApplication{
			app(1, "A", "a", "base64", true), app(2, "B", "b", "base64", true)}, 0},
		{"Default not base64 is not preferred", []*client.SubscribeApplication{
			app(1, "Default", "", "yaml", true), app(2, "B", "b", "base64", true)}, 2},
	}
	for _, c := range cases {
		got := chooseDefaultApplication(c.apps, defaultsOf(c.apps))
		if (got == nil && c.want != 0) || (got != nil && got.Id != c.want) {
			t.Errorf("%s: got %v, want %d", c.name, got, c.want)
		}
	}
}
