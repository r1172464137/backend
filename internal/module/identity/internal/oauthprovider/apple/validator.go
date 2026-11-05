package apple

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// ValidationURL is the endpoint for verifying tokens
	ValidationURL string = "https://appleid.apple.com/auth/token"
	// ContentType is the one expected by Apple
	ContentType string = "application/x-www-form-urlencoded"
	// UserAgent is required by Apple or the request will fail
	UserAgent string = "go-signin-with-apple"
	// AcceptHeader is the content that we are willing to accept
	AcceptHeader string = "application/json"
)

// Client validates Sign in with Apple authorization codes.
type Client struct {
	config        Config
	validationURL string
	secret        string
	client        *http.Client
}

// VerifyWebToken exchanges the authorization code of a web sign-in for its
// tokens. Apple answers a rejected code with an error body, which is
// returned in the response's Error field.
func (c *Client) VerifyWebToken(ctx context.Context, code string) (ValidationResponse, error) {
	data := url.Values{
		"client_id":     {c.config.ClientID},
		"client_secret": {c.secret},
		"code":          {code},
		"redirect_uri":  {c.config.RedirectURI},
		"grant_type":    {"authorization_code"},
	}
	var resp ValidationResponse
	err := doRequest(ctx, c.client, &resp, c.validationURL, data)
	return resp, err
}

// GetUniqueID decodes the id_token response and returns the unique subject ID to identify the user
func GetUniqueID(idToken string) (string, error) {
	token, _, err := new(jwt.Parser).ParseUnverified(idToken, jwt.MapClaims{})
	if err != nil {
		return "", err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid token claims")
	}

	return fmt.Sprintf("%v", claims["sub"]), nil
}

// GetClaims decodes the id_token response and returns the JWT claims to identify the user
func GetClaims(idToken string) (*jwt.MapClaims, error) {
	token, _, err := new(jwt.Parser).ParseUnverified(idToken, jwt.MapClaims{})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	return &claims, nil
}

// doRequest posts data to Apple and decodes the answer into result. Apple
// reports a rejected request with an error body, so a 4xx answer that
// decodes is returned for the caller to inspect; any other failure is an
// error.
func doRequest(ctx context.Context, client *http.Client, result any, url string, data url.Values) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Add("content-type", ContentType)
	req.Header.Add("accept", AcceptHeader)
	req.Header.Add("user-agent", UserAgent) // apple requires a user agent

	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("apple returned status %d", res.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(result); err != nil {
		return fmt.Errorf("decode apple response (status %d): %w", res.StatusCode, err)
	}
	return nil
}
