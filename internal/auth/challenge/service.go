package challenge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

type service struct {
	timeout time.Duration
	secret  string
	url     string
	client  *http.Client
}

func newService(config Config) Service {
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	if config.URL == "" {
		config.URL = SiteVerifyURL
	}
	return &service{
		secret:  config.Secret,
		timeout: config.Timeout,
		url:     config.URL,
		client:  &http.Client{Timeout: config.Timeout},
	}
}

func (s *service) Verify(ctx context.Context, token string, ip string) (bool, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for _, field := range [][2]string{{"secret", s.secret}, {"response", token}, {"remoteip", ip}} {
		if err := writer.WriteField(field[0], field[1]); err != nil {
			return false, err
		}
	}
	if err := writer.Close(); err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(timeoutCtx, http.MethodPost, s.url, body)
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := s.client.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return false, fmt.Errorf("turnstile returned HTTP %d", resp.StatusCode)
	}
	var outcome struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&outcome); err != nil {
		return false, err
	}
	return outcome.Success, nil
}
