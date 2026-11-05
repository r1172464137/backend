// Package apple is the Sign in with Apple client of the Apple sign-in
// method: it signs the client secret from the configured private key and
// exchanges authorization codes for the identity token.
package apple

import (
	"net/http"
	"time"
)

type Config struct {
	TeamID       string
	ClientID     string
	KeyID        string
	ClientSecret string
	RedirectURI  string
}

// New creates a Client for the Apple validation endpoint. It signs the
// client secret from the configured private key, so it fails for a key that
// does not parse.
func New(c Config) (*Client, error) {
	secret, err := GenerateClientSecret(c.ClientSecret, c.TeamID, c.ClientID, c.KeyID)
	if err != nil {
		return nil, err
	}
	return &Client{
		config:        c,
		validationURL: ValidationURL,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		secret: secret,
	}, nil
}
