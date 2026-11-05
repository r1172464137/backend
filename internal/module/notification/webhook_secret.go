package notification

// The webhook secret is what Telegram echoes back in the
// X-Telegram-Bot-Api-Secret-Token header of every webhook call. It is
// derived, never stored: it rotates automatically whenever the bot token
// changes, and a database leak cannot expose it.

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// telegramWebhookSecretPurpose separates this derivation from any other use of the bot token as
// key material; the version suffix lets the scheme change unambiguously.
const telegramWebhookSecretPurpose = "telegram-webhook:v1"

// WebhookSecret computes the webhook secret token for a bot token. The result is
// 64 hex characters, within Telegram's 1-256 length limit and its
// [A-Za-z0-9_-] alphabet.
func WebhookSecret(botToken string) string {
	mac := hmac.New(sha256.New, []byte(botToken))
	mac.Write([]byte(telegramWebhookSecretPurpose))
	return hex.EncodeToString(mac.Sum(nil))
}

// WebhookSecretEqual compares a presented secret with the expected one in constant time,
// so the comparison leaks nothing about the expected value.
func WebhookSecretEqual(presented, expected string) bool {
	return subtle.ConstantTimeCompare([]byte(presented), []byte(expected)) == 1
}
