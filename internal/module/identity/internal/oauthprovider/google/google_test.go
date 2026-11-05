package google

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func stubUserInfo(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	original := userInfoURL
	userInfoURL = server.URL
	t.Cleanup(func() { userInfoURL = original })
}

func TestGetUserInfoParsesProfile(t *testing.T) {
	stubUserInfo(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Errorf("Authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"id":"1001","email":"user@example.com","picture":"https://cdn.example/p.jpg","verified_email":"true"}`))
	})

	info, err := New(&Config{}).GetUserInfo(context.Background(), "access-token")
	if err != nil {
		t.Fatalf("GetUserInfo error = %v", err)
	}
	if info.OpenID != "1001" || info.Email != "user@example.com" || !info.VerifiedEmail || info.Picture != "https://cdn.example/p.jpg" {
		t.Fatalf("info = %+v", info)
	}
}

// A refused or empty answer is a failed sign-in, not an account without an
// id.
func TestGetUserInfoRejectsFailedResponses(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"unauthorized": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"id":"1001"}`))
		},
		"no id":     func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) },
		"malformed": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{`)) },
	} {
		t.Run(name, func(t *testing.T) {
			stubUserInfo(t, handler)
			if info, err := New(&Config{}).GetUserInfo(context.Background(), "access-token"); err == nil {
				t.Fatalf("GetUserInfo = %+v, want an error", info)
			}
		})
	}
}

// The request carries the caller's deadline instead of waiting forever.
func TestGetUserInfoHonoursTheCallerDeadline(t *testing.T) {
	stubUserInfo(t, func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := New(&Config{}).GetUserInfo(ctx, "access-token"); err == nil {
		t.Fatal("GetUserInfo succeeded past its deadline")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("GetUserInfo returned after %v, want the 50ms deadline", elapsed)
	}
}
