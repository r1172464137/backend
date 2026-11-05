package protocolkey

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math/rand"
)

// FixedUniqueString derives from s a string of length distinct characters
// of alphabet (letters and digits when empty), the same for the same s. The
// short code of a subscription, the first label of its host name in
// pan-domain mode, is derived from the subscription token this way, so the
// output must not change.
func FixedUniqueString(s string, length int, alphabet string) (string, error) {
	if alphabet == "" {
		alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	}
	if length <= 0 {
		return "", errors.New("length must be > 0")
	}
	if length > len(alphabet) {
		return "", errors.New("length greater than available unique characters")
	}

	// Seed the shuffle from the first 8 bytes of the SHA-256 of s, so it is
	// deterministic.
	hash := sha256.Sum256([]byte(s))
	seed := int64(binary.LittleEndian.Uint64(hash[:8]))

	r := rand.New(rand.NewSource(seed)) //nolint:gosec // G404: a deterministic shuffle seeded from SHA-256, not a source of randomness

	// Copy alphabet to mutable array
	data := []rune(alphabet)

	// Deterministic shuffle (Fisher–Yates)
	for i := len(data) - 1; i > 0; i-- {
		j := r.Intn(i + 1)
		data[i], data[j] = data[j], data[i]
	}

	// Take first N characters
	return string(data[:length]), nil
}
