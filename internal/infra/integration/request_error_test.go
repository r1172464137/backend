package integration

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestRequestErrorDropsTheURLAndKeepsTheCause(t *testing.T) {
	cause := errors.New("dial tcp 127.0.0.1:2525: connection refused")
	err := RequestError("smsbao", &url.Error{Op: "Get", URL: "https://api.example.test/sms?p=secret&m=13800000000", Err: cause})

	if !errors.Is(err, cause) {
		t.Fatalf("error %v does not wrap the cause", err)
	}
	if s := err.Error(); strings.Contains(s, "secret") || strings.Contains(s, "13800000000") || !strings.HasPrefix(s, "smsbao request: ") {
		t.Fatalf("error = %q, want the provider prefix and no URL", s)
	}
}

// Failures that are not round trips keep their message.
func TestRequestErrorKeepsOtherErrors(t *testing.T) {
	cause := errors.New("status code: 502")
	err := RequestError("abosend", cause)
	if !errors.Is(err, cause) || err.Error() != "abosend request: status code: 502" {
		t.Fatalf("error = %v", err)
	}
}
