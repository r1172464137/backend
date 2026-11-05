package cryptomus

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func md5Hex(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

func signBody(body []byte, apiKey string) string {
	return md5Hex(base64.StdEncoding.EncodeToString(body) + apiKey)
}

// signedNotification builds a webhook payload the way the gateway does: the
// signature covers the JSON without the sign member, which is then appended
// as the last top-level field.
func signedNotification(t *testing.T, apiKey string, fields map[string]any) []byte {
	t.Helper()
	unsigned, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal notification: %v", err)
	}
	sign := signBody(unsigned, apiKey)
	signed := strings.TrimSuffix(string(unsigned), "}") + `,"sign":"` + sign + `"}`
	return []byte(signed)
}

func TestSignPayloadMatchesDocumentedAlgorithm(t *testing.T) {
	client := NewClient(Config{MerchantID: "merchant-1", APIKey: "api-key"})
	body := []byte(`{"amount":"10.00","currency":"USD","order_id":"order-1"}`)
	if got, want := client.signPayload(body), signBody(body, "api-key"); got != want {
		t.Fatalf("signPayload=%s, want %s", got, want)
	}
}

func TestVerifyNotificationSign(t *testing.T) {
	client := NewClient(Config{MerchantID: "merchant-1", APIKey: "api-key"})
	body := signedNotification(t, "api-key", map[string]any{
		"type": "payment", "uuid": "uuid-1", "order_id": "order-1",
		"amount": "10.00", "currency": "USD", "status": "paid", "is_final": true,
	})
	if !client.VerifyNotificationSign(body) {
		t.Fatal("valid signature must be accepted")
	}

	tampered := []byte(strings.Replace(string(body), `"10.00"`, `"1.00"`, 1))
	if client.VerifyNotificationSign(tampered) {
		t.Fatal("changing a signed field must invalidate the signature")
	}

	wrongKey := NewClient(Config{MerchantID: "merchant-1", APIKey: "other-key"})
	if wrongKey.VerifyNotificationSign(body) {
		t.Fatal("signature must be bound to the API key")
	}

	if client.VerifyNotificationSign([]byte(`{"order_id":"order-1"}`)) {
		t.Fatal("missing signature must be rejected")
	}

	duplicated := []byte(strings.TrimSuffix(string(body), "}") + `,"sign":"` + md5Hex("second") + `"}`)
	if client.VerifyNotificationSign(duplicated) {
		t.Fatal("payload with two sign members must be rejected")
	}
}

// TestVerifyNotificationSignPreservesPHPEscaping guards against the classic
// re-serialization pitfall: Cryptomus produces webhook bodies with PHP's
// json_encode, which escapes "/" as "\/" and leaves unicode unescaped, while
// most other JSON encoders do neither. Implementations that decode the body
// and re-encode it before hashing must patch the escaping back by hand; this
// verifier hashes the original bytes with only the sign member removed, so
// the sender's escaping never enters the picture. The fixture below is the
// exact byte shape PHP delivers.
func TestVerifyNotificationSignPreservesPHPEscaping(t *testing.T) {
	const apiKey = "api-key"
	unsigned := `{"type":"payment","uuid":"uuid-1","order_id":"order-1","amount":"10.00",` +
		`"currency":"USD","additional_data":"https:\/\/merchant.example\/return?plan=年付","status":"paid","is_final":true}`
	sign := signBody([]byte(unsigned), apiKey)
	body := []byte(strings.TrimSuffix(unsigned, "}") + `,"sign":"` + sign + `"}`)

	client := NewClient(Config{MerchantID: "merchant-1", APIKey: apiKey})
	if !client.VerifyNotificationSign(body) {
		t.Fatal("a PHP-escaped payload signed over its own bytes must verify")
	}

	// The same payload naively decoded and re-encoded by encoding/json loses
	// the "\/" escaping, which is exactly the mismatch the raw-byte approach
	// avoids; the fixture must actually exercise that difference.
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode PHP-escaped payload: %v", err)
	}
	delete(decoded, "sign")
	reencoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("re-encode payload: %v", err)
	}
	if strings.Contains(string(reencoded), `\/`) {
		t.Fatal("fixture no longer demonstrates the escaping mismatch")
	}
	if signBody(reencoded, apiKey) == sign {
		t.Fatal("re-encoded bytes must not accidentally reproduce the signature")
	}

	notification, err := ParseNotification(body)
	if err != nil {
		t.Fatalf("ParseNotification: %v", err)
	}
	if notification.OrderNo != "order-1" || !PaidStatus(notification.Status) {
		t.Fatalf("unexpected notification: %+v", notification)
	}
}

