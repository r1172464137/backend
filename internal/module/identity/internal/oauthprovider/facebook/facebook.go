// Package facebook is the Graph API client of the Facebook sign-in method:
// the OAuth endpoints and the profile of the signed-in user.
package facebook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"golang.org/x/oauth2"
)

// Endpoint pins a current Graph API version; the constants shipped with
// golang.org/x/oauth2/facebook still point at the retired v3.2 dialog.
var Endpoint = oauth2.Endpoint{ //nolint:gosec // G101: OAuth endpoint URLs, not credentials
	AuthURL:  "https://www.facebook.com/v22.0/dialog/oauth",
	TokenURL: "https://graph.facebook.com/v22.0/oauth/access_token",
}

// userInfoURL is a variable so tests can point the client at a stub server.
var userInfoURL = "https://graph.facebook.com/v22.0/me"

type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type Client struct {
	*oauth2.Config
}

// UserInfo is the subset of the Graph API "me" node the login flow needs.
type UserInfo struct {
	OpenID  string
	Name    string
	Email   string
	Picture string
}

func New(config *Config) *Client {
	return &Client{
		&oauth2.Config{
			ClientID:     config.ClientID,
			ClientSecret: config.ClientSecret,
			RedirectURL:  config.RedirectURL,
			Scopes:       []string{"email", "public_profile"},
			Endpoint:     Endpoint,
		},
	}
}

// GetUserInfo fetches the user profile from the Graph API. Facebook only
// returns the email field when the account has a confirmed address and the
// user granted the email permission, so a non-empty value is verified. The
// request is bound to ctx, which carries the caller's deadline.
func (c *Client) GetUserInfo(ctx context.Context, token string) (*UserInfo, error) {
	query := url.Values{}
	query.Set("fields", "id,name,email,picture.type(large)")
	// appsecret_proof is mandatory when the app enables "Require App
	// Secret" and harmless otherwise.
	query.Set("appsecret_proof", c.appSecretProof(token))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Client(ctx, &oauth2.Token{AccessToken: token}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("facebook graph api request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("facebook graph api returned status %d", resp.StatusCode)
	}

	var raw struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Email   string `json:"email"`
		Picture struct {
			Data struct {
				URL string `json:"url"`
			} `json:"data"`
		} `json:"picture"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode facebook user: %w", err)
	}
	if raw.ID == "" {
		return nil, fmt.Errorf("facebook graph api returned no user id")
	}

	return &UserInfo{
		OpenID:  raw.ID,
		Name:    raw.Name,
		Email:   raw.Email,
		Picture: raw.Picture.Data.URL,
	}, nil
}

// appSecretProof signs the access token with the app secret as required by
// Graph API calls from servers.
// Ref: https://developers.facebook.com/docs/graph-api/securing-requests
func (c *Client) appSecretProof(token string) string {
	mac := hmac.New(sha256.New, []byte(c.ClientSecret))
	mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}
