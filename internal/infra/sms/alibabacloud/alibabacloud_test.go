package alibabacloud

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
)

const (
	testAccessKey = "LTAI-test"
	testSecret    = "test-secret"
)

type aliyunRequest struct {
	method, path, action, version string
	query                         url.Values
	signatureOK                   bool
	signatureProblem              string
}

func aliyunAPI(t *testing.T, status int, body string) (*Client, *aliyunRequest) {
	t.Helper()
	received := &aliyunRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		received.method, received.path = r.Method, r.URL.Path
		received.action, received.version = r.Header.Get("x-acs-action"), r.Header.Get("x-acs-version")
		received.query = r.URL.Query()
		received.signatureProblem = verifyACS3(r, payload, testSecret)
		received.signatureOK = received.signatureProblem == ""
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	endpoint := strings.TrimPrefix(server.URL, "http://")
	client, err := newClient(Config{Access: testAccessKey, Secret: testSecret, SignName: "PPanel", TemplateCode: "SMS_1000", Endpoint: endpoint}, "http")
	if err != nil {
		t.Fatal(err)
	}
	return client, received
}

// verifyACS3 checks the request's ACS3-HMAC-SHA256 signature the way the
// Alibaba Cloud gateway does, independently of the SDK, and returns what is
// wrong with it, if anything.
func verifyACS3(r *http.Request, payload []byte, secret string) string {
	const prefix = "ACS3-HMAC-SHA256 "
	authorization := r.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, prefix) {
		return "no ACS3 authorization: " + authorization
	}
	fields := map[string]string{}
	for _, part := range strings.Split(strings.TrimPrefix(authorization, prefix), ",") {
		key, value, _ := strings.Cut(part, "=")
		fields[key] = value
	}
	if fields["Credential"] != testAccessKey {
		return "credential " + fields["Credential"]
	}
	payloadHash := sha256.Sum256(payload)
	if got := r.Header.Get("x-acs-content-sha256"); got != hex.EncodeToString(payloadHash[:]) {
		return "payload hash " + got
	}

	query := r.URL.Query()
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var canonicalQuery []string
	for _, key := range keys {
		escaped := url.QueryEscape(query.Get(key))
		escaped = strings.NewReplacer("+", "%20", "*", "%2A", "%7E", "~").Replace(escaped)
		canonicalQuery = append(canonicalQuery, key+"="+escaped)
	}

	signedHeaders := strings.Split(fields["SignedHeaders"], ";")
	var canonicalHeaders strings.Builder
	for _, name := range signedHeaders {
		value := r.Header.Get(name)
		if name == "host" {
			value = r.Host
		}
		canonicalHeaders.WriteString(name + ":" + strings.TrimSpace(value) + "\n")
	}
	for _, required := range []string{"host", "x-acs-action", "x-acs-date", "x-acs-signature-nonce", "x-acs-content-sha256"} {
		if !strings.Contains(fields["SignedHeaders"], required) {
			return "unsigned header " + required
		}
	}

	canonicalRequest := strings.Join([]string{
		r.Method, "/", strings.Join(canonicalQuery, "&"), canonicalHeaders.String(),
		fields["SignedHeaders"], hex.EncodeToString(payloadHash[:]),
	}, "\n")
	requestHash := sha256.Sum256([]byte(canonicalRequest))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("ACS3-HMAC-SHA256\n" + hex.EncodeToString(requestHash[:])))
	if want := hex.EncodeToString(mac.Sum(nil)); fields["Signature"] != want {
		return "signature " + fields["Signature"] + ", want " + want
	}
	return ""
}

func TestSendTemplateSendsASignedSendSmsCall(t *testing.T) {
	client, received := aliyunAPI(t, http.StatusOK, `{"Code":"OK","Message":"OK","RequestId":"R1","BizId":"B1"}`)

	if err := client.SendTemplate(context.Background(), "86", "13800000000", map[string]string{"code": "123456"}); err != nil {
		t.Fatalf("SendTemplate: %v", err)
	}
	if received.method != http.MethodPost || received.path != "/" || received.action != "SendSms" || received.version != "2017-05-25" {
		t.Fatalf("request = %s %s action=%s version=%s", received.method, received.path, received.action, received.version)
	}
	for key, want := range map[string]string{
		"PhoneNumbers":  "8613800000000",
		"SignName":      "PPanel",
		"TemplateCode":  "SMS_1000",
		"TemplateParam": `{"code":"123456"}`,
	} {
		if got := received.query.Get(key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
	if !received.signatureOK {
		t.Fatalf("signature rejected: %s", received.signatureProblem)
	}
}

func TestSendTemplateReportsProviderFailures(t *testing.T) {
	for name, tt := range map[string]struct {
		status int
		body   string
		want   string
	}{
		"business error": {http.StatusOK, `{"Code":"isv.MOBILE_NUMBER_ILLEGAL","Message":"invalid number","RequestId":"R2"}`, "isv.MOBILE_NUMBER_ILLEGAL"},
		"gateway error":  {http.StatusBadRequest, `{"Code":"SignatureDoesNotMatch","Message":"bad signature","RequestId":"R3"}`, "SignatureDoesNotMatch"},
	} {
		t.Run(name, func(t *testing.T) {
			client, _ := aliyunAPI(t, tt.status, tt.body)
			err := client.SendTemplate(context.Background(), "86", "13800000000", map[string]string{"code": "1"})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

// A transport failure is logged by the sending task, so its error must not
// carry the request URL: the SDK sends the number and the template
// parameters, the code among them, in the query string.
func TestSendTemplateTransportFailureKeepsSecretsOutOfTheError(t *testing.T) {
	client, err := newClient(Config{Access: testAccessKey, Secret: testSecret, SignName: "PPanel", TemplateCode: "SMS_1000", Endpoint: closedAddr(t)}, "http")
	if err != nil {
		t.Fatal(err)
	}

	err = client.SendTemplate(context.Background(), "86", "13800000000", map[string]string{"code": "123456"})

	if err == nil {
		t.Fatal("a refused connection reported success")
	}
	for _, secret := range []string{testAccessKey, testSecret, "13800000000", "123456", "TemplateParam", "?"} {
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

func TestSendTemplateHonoursContext(t *testing.T) {
	client, received := aliyunAPI(t, http.StatusOK, `{"Code":"OK"}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.SendTemplate(ctx, "86", "13800000000", map[string]string{"code": "1"}); err == nil {
		t.Fatal("a cancelled send reported success")
	}
	if received.path != "" {
		t.Fatal("a cancelled send reached the provider")
	}
}

func TestNewClientDefaultsTheEndpoint(t *testing.T) {
	client, err := NewClient(Config{Access: testAccessKey, Secret: testSecret})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := *client.client.Endpoint; got != defaultEndpoint {
		t.Fatalf("endpoint = %q, want %q", got, defaultEndpoint)
	}
}
