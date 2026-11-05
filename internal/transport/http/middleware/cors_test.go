package middleware

import (
	"context"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
)

// corsRequest sends method with origin through the CORS middleware built
// for allowed and returns the response.
func corsRequest(t *testing.T, allowed []string, method, origin string) (*app.RequestContext, bool) {
	t.Helper()
	engine := server.Default()
	reached := false
	engine.GET("/v1/common/heartbeat", NewCorsMiddleware(allowed), func(_ context.Context, c *app.RequestContext) {
		reached = true
		c.String(http.StatusOK, "ok")
	})
	engine.OPTIONS("/v1/common/heartbeat", NewCorsMiddleware(allowed), func(_ context.Context, c *app.RequestContext) {
		reached = true
	})
	c := requestContext(engine, method, "/v1/common/heartbeat")
	if origin != "" {
		c.Request.Header.Set("Origin", origin)
	}
	engine.ServeHTTP(context.Background(), c)
	return c, reached
}

// Without an allowlist the middleware reflects any Origin with credentials,
// as it always did, and a request without an Origin gets the wildcard.
func TestCorsWithoutAnAllowlistReflectsTheOrigin(t *testing.T) {
	c, reached := corsRequest(t, nil, http.MethodGet, "https://anything.example")
	if !reached || c.Response.StatusCode() != http.StatusOK {
		t.Fatalf("handler reached = %v, status = %d", reached, c.Response.StatusCode())
	}
	if got := string(c.Response.Header.Peek("Access-Control-Allow-Origin")); got != "https://anything.example" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want the origin reflected", got)
	}
	if got := string(c.Response.Header.Peek("Access-Control-Allow-Credentials")); got != "true" {
		t.Fatalf("Access-Control-Allow-Credentials = %q, want true", got)
	}
	c, _ = corsRequest(t, nil, http.MethodGet, "")
	if got := string(c.Response.Header.Peek("Access-Control-Allow-Origin")); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin without an Origin = %q, want *", got)
	}
}

// With an allowlist only an origin on it, compared exactly whatever its
// case or trailing slash, is admitted, with credentials; any other origin
// gets no CORS headers, so the browser withholds the response from the
// page, while the request itself is still served. The answer varies by
// Origin, so caches are told.
func TestCorsWithAnAllowlistAdmitsOnlyListedOrigins(t *testing.T) {
	allowed := []string{"https://Panel.Example/", "http://localhost:3000"}
	for name, tc := range map[string]struct {
		origin  string
		allowed bool
	}{
		"listed":                    {"https://panel.example", true},
		"listed with port":          {"http://localhost:3000", true},
		"listed, different case":    {"https://PANEL.example", true},
		"subdomain not listed":      {"https://app.panel.example", false},
		"other scheme not listed":   {"http://panel.example", false},
		"other port not listed":     {"http://localhost:3001", false},
		"attacker origin":           {"https://evil.example", false},
		"attacker suffix lookalike": {"https://panel.example.evil.example", false},
		"null origin":               {"null", false},
	} {
		t.Run(name, func(t *testing.T) {
			c, reached := corsRequest(t, allowed, http.MethodGet, tc.origin)
			if !reached || c.Response.StatusCode() != http.StatusOK {
				t.Fatalf("handler reached = %v, status = %d; the request is served either way", reached, c.Response.StatusCode())
			}
			acao := string(c.Response.Header.Peek("Access-Control-Allow-Origin"))
			credentials := string(c.Response.Header.Peek("Access-Control-Allow-Credentials"))
			if tc.allowed && (acao != tc.origin || credentials != "true") {
				t.Fatalf("allowed origin got ACAO %q, credentials %q", acao, credentials)
			}
			if !tc.allowed && (acao != "" || credentials != "" || len(c.Response.Header.Peek("Access-Control-Allow-Methods")) != 0) {
				t.Fatalf("refused origin got ACAO %q, credentials %q", acao, credentials)
			}
			if got := string(c.Response.Header.Peek("Vary")); got != "Origin" {
				t.Fatalf("Vary = %q, want Origin", got)
			}
		})
	}
}

// A preflight is answered with 204 and never reaches the handler, for an
// admitted and for a refused origin alike; only the admitted one gets the
// CORS headers.
func TestCorsPreflightIsAnsweredWithoutTheHandler(t *testing.T) {
	for _, origin := range []string{"https://panel.example", "https://evil.example"} {
		c, reached := corsRequest(t, []string{"https://panel.example"}, http.MethodOptions, origin)
		if reached || c.Response.StatusCode() != http.StatusNoContent {
			t.Fatalf("%s: handler reached = %v, status = %d", origin, reached, c.Response.StatusCode())
		}
		acao := string(c.Response.Header.Peek("Access-Control-Allow-Origin"))
		if (origin == "https://panel.example") != (acao == origin) {
			t.Fatalf("%s: Access-Control-Allow-Origin = %q", origin, acao)
		}
	}
}
