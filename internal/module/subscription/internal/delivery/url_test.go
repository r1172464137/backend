package delivery

import (
	"testing"
)

func TestSubscribeURLPreservesRequestURI(t *testing.T) {
	tests := []struct {
		name   string
		domain string
		uri    string
		want   string
	}{
		{"request host", "", "/subscribe/test-token?client=clash", "https://request.example.test/subscribe/test-token?client=clash"},
		{"custom path", "", "/custom/test-token?client=sing-box", "https://request.example.test/custom/test-token?client=sing-box"},
		{"custom domain", "sub.example.test", "/subscribe/test-token", "https://sub.example.test/subscribe/test-token"},
		{"first configured domain", "first.example.test\nsecond.example.test", "/subscribe/test-token", "https://first.example.test/subscribe/test-token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Deps{ConfigSnapshot: func() Config { return Config{SubscribeDomain: tt.domain} }}.config()
			if got := getSubscribeV2URL(cfg, RequestMeta{Host: "request.example.test", RequestURI: tt.uri}); got != tt.want {
				t.Fatalf("getSubscribeV2URL = %q, want %q", got, tt.want)
			}
		})
	}
}
