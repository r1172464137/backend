package facebook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAppSecretProofSignsTokenWithClientSecret(t *testing.T) {
	client := New(&Config{ClientID: "id", ClientSecret: "secret"})

	// echo -n "token" | openssl dgst -sha256 -hmac "secret"
	want := "e941110e3d2bfe82621f0e3e1434730d7305d106c5f68c87165d0b27a4611a4a"
	if got := client.appSecretProof("token"); got != want {
		t.Fatalf("appSecretProof = %q, want %q", got, want)
	}
}

func TestGetUserInfoParsesGraphResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("fields"); got != "id,name,email,picture.type(large)" {
			t.Errorf("fields = %q", got)
		}
		if r.URL.Query().Get("appsecret_proof") == "" {
			t.Error("appsecret_proof missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"42","name":"Test User","email":"user@example.com","picture":{"data":{"url":"https://cdn.example/p.jpg"}}}`))
	}))
	t.Cleanup(server.Close)
	original := userInfoURL
	userInfoURL = server.URL
	t.Cleanup(func() { userInfoURL = original })

	info, err := New(&Config{ClientSecret: "secret"}).GetUserInfo(context.Background(), "token")
	if err != nil {
		t.Fatalf("GetUserInfo error = %v", err)
	}
	if info.OpenID != "42" || info.Email != "user@example.com" || info.Picture != "https://cdn.example/p.jpg" {
		t.Fatalf("info = %+v", info)
	}
}

func TestGetUserInfoRejectsMissingUserID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	original := userInfoURL
	userInfoURL = server.URL
	t.Cleanup(func() { userInfoURL = original })

	if _, err := New(&Config{ClientSecret: "secret"}).GetUserInfo(context.Background(), "token"); err == nil {
		t.Fatal("expected error for response without user id")
	}
}
