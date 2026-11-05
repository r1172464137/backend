package usersub

import (
	"regexp"
	"testing"
)

// The tokens the server issues are acceptable; the empty string, a short
// value and anything with whitespace or control characters are not.
func TestAcceptableTokenTurnsAwayWhatCannotBeAToken(t *testing.T) {
	if !AcceptableToken(NewToken()) {
		t.Fatal("an issued token is not acceptable")
	}
	for name, token := range map[string]string{
		"empty": "", "short": "abcdef1", "space": "0123456789abcdef 0123456789abcdef",
		"newline": "0123456789abcdef\n", "control": "0123456789abcdef\x00",
	} {
		if AcceptableToken(token) {
			t.Errorf("%s token %q is acceptable", name, token)
		}
	}
	if !AcceptableToken("abcdef12") {
		t.Fatalf("a token of MinTokenLength (%d) is refused", MinTokenLength)
	}
}

func TestNewTokenKeepsStoredFormatAndIsRandom(t *testing.T) {
	format := regexp.MustCompile(`^[0-9a-f]{32}$`)
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		token := NewToken()
		if !format.MatchString(token) {
			t.Fatalf("NewToken() = %q, want 32 lowercase hex characters", token)
		}
		if _, dup := seen[token]; dup {
			t.Fatalf("NewToken() repeated %q", token)
		}
		seen[token] = struct{}{}
	}
}
