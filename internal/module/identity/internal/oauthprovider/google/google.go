// Package google is the Google client of the Google sign-in method: the
// OAuth configuration and the profile of the signed-in user.
package google

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// userInfoURL is a variable so tests can point the client at a stub server.
var userInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"

type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}
type Client struct {
	*oauth2.Config
}
type UserInfo struct {
	OpenID        string `json:"id"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
	VerifiedEmail bool   `json:"verified_email"`
}

func New(config *Config) *Client {
	return &Client{
		&oauth2.Config{
			ClientID:     config.ClientID,
			ClientSecret: config.ClientSecret,
			RedirectURL:  config.RedirectURL,
			Scopes:       []string{"openid", "profile", "email"},
			Endpoint:     google.Endpoint,
		},
	}
}

// GetUserInfo fetches the profile of the access token's user. The request
// is bound to ctx, which carries the caller's deadline.
func (c *Client) GetUserInfo(ctx context.Context, token string) (*UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Client(ctx, &oauth2.Token{AccessToken: token}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("google userinfo request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google userinfo returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read google userinfo: %w", err)
	}
	var raw struct {
		ID            string `json:"id"`
		Email         string `json:"email"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
		VerifiedEmail any    `json:"verified_email"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode google userinfo: %w", err)
	}
	if raw.ID == "" {
		return nil, fmt.Errorf("google userinfo returned no user id")
	}

	verified := false
	switch v := raw.VerifiedEmail.(type) {
	case bool:
		verified = v
	case string:
		verified = v == "true"
	}

	return &UserInfo{
		OpenID:        raw.ID,
		Email:         raw.Email,
		Name:          raw.Name,
		Picture:       raw.Picture,
		VerifiedEmail: verified,
	}, nil
}
