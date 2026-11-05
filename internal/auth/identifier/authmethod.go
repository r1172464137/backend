// Package identifier normalizes and validates the identifiers accounts sign
// in with: email addresses, phone numbers and device identifiers. Every
// module stores and looks up identifiers in the canonical form this package
// produces, so one address cannot become two accounts by being spelled two
// ways.
package identifier

import (
	"strings"
)

const (
	Email  = "email"
	Mobile = "mobile"
	Device = "device"
)

func CanonicalEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// NormalizeIdentifier returns the form an identifier of authType is stored
// and looked up in: an email address lower-cased and trimmed, a phone number
// in E.164 (see CanonicalMobile). Other identifiers are opaque and returned
// as given. It fails for an empty email address or a phone number that does
// not parse.
func NormalizeIdentifier(authType, identifier string) (string, error) {
	switch authType {
	case Email:
		canonical := CanonicalEmail(identifier)
		if canonical == "" {
			return "", ErrInvalidEmail
		}
		return canonical, nil
	case Mobile:
		return CanonicalMobile(identifier)
	default:
		return identifier, nil
	}
}

// CanonicalIdentifier is NormalizeIdentifier for lookups: an identifier that
// cannot be normalized is returned as given and simply matches nothing.
func CanonicalIdentifier(authType, identifier string) string {
	if canonical, err := NormalizeIdentifier(authType, identifier); err == nil {
		return canonical
	}
	if authType == Email {
		return CanonicalEmail(identifier)
	}
	return identifier
}

// EmailMailboxKey maps an email address to the mailbox that receives it: the
// subaddress after "+" is dropped, and Gmail ignores dots in the local part
// and treats googlemail.com as gmail.com. Addresses sharing a key reach one
// inbox, so one person could register them all. The key only detects such
// aliases; it is never stored or used to sign in.
func EmailMailboxKey(email string) string {
	email = CanonicalEmail(email)
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return email
	}
	local, domain := email[:at], email[at+1:]
	if plus := strings.Index(local, "+"); plus > 0 {
		local = local[:plus]
	}
	if domain == "googlemail.com" {
		domain = "gmail.com"
	}
	if domain == "gmail.com" {
		local = strings.ReplaceAll(local, ".", "")
	}
	return local + "@" + domain
}
