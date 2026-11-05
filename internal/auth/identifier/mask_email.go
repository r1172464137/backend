package identifier

import (
	"strings"
)

// MaskEmail hides the local part of an email address for display, keeping
// only its first and last characters: alice@example.com becomes
// a***e@example.com. A local part of one or two characters keeps at most its
// first, so a short address is not given away whole, and a string that is not
// an address becomes "***".
func MaskEmail(email string) string {
	atIndex := strings.Index(email, "@")
	if atIndex == -1 || atIndex == 0 || atIndex == len(email)-1 {
		return "***"
	}
	localPart := email[:atIndex]
	domainPart := email[atIndex+1:]
	localRunes := []rune(localPart)

	if len(localRunes) == 1 {
		return "*@" + domainPart
	}
	if len(localRunes) == 2 {
		return string(localRunes[0]) + "*@" + domainPart
	}
	maskedLocal := string(localRunes[0]) + strings.Repeat("*", len(localRunes)-2) + string(localRunes[len(localRunes)-1])
	return maskedLocal + "@" + domainPart
}
