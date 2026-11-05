package telegram

import (
	"crypto/hmac"
	"encoding/hex"
	"fmt"
)

type AuthData struct {
	Id        *int64  `json:"id,omitempty"`
	FirstName *string `json:"first_name,omitempty"`
	LastName  *string `json:"last_name,omitempty"`
	Username  *string `json:"username,omitempty"`
	PhotoUrl  *string `json:"photo_url,omitempty"`
	AuthDate  *int64  `json:"auth_date,omitempty"`
	Hash      *string `json:"hash,omitempty"`

	// raw keeps every field of the signed payload, including ones this
	// struct does not model, because Telegram's hash covers all of them.
	raw map[string]any
}

// Validate checks the hash of AuthData with computed one. To compute hash botToken is required.
// Ref: https://core.telegram.org/widgets/login#checking-authorization
func (d *AuthData) Validate(botToken []byte) error {
	if d.Hash == nil {
		return fmt.Errorf("auth data has no 'hash' value")
	}
	if len(botToken) == 0 {
		return fmt.Errorf("telegram bot token is not provided")
	}
	received, err := hex.DecodeString(*d.Hash)
	if err != nil {
		return fmt.Errorf("hash is not valid")
	}
	computed, err := hex.DecodeString(computeHash(d.raw, botToken))
	if err != nil {
		return fmt.Errorf("compute hash failed: %w", err)
	}
	// Constant-time comparison keeps the verification free of a timing
	// side-channel, matching how the other signed callbacks are checked.
	if !hmac.Equal(received, computed) {
		return fmt.Errorf("hash is not valid")
	}
	return nil
}
