package telegram

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

const testBotToken = "123456:AA-test-token"

func signCheckString(t *testing.T, check string) string {
	t.Helper()
	key := sha256.Sum256([]byte(testBotToken))
	h := hmac.New(sha256.New, key[:])
	h.Write([]byte(check))
	return hex.EncodeToString(h.Sum(nil))
}

// signedPayload builds a payload the way Telegram does: every field it sends
// is part of the check string, sorted by key.
func signedPayload(t *testing.T, fields map[string]any) string {
	t.Helper()
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	// insertion-order independent
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+formatCheckValue(fields[k]))
	}
	withHash := make(map[string]any, len(fields)+1)
	for k, v := range fields {
		withHash[k] = v
	}
	withHash["hash"] = signCheckString(t, strings.Join(parts, "\n"))
	encoded, err := json.Marshal(withHash)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return string(encoded)
}

func baseFields() map[string]any {
	return map[string]any{
		"id":         float64(42),
		"first_name": "Ada",
		"username":   "ada",
		"auth_date":  float64(1785000000),
	}
}

// Telegram's tgAuthResult reaches the server in whichever base64 flavour the
// widget and the surrounding URL handling produced. Only unpadded standard
// base64 used to decode; the others failed with a misleading JSON error.
func TestParseAndValidateAcceptsEveryBase64Flavour(t *testing.T) {
	payload := signedPayload(t, baseFields())
	encodings := map[string]*base64.Encoding{
		"raw-std": base64.RawStdEncoding,
		"std":     base64.StdEncoding,
		"raw-url": base64.RawURLEncoding,
		"url":     base64.URLEncoding,
	}
	for name, enc := range encodings {
		t.Run(name, func(t *testing.T) {
			encoded := enc.EncodeToString([]byte(payload))
			data, err := ParseAndValidateBase64([]byte(encoded), testBotToken)
			if err != nil {
				t.Fatalf("ParseAndValidateBase64 error = %v, want nil", err)
			}
			if data.Id == nil || *data.Id != 42 {
				t.Fatalf("Id = %v, want 42", data.Id)
			}
		})
	}
}

// Telegram signs every field it sent. A field this struct does not model
// used to be dropped from the check string, so the hash never matched and
// the login failed.
func TestValidateCoversFieldsBeyondTheKnownStruct(t *testing.T) {
	fields := baseFields()
	fields["is_premium"] = true
	fields["photo_url"] = "https://t.me/i/ada.jpg"
	payload := signedPayload(t, fields)

	if _, err := ParseAndValidateJson([]byte(payload), []byte(testBotToken)); err != nil {
		t.Fatalf("ParseAndValidateJson error = %v, want nil", err)
	}
}

// The widget serialises absent optional fields as a JSON null or the literal
// string "null", and Telegram excludes both from its check string.
func TestValidateSkipsNullValuedFields(t *testing.T) {
	fields := baseFields()
	signed := signedPayload(t, fields)

	var payload map[string]any
	if err := json.Unmarshal([]byte(signed), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	payload["last_name"] = nil
	payload["photo_url"] = "null"
	augmented, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if _, err := ParseAndValidateJson(augmented, []byte(testBotToken)); err != nil {
		t.Fatalf("ParseAndValidateJson error = %v, want nil", err)
	}
}

func TestValidateRejectsTamperedAndUnsignedPayloads(t *testing.T) {
	payload := signedPayload(t, baseFields())

	tests := []struct {
		name    string
		payload string
		token   string
	}{
		{name: "tampered id", payload: strings.Replace(payload, `"id":42`, `"id":43`, 1), token: testBotToken},
		{name: "wrong bot token", payload: payload, token: "123456:other-token"},
		{name: "empty bot token", payload: payload, token: ""},
		{name: "no hash", payload: `{"id":42,"auth_date":1785000000}`, token: testBotToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseAndValidateJson([]byte(tt.payload), []byte(tt.token)); err == nil {
				t.Fatal("ParseAndValidateJson error = nil, want rejection")
			}
		})
	}
}

// The secret half of a bot token must never reach a browser-facing URL, so a
// token that is not "<bot_id>:<secret>" yields no URL at all.
func TestBuildTelegramOAuthURLRequiresAWellFormedBotToken(t *testing.T) {
	if _, err := BuildTelegramOAuthURL("no-colon-token", "https://panel.example.com/cb"); err == nil {
		t.Fatal("BuildTelegramOAuthURL error = nil, want malformed-token rejection")
	}
	if got := GenerateTelegramOAuthURL("no-colon-token", "https://panel.example.com/cb"); got != "" {
		t.Fatalf("GenerateTelegramOAuthURL = %q, want empty", got)
	}

	uri, err := BuildTelegramOAuthURL(testBotToken, "https://panel.example.com/cb")
	if err != nil {
		t.Fatalf("BuildTelegramOAuthURL error = %v", err)
	}
	if !strings.Contains(uri, "bot_id=123456") {
		t.Fatalf("url %q does not carry the bot id", uri)
	}
	if strings.Contains(uri, "AA-test-token") {
		t.Fatalf("url %q leaks the bot token secret", uri)
	}
}

// embed must be 0: any other value puts Telegram in widget mode, where it
// posts the result to window.opener and closes itself instead of redirecting
// back — the browser tab simply disappears and the login is lost.
func TestBuildTelegramOAuthURLUsesTheRedirectFlow(t *testing.T) {
	uri, err := BuildTelegramOAuthURL(testBotToken, "https://panel.example.com/bind/telegram")
	if err != nil {
		t.Fatalf("BuildTelegramOAuthURL error = %v", err)
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	query := parsed.Query()
	if got := query.Get("embed"); got != "0" {
		t.Fatalf("embed = %q, want \"0\" (redirect flow)", got)
	}
	if got := query.Get("return_to"); got != "https://panel.example.com/bind/telegram" {
		t.Fatalf("return_to = %q", got)
	}
	if got := query.Get("origin"); got != "https://panel.example.com" {
		t.Fatalf("origin = %q, want the redirect's scheme and host", got)
	}
}
