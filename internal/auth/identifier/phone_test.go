package identifier

import (
	"errors"
	"testing"
)

func TestCheck(t *testing.T) {
	if !Check("86", "15502505555") {
		t.Fatal("a valid Chinese mobile number was rejected")
	}
	if Check("86", "123") {
		t.Fatal("a too short number was accepted")
	}
}

func TestGetCountryCode(t *testing.T) {
	if got := GetCountryCode("14407941888"); got != "1" {
		t.Fatalf("GetCountryCode = %q, want 1", got)
	}
}

func TestFormatToInternational(t *testing.T) {
	if got := FormatToInternational("8615502505555"); got != "+86 155 0250 5555" {
		t.Fatalf("FormatToInternational = %q", got)
	}
}

func TestFormatToE164(t *testing.T) {
	e164, err := FormatToE164("1", "4407941888")
	if err != nil || e164 != "+14407941888" {
		t.Fatalf("FormatToE164 = %q, %v; want +14407941888", e164, err)
	}
	if _, err := FormatToE164("86", "abc"); !errors.Is(err, ErrInvalidMobile) {
		t.Fatalf("FormatToE164 accepted a number that does not parse: %v", err)
	}
}

// Every way the panels wrote a number normalizes to the E.164 form sign-in
// looks it up in; the admin panel used to store "<area>-<number>".
func TestCanonicalMobileAcceptsEveryStoredForm(t *testing.T) {
	for _, number := range []string{"+8613800138000", "8613800138000", "86-13800138000", "+86 138 0013 8000", " +86-138-0013-8000 "} {
		got, err := CanonicalMobile(number)
		if err != nil || got != "+8613800138000" {
			t.Errorf("CanonicalMobile(%q) = %q, %v; want +8613800138000", number, got, err)
		}
	}
	for _, number := range []string{"", "   ", "abc", "+"} {
		if got, err := CanonicalMobile(number); !errors.Is(err, ErrInvalidMobile) {
			t.Errorf("CanonicalMobile(%q) = %q, %v; want ErrInvalidMobile", number, got, err)
		}
	}
}

// A number registered through the area-code form is found through the full
// form and the other way round.
func TestFormatToE164MatchesCanonicalMobile(t *testing.T) {
	fromParts, err := FormatToE164("86", "13800138000")
	if err != nil {
		t.Fatal(err)
	}
	fromLegacy, err := CanonicalMobile("86-13800138000")
	if err != nil || fromLegacy != fromParts {
		t.Fatalf("CanonicalMobile = %q, %v; FormatToE164 = %q", fromLegacy, err, fromParts)
	}
}

func TestMaskPhoneNumberKeepsCountryCodeAndEnds(t *testing.T) {
	for number, want := range map[string]string{
		"+8613800138000": "+86 138 **** 8000",
		"+14407941888":   "+1 440-***-1888",
		"not a number":   "",
	} {
		if got := MaskPhoneNumber(number); got != want {
			t.Errorf("MaskPhoneNumber(%q) = %q, want %q", number, got, want)
		}
	}
}
