package epay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestVerifySignRejectsMissingAndInvalidSignatures(t *testing.T) {
	client := NewClient("merchant-1", "https://pay.example", "secret", "alipay")
	params := map[string]string{
		"pid":          "merchant-1",
		"out_trade_no": "order-1",
		"trade_no":     "trade-1",
		"money":        "10.00",
		"trade_status": "TRADE_SUCCESS",
		"type":         "alipay",
		"sign_type":    "MD5",
	}

	if client.VerifySign(params) {
		t.Fatal("missing signature must be rejected")
	}
	params["sign"] = "00000000000000000000000000000000"
	if client.VerifySign(params) {
		t.Fatal("invalid signature must be rejected")
	}
	params["sign"] = client.createSign(params)
	if !client.VerifySign(params) {
		t.Fatal("valid signature must be accepted")
	}
	params["money"] = "0.01"
	if client.VerifySign(params) {
		t.Fatal("changing a signed parameter must invalidate the signature")
	}
}

func TestCreatePayUrlBuildsSignedSubmitRedirect(t *testing.T) {
	client := NewClient("1001", "https://pay.example/", "secret", "alipay")
	payURL, err := client.CreatePayUrl(Order{
		Name: "product", OrderNo: "order-1", Amount: 10,
		NotifyUrl: "https://merchant.example/notify", ReturnUrl: "https://merchant.example/return",
	})
	if err != nil {
		t.Fatalf("CreatePayUrl: %v", err)
	}
	if !strings.HasPrefix(payURL, "https://pay.example/submit.php?") {
		t.Fatalf("unexpected submit redirect: %s", payURL)
	}
	parsed, err := url.Parse(payURL)
	if err != nil {
		t.Fatal(err)
	}
	params := make(map[string]string, len(parsed.Query()))
	for name := range parsed.Query() {
		params[name] = parsed.Query().Get(name)
	}
	if params["sign_type"] != "MD5" || !client.VerifySign(params) {
		t.Fatalf("submit redirect has invalid signature: %+v", params)
	}
}

func TestQueryOrderReturnsAuthoritativeGatewayFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/gateway/api.php" {
			t.Errorf("path=%q", got)
		}
		for name, want := range map[string]string{
			"act": "order", "pid": "1001", "key": "secret", "out_trade_no": "order-1",
		} {
			if got := r.URL.Query().Get(name); got != want {
				t.Errorf("query %s=%q, want %q", name, got, want)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 1, "msg": "ok", "pid": 1001, "trade_no": "trade-1",
			"out_trade_no": "order-1", "type": "alipay", "money": "10.00", "status": 1,
		})
	}))
	defer server.Close()

	client := NewClient("1001", server.URL+"/gateway", "secret", "alipay")
	result, err := client.QueryOrder(context.Background(), "order-1")
	if err != nil {
		t.Fatalf("QueryOrder: %v", err)
	}
	if result.MerchantID != "1001" || result.OrderNo != "order-1" || result.TradeNo != "trade-1" || result.Money != "10.00" || !result.Paid {
		t.Fatalf("unexpected query result: %+v", result)
	}
}

func TestQueryOrderFallsBackToEasyPayStatusQuery(t *testing.T) {
	var standardRequests, fallbackRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gateway/api.php":
			standardRequests++
			if r.Method != http.MethodGet {
				t.Errorf("standard method=%s, want GET", r.Method)
			}
			http.NotFound(w, r)
		case "/gateway/api/EasyPay/queryOrder":
			fallbackRequests++
			if r.Method != http.MethodPost {
				t.Errorf("fallback method=%s, want POST", r.Method)
			}
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
				t.Errorf("fallback content-type=%q", r.Header.Get("Content-Type"))
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if got := r.PostForm.Get("orderNo"); got != "order-1" {
				t.Errorf("orderNo=%q, want order-1", got)
			}
			if len(r.PostForm) != 1 {
				t.Errorf("fallback params=%v, want only orderNo", r.PostForm)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 1, "msg": "query success", "data": map[string]string{"status": "success"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("1001", server.URL+"/gateway", "secret", "alipay")
	result, err := client.QueryOrder(context.Background(), "order-1")
	if err != nil {
		t.Fatalf("QueryOrder: %v", err)
	}
	if standardRequests != 1 || fallbackRequests != 1 {
		t.Fatalf("standard=%d fallback=%d, want one request each", standardRequests, fallbackRequests)
	}
	if !result.Paid || !result.StatusOnly || result.Message != "query success" {
		t.Fatalf("unexpected fallback result: %+v", result)
	}
}

func TestQueryOrderEasyPayFallbackReportsUnpaid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api.php" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 1, "data": map[string]string{"status": "pending"},
		})
	}))
	defer server.Close()

	result, err := NewClient("1001", server.URL, "secret", "alipay").QueryOrder(context.Background(), "order-1")
	if err != nil {
		t.Fatalf("QueryOrder: %v", err)
	}
	if result.Paid || !result.StatusOnly {
		t.Fatalf("unexpected fallback result: %+v", result)
	}
}

