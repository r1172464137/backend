// Package protocol implements EPay V1 and V2 using Go's standard library.
// NewClient selects V2 (RSA); NewV1Client selects V1 (MD5).
// Neither client automatically retries financial operations.
// Adapted from the local github.com/herta-network/epay SDK (2026-10-25).
// See docs/guide/epay.md for protocol coverage and backend integration boundaries.
package protocol

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 2 << 20

// Config configures the EPay V2 RSA client. Use V1Config for shared-key V1.
type Config struct {
	BaseURL            string // Required HTTPS origin of the target EPay deployment, optionally with a deployment path.
	MerchantID         string
	MerchantPrivateKey []byte // Unencrypted PEM or bare Base64 DER.
	PlatformPublicKey  []byte // PEM or bare Base64 DER; never the merchant public key.
	HTTPClient         *http.Client
	MaxClockSkew       time.Duration // Zero selects the PHP SDK's 300-second window.
}

// Client implements EPay V2. Use V1Client for the MD5 protocol.
type Client struct {
	baseURL    string
	merchantID string
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	http       *http.Client
	clockSkew  time.Duration
	now        func() time.Time
}

func NewClient(config Config) (*Client, error) {
	if config.MerchantID == "" || strings.Trim(config.MerchantID, "0123456789") != "" {
		return nil, errors.New("epay: MerchantID must contain decimal digits")
	}
	base := config.BaseURL
	if base == "" {
		return nil, errors.New("epay: BaseURL is required; configure your EPay provider's HTTPS origin")
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || !validBasePath(u) {
		return nil, errors.New("epay: BaseURL must be an HTTPS origin or deployment path without credentials, query or fragment")
	}
	privateKey, err := parsePrivateKey(config.MerchantPrivateKey)
	if err != nil {
		return nil, err
	}
	publicKey, err := parsePublicKey(config.PlatformPublicKey)
	if err != nil {
		return nil, err
	}
	skew := config.MaxClockSkew
	if skew < 0 {
		return nil, errors.New("epay: MaxClockSkew cannot be negative")
	}
	if skew == 0 {
		skew = 5 * time.Minute
	}
	httpClient := http.Client{Timeout: 15 * time.Second}
	if config.HTTPClient != nil {
		httpClient = *config.HTTPClient
		if httpClient.Timeout == 0 {
			httpClient.Timeout = 15 * time.Second
		}
	}
	// Never forward signed payment data through a redirect; do not mutate the caller's client.
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{
		baseURL: u.Scheme + "://" + u.Host + strings.TrimSuffix(u.EscapedPath(), "/"), merchantID: config.MerchantID,
		privateKey: privateKey, publicKey: publicKey, http: &httpClient,
		clockSkew: skew, now: time.Now,
	}, nil
}

// Request separates signed scalar fields from arrays, which the upstream
// protocol excludes from its signature. Arrays use PHP-compatible key[index] encoding.
type Request struct {
	Params Params
	Arrays map[string][]string
}

// SignedParams injects pid, timestamp and sign_type. Caller-owned maps are not changed.
func (c *Client) SignedParams(params Params) (Params, error) {
	result := make(Params, len(params)+4)
	for key, value := range params {
		switch key {
		case "pid", "timestamp", "sign", "sign_type":
			return nil, fmt.Errorf("epay: %s is managed by the client", key)
		}
		result[key] = value
	}
	result["pid"] = c.merchantID
	result["timestamp"] = strconv.FormatInt(c.now().Unix(), 10)
	result["sign_type"] = "RSA"
	signature, err := signParams(c.privateKey, result)
	if err != nil {
		return nil, err
	}
	result["sign"] = signature
	return result, nil
}

func (c *Client) form(request Request) (url.Values, error) {
	signed, err := c.SignedParams(request.Params)
	if err != nil {
		return nil, err
	}
	values := make(url.Values, len(signed)+len(request.Arrays))
	for key, value := range signed {
		values.Set(key, value)
	}
	for key, items := range request.Arrays {
		if !parameterName.MatchString(key) || key == "file" {
			return nil, fmt.Errorf("epay: invalid array name %q", key)
		}
		if _, exists := signed[key]; exists {
			return nil, fmt.Errorf("epay: scalar/array collision for %s", key)
		}
		for i, value := range items {
			values.Set(fmt.Sprintf("%s[%d]", key, i), value)
		}
	}
	return values, nil
}

func (c *Client) endpoint(apiPath string) (string, error) {
	if !strings.HasPrefix(apiPath, "/api/") || strings.ContainsAny(apiPath, "?#%\\") || path.Clean(apiPath) != apiPath {
		return "", errors.New("epay: expected an absolute /api/... path without query, fragment or traversal")
	}
	return c.baseURL + apiPath, nil
}

// Call sends a single form POST and verifies successful responses before exposing
// their contents. Use this for merchant, transfer and complaint API extensions too.
func (c *Client) Call(ctx context.Context, apiPath string, request Request) (*Response, error) {
	endpoint, err := c.endpoint(apiPath)
	if err != nil {
		return nil, err
	}
	form, err := c.form(request)
	if err != nil {
		return nil, err
	}
	return c.send(ctx, endpoint, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
}

type HTTPError struct{ StatusCode int }

func (e *HTTPError) Error() string { return fmt.Sprintf("epay: HTTP status %d", e.StatusCode) }

func (c *Client) send(ctx context.Context, endpoint, contentType string, body io.Reader) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	response, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("epay: request outcome may be unknown: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &HTTPError{StatusCode: response.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("epay: read response: %w", err)
	}
	if len(data) > maxResponseBytes {
		return nil, errors.New("epay: response exceeds 2 MiB")
	}
	return c.parseResponse(data)
}

func (c *Client) verify(params Params) error {
	if err := verifyParams(c.publicKey, params); err != nil {
		return err
	}
	text := params["timestamp"]
	if len(text) != 10 || strings.Trim(text, "0123456789") != "" {
		return errors.New("epay: timestamp must be a 10-digit Unix timestamp in seconds")
	}
	seconds, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return errors.New("epay: invalid timestamp")
	}
	// Unix seconds match PHP time(); subsecond differences must not reject the boundary.
	now := time.Unix(c.now().Unix(), 0)
	stamp := time.Unix(seconds, 0)
	if stamp.Before(now.Add(-c.clockSkew)) || stamp.After(now.Add(c.clockSkew)) {
		return errors.New("epay: timestamp outside permitted clock skew")
	}
	if pid, ok := params["pid"]; ok && pid != c.merchantID {
		return errors.New("epay: merchant ID mismatch")
	}
	return nil
}

func validBasePath(u *url.URL) bool {
	// RawPath is only an optional hint: canonical escapes such as %23 and
	// %3F can leave it empty even though Path contains decoded delimiters.
	// Check before trimming so a root path of // cannot collapse to / and
	// later produce a browser URL interpreted as a different authority.
	if u.RawPath != "" || u.EscapedPath() != u.Path || strings.Contains(u.Path, "//") {
		return false
	}
	p := strings.TrimSuffix(u.Path, "/")
	if p == "" {
		return true
	}
	return strings.HasPrefix(p, "/") && path.Clean(p) == p && !strings.ContainsAny(p, "%\\")
}
