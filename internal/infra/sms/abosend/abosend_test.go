package abosend

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// abosendAPI records the one request it receives and answers with status
// and body.
func abosendAPI(t *testing.T, status int, body string) (*httptest.Server, *http.Request, *request) {
	t.Helper()
	var received http.Request
	var payload request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = *r
		data, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Errorf("request body %q is not JSON: %v", data, err)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server, &received, &payload
}

func newTestClient(server *httptest.Server) *Client {
	return NewClient(Config{ApiDomain: server.URL + "/", Access: "ORG01", Secret: "md5-key"}, server.Client())
}

func TestSendTextBuildsSignedRequest(t *testing.T) {
	server, received, payload := abosendAPI(t, http.StatusOK, `{"code":200,"message":"ok","data":{"sendCode":"S1"}}`)

	if err := newTestClient(server).SendText(context.Background(), "852", "91234567", "Your code is 123456"); err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if received.Method != http.MethodPost || received.URL.Path != "/v2/api/sendSMS" || received.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("request = %s %s (%s)", received.Method, received.URL.Path, received.Header.Get("Content-Type"))
	}
	if payload.OrgCode != "ORG01" || payload.MobileArea != "+852" || payload.Mobile != "85291234567" || payload.Content != "Your code is 123456" {
		t.Fatalf("payload = %+v", payload)
	}
	if !regexp.MustCompile(`^[0-9]{6}$`).MatchString(payload.Rand) {
		t.Fatalf("nonce = %q, want six digits", payload.Rand)
	}
	sum := md5.Sum([]byte("ORG01" + "Your code is 123456" + payload.Rand + "md5-key"))
	if want := strings.ToUpper(hex.EncodeToString(sum[:])); payload.Sign != want {
		t.Fatalf("sign = %q, want %q", payload.Sign, want)
	}
}

func TestSendTextReportsProviderFailures(t *testing.T) {
	for name, tt := range map[string]struct {
		status int
		body   string
		want   string
	}{
		"rejected":     {http.StatusOK, `{"code":4001,"message":"insufficient balance"}`, "insufficient balance"},
		"server error": {http.StatusBadGateway, `bad gateway`, "status code: 502"},
		"garbage":      {http.StatusOK, `<html>`, "unmarshal response"},
	} {
		t.Run(name, func(t *testing.T) {
			server, _, _ := abosendAPI(t, tt.status, tt.body)
			err := newTestClient(server).SendText(context.Background(), "86", "13800000000", "hi")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

// A transport failure is logged by the sending task, so its error must not
// carry the organisation code, the key, the number or the code.
func TestSendTextTransportFailureKeepsSecretsOutOfTheError(t *testing.T) {
	client := NewClient(Config{ApiDomain: "http://" + closedAddr(t), Access: "ORG01", Secret: "md5-key"}, http.DefaultClient)

	err := client.SendText(context.Background(), "86", "13800000000", "Your code is 123456")

	if err == nil {
		t.Fatal("a refused connection reported success")
	}
	for _, secret := range []string{"ORG01", "md5-key", "13800000000", "123456", "?"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error %q carries %q", err, secret)
		}
	}
}

// closedAddr returns an address nothing listens on, so connecting to it is
// refused.
func closedAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	return addr
}

func TestSendTextHonoursContext(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := newTestClient(server).SendText(ctx, "86", "13800000000", "hi"); err == nil {
		t.Fatal("a cancelled send reported success")
	}
	if calls != 0 {
		t.Fatalf("calls = %d, want none after cancellation", calls)
	}
}

func TestNewClientDefaultsToTheProviderDomain(t *testing.T) {
	if got := NewClient(Config{}, http.DefaultClient).baseURL; got != BaseURL {
		t.Fatalf("base URL = %q, want %q", got, BaseURL)
	}
}
