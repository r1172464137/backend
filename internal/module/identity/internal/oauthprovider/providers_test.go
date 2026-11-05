package oauthprovider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"
)

const botToken = "123456:AA-test-token"

// widgetResult signs fields the way the Telegram Login widget does and
// encodes them as the tgAuthResult the browser brings back.
func widgetResult(t *testing.T, fields map[string]any) string {
	t.Helper()
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("%s=%v", key, fields[key]))
	}
	secret := sha256.Sum256([]byte(botToken))
	mac := hmac.New(sha256.New, secret[:])
	mac.Write([]byte(strings.Join(lines, "\n")))
	signed := map[string]any{"hash": hex.EncodeToString(mac.Sum(nil))}
	for key, value := range fields {
		signed[key] = value
	}
	encoded, err := json.Marshal(signed)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func newTelegramProvider(t *testing.T) Provider {
	t.Helper()
	provider, err := newTelegram(`{"bot_token":"` + botToken + `"}`)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func TestTelegramVouchesForASignedFreshResult(t *testing.T) {
	result := widgetResult(t, map[string]any{"id": 42, "auth_date": time.Now().Unix(), "photo_url": "https://t.me/p.jpg"})
	identity, err := newTelegramProvider(t).Identify(context.Background(), Callback{Fields: map[string]any{"tgAuthResult": result}})
	if err != nil {
		t.Fatalf("Identify() error = %v", err)
	}
	if identity.Subject != "42" || identity.Avatar != "https://t.me/p.jpg" || identity.Email != "" {
		t.Fatalf("identity = %+v", identity)
	}
	if !strings.HasPrefix(identity.ReplayKey, "auth:telegram_callback:") || strings.Contains(identity.ReplayKey, result) {
		t.Fatalf("replay key %q must be a fingerprint of the result", identity.ReplayKey)
	}
}

func TestTelegramRefusesWhatIsNotAFreshSignedResult(t *testing.T) {
	now := time.Now().Unix()
	tampered := widgetResult(t, map[string]any{"id": 42, "auth_date": now})
	decoded, _ := base64.RawURLEncoding.DecodeString(tampered)
	decoded = []byte(strings.Replace(string(decoded), `"id":42`, `"id":43`, 1))
	for name, tc := range map[string]struct {
		fields map[string]any
		want   error
	}{
		"missing":  {map[string]any{}, ErrIncompleteCallback},
		"tampered": {map[string]any{"tgAuthResult": base64.RawURLEncoding.EncodeToString(decoded)}, ErrInvalidCallback},
		"no id":    {map[string]any{"tgAuthResult": widgetResult(t, map[string]any{"auth_date": now})}, ErrIncompleteCallback},
		"expired": {map[string]any{"tgAuthResult": widgetResult(t, map[string]any{
			"id": 42, "auth_date": now - int64(CallbackLifetime.Seconds()) - 1,
		})}, ErrExpiredCallback},
		"from the future": {map[string]any{"tgAuthResult": widgetResult(t, map[string]any{
			"id": 42, "auth_date": now + int64(callbackClockSkew.Seconds()) + 60,
		})}, ErrExpiredCallback},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := newTelegramProvider(t).Identify(context.Background(), Callback{Fields: tc.fields})
			if !errors.Is(err, tc.want) {
				t.Fatalf("Identify() error = %v, want %v", err, tc.want)
			}
		})
	}
}

// Every provider is built from its stored configuration; one that does not
// parse is a misconfiguration.
func TestProvidersRefuseConfigurationsThatDoNotParse(t *testing.T) {
	for name, method := range Default() {
		if _, err := method.New(`{`); !errors.Is(err, ErrMisconfigured) {
			t.Errorf("%s: New() error = %v, want ErrMisconfigured", name, err)
		}
		if _, err := method.New(`{}`); err != nil {
			t.Errorf("%s: New() error = %v for an empty configuration", name, err)
		}
	}
}

// The authorization URLs keep the exact shape clients were given before.
func TestAuthorizationURLs(t *testing.T) {
	google, _ := newGoogle(`{"client_id":"google-id"}`)
	uri, _ := google.AuthURL("https://panel.example/oauth", "state-1")
	parsed, _ := url.Parse(uri)
	if q := parsed.Query(); parsed.Host != "accounts.google.com" || q.Get("state") != "state-1" || q.Get("access_type") != "offline" ||
		q.Get("redirect_uri") != "https://panel.example/oauth" || q.Get("client_id") != "google-id" {
		t.Fatalf("google url = %q", uri)
	}
	facebook, _ := newFacebook(`{"client_id":"fb-id"}`)
	uri, _ = facebook.AuthURL("https://panel.example/oauth", "state-2")
	if parsed, _ = url.Parse(uri); parsed.Query().Get("access_type") != "" || parsed.Query().Get("state") != "state-2" {
		t.Fatalf("facebook url = %q", uri)
	}
	apple, _ := newApple(`{"client_id":"com.example.panel","redirect_url":"https://api.panel.example"}`)
	uri, _ = apple.AuthURL("https://panel.example/oauth", "state-3")
	want := "https://appleid.apple.com/auth/authorize?client_id=com.example.panel&redirect_uri=https://api.panel.example/v1/auth/oauth/callback/apple&response_type=code&state=state-3&scope=name email&response_mode=form_post"
	if uri != want {
		t.Fatalf("apple url = %q, want %q", uri, want)
	}
}

// Apple sends email_verified as a string.
func TestClaimBool(t *testing.T) {
	for value, want := range map[any]bool{true: true, "true": true, " TRUE ": true, false: false, "false": false, 1: false} {
		if got := claimBool(value); got != want {
			t.Errorf("claimBool(%v) = %v, want %v", value, got, want)
		}
	}
	if claimBool(nil) {
		t.Error("claimBool(nil) = true")
	}
}

// An Apple private key that does not parse only fails the sign-in, not the
// authorization URL.
func TestAppleWithAnUnusableKeyIsMisconfigured(t *testing.T) {
	apple, err := newApple(`{"client_id":"com.example.panel","client_secret":"not a key"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := apple.AuthURL("https://panel.example/oauth", "state"); err != nil {
		t.Fatalf("AuthURL() error = %v", err)
	}
	if _, err := apple.Identify(context.Background(), Callback{Code: "code"}); !errors.Is(err, ErrMisconfigured) {
		t.Fatalf("Identify() error = %v, want ErrMisconfigured", err)
	}
}
