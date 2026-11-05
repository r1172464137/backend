package token

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "token-test-secret"

// A token this package issued parses back with its claims.
func TestIssuedTokensParse(t *testing.T) {
	signed, err := NewJwtToken(testSecret, time.Now().Unix(), 60, WithOption("UserId", int64(7)))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseJwtToken(signed, testSecret)
	if err != nil {
		t.Fatalf("ParseJwtToken() error = %v", err)
	}
	if id, _ := claims["UserId"].(float64); id != 7 {
		t.Fatalf("UserId claim = %v, want 7", claims["UserId"])
	}
	if _, ok := claims["exp"]; !ok {
		t.Fatal("an issued token carries no exp")
	}
}

// Only the algorithm the package signs with is accepted, an expiry is
// required and the signature must verify: a token forged with alg none,
// signed with another algorithm, left without an expiry or signed with
// another secret is refused, as is an expired one.
func TestParseJwtTokenRefusesForgedTokens(t *testing.T) {
	now := time.Now().Unix()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	sign := func(method jwt.SigningMethod, key any, claims jwt.MapClaims) string {
		token := jwt.New(method)
		token.Claims = claims
		signed, err := token.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return signed
	}
	live := jwt.MapClaims{"exp": now + 60, "iat": now, "UserId": int64(7)}

	wrongSecret, err := NewJwtToken("another-secret", now, 60)
	if err != nil {
		t.Fatal(err)
	}
	expired, err := NewJwtToken(testSecret, now-120, 60)
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		token string
		want  error
	}{
		"alg none":       {sign(jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, live), jwt.ErrTokenSignatureInvalid},
		"RS256":          {sign(jwt.SigningMethodRS256, rsaKey, live), jwt.ErrTokenSignatureInvalid},
		"HS512":          {sign(jwt.SigningMethodHS512, []byte(testSecret), live), jwt.ErrTokenSignatureInvalid},
		"missing exp":    {sign(jwt.SigningMethodHS256, []byte(testSecret), jwt.MapClaims{"iat": now, "UserId": int64(7)}), jwt.ErrTokenRequiredClaimMissing},
		"wrong secret":   {wrongSecret, jwt.ErrTokenSignatureInvalid},
		"expired":        {expired, jwt.ErrTokenExpired},
		"garbage":        {"not-a-token", jwt.ErrTokenMalformed},
		"tampered claim": {tamper(t, sign(jwt.SigningMethodHS256, []byte(testSecret), live)), jwt.ErrTokenSignatureInvalid},
	} {
		t.Run(name, func(t *testing.T) {
			claims, err := ParseJwtToken(tc.token, testSecret)
			if err == nil {
				t.Fatalf("ParseJwtToken() = %v, want an error", claims)
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("ParseJwtToken() error = %v, want %v", err, tc.want)
			}
		})
	}
}

// tamper changes a signed claim without re-signing it. Changing only the final
// base64 character can change unused padding bits while leaving the signature's
// decoded bytes intact, so it does not reliably produce a forged token.
func tamper(t *testing.T, signed string) string {
	t.Helper()
	parts := strings.Split(signed, ".")
	if len(parts) != 3 {
		t.Fatal("expected a signed JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims jwt.MapClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	claims["UserId"] = int64(8)
	payload, err = json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	parts[1] = base64.RawURLEncoding.EncodeToString(payload)
	return strings.Join(parts, ".")
}
