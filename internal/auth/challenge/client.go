// Package challenge verifies Cloudflare Turnstile tokens, the human check
// guarding sign-in, registration, password resets and guest purchases.
package challenge

import (
	"context"
	"time"
)

// SiteVerifyURL is Cloudflare's Turnstile verification endpoint.
const SiteVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// Config configures a Turnstile client.
type Config struct {
	// Secret is the site's Turnstile secret key; required.
	Secret string
	// Timeout bounds one verification; 10 seconds when zero.
	Timeout time.Duration
	// URL is the verification endpoint; SiteVerifyURL when empty.
	URL string
}

// Service verifies Turnstile tokens.
type Service interface {
	// Verify reports whether token is a valid Turnstile response issued to
	// the client at ip. An error means the verification could not be done.
	Verify(ctx context.Context, token string, ip string) (bool, error)
}

// New returns a Turnstile client.
func New(config Config) Service {
	return newService(config)
}
