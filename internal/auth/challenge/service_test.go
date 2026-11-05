package challenge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newSiteVerify(t *testing.T, handler http.HandlerFunc) Service {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return New(Config{Secret: "site-secret", Timeout: 200 * time.Millisecond, URL: server.URL})
}

func TestVerifySendsTheTokenAndReportsCloudflaresVerdict(t *testing.T) {
	for _, success := range []bool{true, false} {
		verifier := newSiteVerify(t, func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse form: %v", err)
			}
			if r.FormValue("secret") != "site-secret" || r.FormValue("response") != "client-token" || r.FormValue("remoteip") != "203.0.113.7" {
				t.Errorf("form = %v", r.MultipartForm.Value)
			}
			if success {
				_, _ = w.Write([]byte(`{"success":true}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":false,"error-codes":["invalid-input-response"]}`))
		})
		ok, err := verifier.Verify(context.Background(), "client-token", "203.0.113.7")
		if err != nil || ok != success {
			t.Fatalf("Verify() = %v, %v; want %v", ok, err, success)
		}
	}
}

func TestVerifyFailsWhenTheCheckCannotBeDone(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"server error": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) },
		"malformed":    func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{`)) },
		"slow": func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(time.Second):
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			ok, err := newSiteVerify(t, handler).Verify(context.Background(), "client-token", "203.0.113.7")
			if err == nil || ok {
				t.Fatalf("Verify() = %v, %v; want a failure", ok, err)
			}
		})
	}
}

func TestNewDefaultsToCloudflare(t *testing.T) {
	s := New(Config{Secret: "site-secret"}).(*service)
	if s.url != SiteVerifyURL || s.timeout != 10*time.Second {
		t.Fatalf("defaults = %q, %v", s.url, s.timeout)
	}
}
