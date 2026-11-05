// Package telegram verifies the signed result of the Telegram Login Widget
// for the Telegram sign-in method and builds the widget's authorization URL.
package telegram

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// ParseAuthDataJson parses provided json content for AuthData
func ParseAuthDataJson(content []byte) (*AuthData, error) {
	data := &AuthData{}
	if err := json.Unmarshal(content, data); err != nil {
		return nil, fmt.Errorf("unmarshaling error: %w", err)
	}
	// Keep the raw payload too: the signature covers every field Telegram
	// sent, not just the ones this struct models.
	if err := json.Unmarshal(content, &data.raw); err != nil {
		return nil, fmt.Errorf("unmarshaling error: %w", err)
	}
	return data, nil
}

// ParseAuthDataBase64 decodes provided content from base64 and parses result for AuthData.
// Telegram's tgAuthResult reaches us in whichever base64 flavour the widget
// and the surrounding URL handling produced, so all four combinations of
// alphabet (standard or URL-safe) and padding are accepted.
func ParseAuthDataBase64(content []byte) (*AuthData, error) {
	decoded, err := decodeBase64Any(string(content))
	if err != nil {
		return nil, err
	}
	return ParseAuthDataJson(decoded)
}

func decodeBase64Any(content string) ([]byte, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("base64 decoding error: content is empty")
	}
	encodings := []*base64.Encoding{
		base64.RawStdEncoding,
		base64.StdEncoding,
		base64.RawURLEncoding,
		base64.URLEncoding,
	}
	var lastErr error
	for _, enc := range encodings {
		decoded, err := enc.DecodeString(content)
		if err == nil {
			return decoded, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("base64 decoding error: %w", lastErr)
}

// ParseAndValidateBase64 parses base64 content for AuthData and validates it
func ParseAndValidateBase64(content []byte, botToken string) (*AuthData, error) {
	authData, err := ParseAuthDataBase64(content)
	if err != nil {
		return nil, err
	}
	err = authData.Validate([]byte(botToken))
	return authData, err
}

// ParseAndValidateJson parses json content for AuthData and validates it
func ParseAndValidateJson(content []byte, botToken []byte) (*AuthData, error) {
	authData, err := ParseAuthDataJson(content)
	if err != nil {
		return nil, err
	}
	err = authData.Validate(botToken)
	return authData, err
}

// BotID extracts the public bot identifier from a bot token, which Telegram
// formats as "<bot_id>:<secret>". The secret half must never leave the
// server, so a token without that shape is rejected instead of being pasted
// into a browser-facing URL.
func BotID(botToken string) (string, error) {
	id, secret, found := strings.Cut(strings.TrimSpace(botToken), ":")
	if !found || id == "" || secret == "" {
		return "", fmt.Errorf("telegram bot token is malformed: expected \"<bot_id>:<secret>\"")
	}
	return id, nil
}

// GenerateTelegramOAuthURL generates a URL for Telegram OAuth
func GenerateTelegramOAuthURL(botToken, redirect string) string {
	uri, err := BuildTelegramOAuthURL(botToken, redirect)
	if err != nil {
		return ""
	}
	return uri
}

// BuildTelegramOAuthURL is GenerateTelegramOAuthURL with the failure reason
// preserved, so callers can log why no URL could be produced.
//
// embed=0 selects the redirect flow: Telegram sends the browser back to
// return_to with the signed result in the fragment. A non-zero embed puts
// Telegram in widget mode, where it posts the result to window.opener and
// closes itself — which loses the result entirely for a full-page
// navigation, since there is no opener.
func BuildTelegramOAuthURL(botToken, redirect string) (string, error) {
	botID, err := BotID(botToken)
	if err != nil {
		return "", err
	}
	parsedURL, err := url.Parse(redirect)
	if err != nil {
		return "", fmt.Errorf("parse redirect %q: %w", redirect, err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return "", fmt.Errorf("redirect %q must be an absolute URL", redirect)
	}
	query := url.Values{
		"bot_id":         []string{botID},
		"origin":         []string{fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)},
		"embed":          []string{"0"},
		"request_access": []string{"write"},
		"return_to":      []string{redirect},
	}
	return "https://oauth.telegram.org/auth?" + query.Encode(), nil
}
