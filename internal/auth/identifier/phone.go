package identifier

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/nyaruka/phonenumbers"
)

// ErrInvalidMobile reports a phone number that does not parse.
var ErrInvalidMobile = errors.New("invalid phone number")

func Check(areaCode, telephone string) bool {
	parsedNumber, err := phonenumbers.Parse(fmt.Sprintf("+%s%s", areaCode, telephone), areaCode)
	if err != nil {
		return false
	}
	return phonenumbers.IsValidNumber(parsedNumber)
}

func CheckPhone(telephone string) bool {
	parsedNumber, err := phonenumbers.Parse(fmt.Sprintf("+%s", telephone), "")
	if err != nil {
		return false
	}
	return phonenumbers.IsValidNumber(parsedNumber)
}

func GetCountryCode(telephone string) string {
	parsedNumber, err := phonenumbers.Parse(fmt.Sprintf("+%s", telephone), "")
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d", *parsedNumber.CountryCode)
}

func FormatToInternational(telephone string) string {
	parsedNumber, err := phonenumbers.Parse(fmt.Sprintf("+%s", telephone), "")
	if err != nil {
		return ""
	}
	return phonenumbers.Format(parsedNumber, phonenumbers.INTERNATIONAL)
}

// FormatToE164 returns the E.164 form of a phone number given as its country
// calling code and national number, the form every sign-in, verification
// code and binding stores and looks the number up in.
func FormatToE164(area, phone string) (string, error) {
	return toE164(fmt.Sprintf("+%s%s", area, phone))
}

// CanonicalMobile returns the E.164 form of a phone number written with its
// country calling code, with or without the leading "+" and with the
// separators people type: "86-13800138000" (the form the admin panel used to
// store), "+86 138 0013 8000" and "8613800138000" all give "+8613800138000".
// The number is parsed, not validated: an unassigned number still has one
// E.164 form, the one sign-in looks it up in.
func CanonicalMobile(number string) (string, error) {
	number = strings.TrimSpace(number)
	if number == "" {
		return "", ErrInvalidMobile
	}
	if !strings.HasPrefix(number, "+") {
		number = "+" + number
	}
	return toE164(number)
}

func toE164(number string) (string, error) {
	parsed, err := phonenumbers.Parse(number, "")
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidMobile, err)
	}
	return phonenumbers.Format(parsed, phonenumbers.E164), nil
}

// MaskPhoneNumber parses a phone number and masks the middle of its national
// part, keeping the country code, the first three and the last four digits.
func MaskPhoneNumber(phone string) string {
	num, err := phonenumbers.Parse(phone, "")
	if err != nil {
		return ""
	}
	// The international format, such as "+1 512-345-6789".
	formatted := phonenumbers.Format(num, phonenumbers.INTERNATIONAL)

	re := regexp.MustCompile(`(\+\d{1,3})\s*(.*)`)
	matches := re.FindStringSubmatch(formatted)
	if len(matches) < 3 {
		return formatted
	}
	countryCode := matches[1] // such as "+1"
	numberPart := matches[2]  // such as "512-345-6789"
	return fmt.Sprintf("%s %s", countryCode, maskDigits(numberPart))
}

// maskDigits replaces the digits between the first three and the last four
// with "*", keeping the separators.
func maskDigits(number string) string {
	digitCount := 0
	for _, r := range number {
		if r >= '0' && r <= '9' {
			digitCount++
		}
	}
	runes := []rune(number)
	digitIndex := 0
	for i, r := range runes {
		if r >= '0' && r <= '9' {
			digitIndex++
			if digitIndex > 3 && digitIndex <= digitCount-4 {
				runes[i] = '*'
			}
		}
	}
	return string(runes)
}
