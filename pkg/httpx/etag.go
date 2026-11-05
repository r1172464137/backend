package httpx

import (
	"crypto/sha256"
	"encoding/hex"
)

// GenerateETag returns the ETag of a response body, its hex SHA-256, so a
// client that sends it back in If-None-Match can be answered 304 Not
// Modified.
func GenerateETag(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
