package protocolkey

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// GenerateCipher derives a fixed-length key from serverKey: the first length
// hex digits (at most 64) of the HMAC-SHA256 of an empty message keyed with
// serverKey. The same serverKey always gives the same key.
func GenerateCipher(serverKey string, length int) string {
	h := hmac.New(sha256.New, []byte(serverKey))
	hash := h.Sum(nil)
	hashStr := hex.EncodeToString(hash)
	// Prevent overflow
	if length > len(hashStr) {
		length = len(hashStr)
	}
	return hashStr[:length]
}
