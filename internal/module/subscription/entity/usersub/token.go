package usersub

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"unicode"
)

// MinTokenLength is the shortest token delivery resolves. Every token the
// server issued has 32 characters (NewToken and its predecessors); the bound
// is deliberately looser than that so it only turns away what can never be a
// token, the empty string above all: an empty token used to match the rows of
// older versions that stored none, and every probe cost a database query and
// a negative cache entry.
const MinTokenLength = 8

// AcceptableToken reports whether raw may be a stored token and is worth a
// lookup: at least MinTokenLength characters, none of them whitespace or a
// control character.
func AcceptableToken(raw string) bool {
	if len(raw) < MinTokenLength {
		return false
	}
	for _, r := range raw {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// NewToken returns a new subscription token: 128 bits from crypto/rand,
// hex-encoded to the 32 characters stored tokens already have. The token is
// the only credential of the subscription URL, so it must not be derivable
// from anything another party can see, such as an order number or the time
// it was issued.
func NewToken() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(fmt.Errorf("generate subscription token: %w", err))
	}
	return hex.EncodeToString(buf[:])
}
