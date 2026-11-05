// Package random makes the random and derived strings the application hands
// out: verification codes, tokens and secrets from crypto/rand, and the
// invite-code encoding of user ids.
package random

import (
	cryptorand "crypto/rand"
	"math/big"
	"slices"
	"strings"
)

const (
	// chars62 is the digit alphabet of EncodeBase62, shuffled so that
	// consecutive ids do not read as consecutive codes.
	chars62 = "E7gLp4jWS6kPv5DzxaY1o9sNcFmBAlUut0ZOhKVM38bqHRJfCwdrTni2QIeXGy"
	base62  = int64(len(chars62))
	// codeLength is the length shorter codes are padded to.
	codeLength = 6
	// padStride separates the padding characters, taken from the end of
	// the alphabet backwards.
	padStride = 3
)

// EncodeBase62 is the invite-code encoding: id in base 62 over the shuffled
// chars62 alphabet, most significant digit first, left-padded to six
// characters. The padding is chars62's last character next to the digits,
// then every third character before it. The padding makes the encoding
// irreversible and not injective (916132831 and 56800235583 both give
// "yyyyyy"); zero encodes as the single character "E" and a negative id as
// the padding alone. Invite codes are stored and shared, so the output must
// never change; TestEncodeBase62Golden pins it.
func EncodeBase62(id int64) string {
	if id == 0 {
		return string(chars62[0])
	}
	// Built least significant digit first, then reversed.
	code := make([]byte, 0, 11)
	for ; id > 0; id /= base62 {
		code = append(code, chars62[id%base62])
	}
	for pad := 0; len(code) < codeLength; pad++ {
		code = append(code, chars62[len(chars62)-1-padStride*pad])
	}
	slices.Reverse(code)
	return string(code)
}

// Key returns length random characters: letters and digits for keyType 1,
// digits otherwise, as in a verification code.
func Key(length int, keyType int) string {
	randomString := "0123456789"
	if keyType == 1 {
		randomString = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdefghijklmnopqrstuvwxyz"
	}
	return secureKey(length, randomString)
}

// KeyNew is Key with one more keyType: 2 draws from upper-case letters and
// digits.
func KeyNew(length int, keyType int) string {
	randomString := "0123456789"
	switch keyType {
	case 1:
		randomString = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdefghijklmnopqrstuvwxyz"
	case 2:
		randomString = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	}
	return secureKey(length, randomString)
}

func secureKey(length int, alphabet string) string {
	if length <= 0 || len(alphabet) == 0 {
		return ""
	}
	result := make([]byte, length)
	upper := big.NewInt(int64(len(alphabet)))
	for i := range result {
		index, err := cryptorand.Int(cryptorand.Reader, upper)
		if err != nil {
			// A functioning OS CSPRNG is a security prerequisite. Never fall back
			// to a predictable generator for verification codes or OAuth state.
			panic("crypto/rand unavailable: " + err.Error())
		}
		result[i] = alphabet[index.Int64()]
	}
	return string(result)
}

// StrToDashedString inserts a dash after every fourth character, to make a
// long code readable ("ABCD-EFGH-IJ").
func StrToDashedString(strNum string) string {
	var result strings.Builder

	for i, ch := range strNum {
		result.WriteRune(ch)
		if (i+1)%4 == 0 && i != len(strNum)-1 {
			result.WriteRune('-')
		}
	}

	return result.String()
}