func TestCreateInvoiceSendsSignedRequest(t *testing.T) {
	const apiKey = "api-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != createInvoicePath {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("merchant") != "merchant-1" {
			t.Errorf("merchant header = %q", r.Header.Get("merchant"))
		}
		body := make([]byte, r.ContentLength)
		if _, err := r.Body.Read(body); err != nil && err.Error() != "EOF" {
			t.Fatalf("read request: %v", err)
		}
		if r.Header.Get("sign") != signBody(body, apiKey) {
			t.Errorf("request signature mismatch")
		}
		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request["amount"] != "10.50" || request["currency"] != "USD" || request["order_id"] != "order-1" {
			t.Errorf("unexpected request fields: %v", request)
		}
		if request["url_callback"] != "https://merchant.example/v1/notify/Cryptomus/token" {
			t.Errorf("unexpected callback: %v", request["url_callback"])
		}
		_, _ = fmt.Fprint(w, `{"state":0,"result":{"uuid":"uuid-1","order_id":"order-1","amount":"10.50","currency":"USD","url":"https://pay.cryptomus.com/pay/uuid-1","status":"check"}}`)
	}))
	defer server.Close()

	client := NewClient(Config{MerchantID: "merchant-1", APIKey: apiKey, BaseURL: server.URL})
	invoice, err := client.CreateInvoice(context.Background(), Order{
		OrderNo:   "order-1",
		Amount:    1050,
		Currency:  "usd",
		NotifyURL: "https://merchant.example/v1/notify/Cryptomus/token",
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if invoice.UUID != "uuid-1" || invoice.URL != "https://pay.cryptomus.com/pay/uuid-1" {
		t.Fatalf("unexpected invoice: %+v", invoice)
	}
}

func TestCreateInvoiceRejectsInvalidOrder(t *testing.T) {
	client := NewClient(Config{MerchantID: "merchant-1", APIKey: "api-key"})
	if _, err := client.CreateInvoice(context.Background(), Order{OrderNo: "", Amount: 100, Currency: "USD"}); err == nil {
		t.Fatal("empty order number must be rejected")
	}
	if _, err := client.CreateInvoice(context.Background(), Order{OrderNo: "order-1", Amount: 0, Currency: "USD"}); err == nil {
		t.Fatal("zero amount must be rejected")
	}
}

func TestGetInvoiceLooksUpByUUIDOrOrderNo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != invoiceInfoPath {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var request map[string]string
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request["uuid"] == "" && request["order_id"] == "" {
			t.Error("lookup request has no identifier")
		}
		_, _ = fmt.Fprint(w, `{"state":0,"result":{"uuid":"uuid-1","order_id":"order-1","amount":"10.50","currency":"USD","payment_status":"paid","is_final":true}}`)
	}))
	defer server.Close()

	client := NewClient(Config{MerchantID: "merchant-1", APIKey: "api-key", BaseURL: server.URL})
	invoice, err := client.GetInvoice(context.Background(), "uuid-1", "")
	if err != nil {
		t.Fatalf("GetInvoice by uuid: %v", err)
	}
	if !invoice.Paid() {
		t.Fatal("payment_status=paid must report Paid()")
	}
	if _, err := client.GetInvoice(context.Background(), "", "order-1"); err != nil {
		t.Fatalf("GetInvoice by order number: %v", err)
	}
	if _, err := client.GetInvoice(context.Background(), "", ""); err == nil {
		t.Fatal("lookup without identifiers must be rejected")
	}
}

func TestGatewayErrorsMapToAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"state":1,"message":"Payment not found"}`)
	}))
	defer server.Close()

	client := NewClient(Config{MerchantID: "merchant-1", APIKey: "api-key", BaseURL: server.URL})
	_, err := client.GetInvoice(context.Background(), "uuid-unknown", "")
	if err == nil {
		t.Fatal("gateway error must be returned")
	}
	if !IsNotFound(err) {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

func TestIsNotFoundRequiresExplicitPaymentError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"payment missing", &APIError{HTTPStatus: 422, State: 1, Message: "Payment not found"}, true},
		{"payment missing with punctuation", &APIError{HTTPStatus: 422, State: 1, Message: "Payment not found."}, true},
		{"payment missing 404", &APIError{HTTPStatus: 404, State: 1, Message: "Payment not found"}, true},
		{"proxy 404", &APIError{HTTPStatus: 404, Message: "non-JSON response"}, false},
		{"generic 404", &APIError{HTTPStatus: 404, State: 1, Message: "Not found"}, false},
		{"merchant missing", &APIError{HTTPStatus: 404, State: 1, Message: "Merchant not found"}, false},
		{"notification missing", &APIError{HTTPStatus: 422, State: 1, Message: "Notification not found"}, false},
		{"service missing", &APIError{HTTPStatus: 422, State: 1, Message: "Not found service to_currency"}, false},
		{"unsuccessful server", &APIError{HTTPStatus: 500, State: 1, Message: "Payment not found"}, false},
		{"invalid API state", &APIError{HTTPStatus: 404, Message: "Payment not found"}, false},
		{"no error", nil, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsNotFound(test.err); got != test.want {
				t.Fatalf("IsNotFound(%v) = %v, want %v", test.err, got, test.want)
			}
		})
	}
}

func TestNonJSONNotFoundIsNotMissingInvoice(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := NewClient(Config{MerchantID: "merchant-1", APIKey: "api-key", BaseURL: server.URL})
	_, err := client.GetInvoice(context.Background(), "uuid-1", "")
	if err == nil || IsNotFound(err) {
		t.Fatalf("proxy 404 must not mean a missing invoice, got %v", err)
	}
}

func TestInvoiceStatePrefersStatusOverPaymentStatus(t *testing.T) {
	// The create endpoint reports only payment_status; the info endpoint
	// fills both. State and Paid must work with either shape.
	infoOnly := &Invoice{PaymentStatus: "paid"}
	if infoOnly.State() != "paid" || !infoOnly.Paid() {
		t.Fatal("payment_status alone must drive the settlement state")
	}
	both := &Invoice{Status: "wrong_amount", PaymentStatus: "paid"}
	if both.State() != "wrong_amount" || both.Paid() {
		t.Fatal("a contradictory status field must win over payment_status")
	}
	if (&Invoice{}).Paid() {
		t.Fatal("an invoice without any status must not report paid")
	}
}

// Every invoice must ask for exactly the payment expectation of the order.
func TestInvoiceRequestSendsExactAmounts(t *testing.T) {
	for amount := int64(1); amount <= 100000; amount++ {
		request := newInvoiceRequest(Order{OrderNo: "order-1", Amount: amount, Currency: "usd"})
		sent, err := ParseMoney(request.Amount)
		if err != nil || sent != amount || request.Currency != "USD" {
			t.Fatalf("amount %d was sent as %q %s", amount, request.Amount, request.Currency)
		}
	}
}

func TestParseMoneyToleratesTrailingZeroDecimals(t *testing.T) {
	tests := []struct {
		value   string
		want    int64
		wantErr bool
	}{
		{value: "15", want: 1500},
		{value: "15.00", want: 1500},
		{value: "3.00000000", want: 300},
		{value: "3.14000000", want: 314},
		{value: "10.5", want: 1050},
		{value: "3.14159", wantErr: true},
		{value: "-1.00", wantErr: true},
		{value: "", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			got, err := ParseMoney(test.value)
			if (err != nil) != test.wantErr {
				t.Fatalf("ParseMoney(%q) error=%v", test.value, err)
			}
			if err == nil && got != test.want {
				t.Fatalf("ParseMoney(%q)=%d, want %d", test.value, got, test.want)
			}
		})
	}
}