// Only an explicit standard status 0 means "awaiting payment"; an omitted,
// refunded or frozen status and a status-only answer leave the state unknown.
func TestQueryOrderReportsExplicitAwaitingPayment(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		paid, unpaid bool
	}{
		{"awaiting payment", `{"code":1,"pid":1001,"out_trade_no":"order-1","status":0}`, false, true},
		{"paid", `{"code":1,"pid":1001,"out_trade_no":"order-1","status":1}`, true, false},
		{"integer strings", `{"code":"1","pid":"1001","out_trade_no":"order-1","status":"1"}`, true, false},
		{"unpaid integer strings", `{"code":"1","pid":"1001","out_trade_no":"order-1","status":"0"}`, false, true},
		{"refunded", `{"code":1,"pid":1001,"out_trade_no":"order-1","status":2}`, false, false},
		{"status omitted", `{"code":1,"pid":1001,"out_trade_no":"order-1"}`, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			result, err := NewClient("1001", server.URL, "secret", "alipay").QueryOrder(context.Background(), "order-1")
			if err != nil {
				t.Fatalf("QueryOrder: %v", err)
			}
			if result.Paid != tt.paid || result.Unpaid != tt.unpaid {
				t.Fatalf("result = %+v, want paid=%t unpaid=%t", result, tt.paid, tt.unpaid)
			}
		})
	}

	statusOnly := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api.php" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"code":1,"data":{"status":"pending"}}`))
	}))
	defer statusOnly.Close()
	result, err := NewClient("1001", statusOnly.URL, "secret", "alipay").QueryOrder(context.Background(), "order-1")
	if err != nil {
		t.Fatalf("QueryOrder: %v", err)
	}
	if result.Paid || result.Unpaid {
		t.Fatalf("status-only result = %+v, want neither paid nor awaiting payment", result)
	}
}

func TestQueryOrderRejectsMismatchedOrderAndRedirects(t *testing.T) {
	for _, body := range []string{
		`{"code":1,"pid":"another","out_trade_no":"order-1","status":0}`,
		`{"code":1,"pid":"1001","out_trade_no":"another","status":0}`,
		`{"code":1,"pid":"1001","out_trade_no":"order-1","status":"1.0"}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
		_, err := NewClient("1001", server.URL, "secret", "").QueryOrder(context.Background(), "order-1")
		server.Close()
		if err == nil {
			t.Fatal("mismatched or malformed query accepted")
		}
	}
	var redirected bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/other" {
			redirected = true
			return
		}
		http.Redirect(w, r, "/other", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	_, err := NewClient("1001", server.URL, "secret", "").QueryOrder(context.Background(), "order-1")
	if err == nil || redirected {
		t.Fatal("gateway query followed a redirect")
	}
}

func TestQueryOrderRejectsUnsuccessfulLookup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":-1,"msg":"not found"}`))
	}))
	defer server.Close()

	client := NewClient("1001", server.URL, "secret", "alipay")
	if _, err := client.QueryOrder(context.Background(), "order-1"); err == nil {
		t.Fatal("unsuccessful gateway lookup must be rejected")
	}
}

func TestQueryOrderReportsUnsupportedWhenGatewayReturnsNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient("1001", server.URL, "secret", "alipay")
	if _, err := client.QueryOrder(context.Background(), "order-1"); !errors.Is(err, ErrQueryNotSupported) {
		t.Fatalf("QueryOrder error=%v, want ErrQueryNotSupported", err)
	}
}

// A gateway that answers the query path with an HTML error page does not
// implement the query protocol; that must classify as unsupported so
// signature-verified callbacks and close attempts are not blocked by it.
func TestQueryOrderTreatsNonJSONResponseAsUnsupported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>502 Bad Gateway</body></html>"))
	}))
	defer server.Close()

	client := NewClient("1001", server.URL+"/gateway", "secret", "alipay")
	_, err := client.QueryOrder(context.Background(), "order-1")
	if !errors.Is(err, ErrQueryNotSupported) {
		t.Fatalf("QueryOrder error = %v, want ErrQueryNotSupported", err)
	}
}

func TestQueryOrderDoesNotTreatOtherHTTPFailuresAsUnsupported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "gateway failure", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient("1001", server.URL, "secret", "alipay")
	if _, err := client.QueryOrder(context.Background(), "order-1"); errors.Is(err, ErrQueryNotSupported) {
		t.Fatal("only a 404 response may be treated as an unsupported query API")
	}
}
