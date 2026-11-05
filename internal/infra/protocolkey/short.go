package protocolkey

import (
	"crypto/sha1" //nolint:gosec // G505: REALITY short IDs are derived with SHA-1 by convention; the output must not change
	"fmt"
)

// GenerateShortID derives a REALITY short ID from the private key: the first
// eight hex digits of its SHA-1, so the same key keeps the same short ID.
func GenerateShortID(privateKey string) string {
	hash := sha1.New() //nolint:gosec // G401: see the import note
	hash.Write([]byte(privateKey))
	hashValue := hash.Sum(nil)
	hashString := fmt.Sprintf("%x", hashValue)
	return hashString[:8]
}
