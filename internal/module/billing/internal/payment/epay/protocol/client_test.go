package protocol

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestClientRequiresExplicitBaseURL(t *testing.T) {
	testClient(t) // Initialize ephemeral fixture keys.
	config := Config{
		MerchantID:         "1001",
		MerchantPrivateKey: []byte(base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PrivateKey(fixtureKeys.merchant))),
		PlatformPublicKey:  []byte(base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&fixtureKeys.platform.PublicKey))),
	}
	if _, err := NewClient(config); err == nil || !strings.Contains(err.Error(), "BaseURL is required") {
		t.Fatalf("missing provider origin must fail explicitly: %v", err)
	}
	config.BaseURL = "https://custom-pay.example.com/"
	c, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := c.endpoint("/api/pay/query")
	if err != nil || endpoint != "https://custom-pay.example.com/api/pay/query" {
		t.Fatalf("configured provider origin: endpoint=%q err=%v", endpoint, err)
	}
}

func testServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	c := testClient(t)
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	c.baseURL = server.URL
	c.http = server.Client()
	c.http.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return c
}

func TestCallFormEncodingAndNoRetry(t *testing.T) {
	var calls atomic.Int32
	var requestProblem atomic.Value
	c := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/api/fail" {
			w.WriteHeader(503)
			return
		}
		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			requestProblem.Store("bad method or content type")
		}
		if err := r.ParseForm(); err != nil {
			requestProblem.Store(err.Error())
			w.WriteHeader(400)
			return
		}
		params := make(Params)
		for key, values := range r.PostForm {
			if !strings.Contains(key, "[") {
				params[key] = values[0]
			}
		}
		if err := verifyParams(&fixtureKeys.merchant.PublicKey, params); err != nil {
			requestProblem.Store(err.Error())
		}
		if r.PostForm.Get("param") != "A+B&=中文" || r.PostForm.Get("images[0]") != "img+1" || r.PostForm.Get("fee_mode") != "0" {
			requestProblem.Store("encoding changed values")
		}
		_, _ = w.Write(testEnvelope(t, map[string]any{"message": "ok"}))
	})
	input := Request{Params: Params{"param": "A+B&=中文", "fee_mode": "0"}, Arrays: map[string][]string{"images": {"img+1"}}}
	if _, err := c.Call(context.Background(), "/api/test", input); err != nil {
		t.Fatal(err)
	}
	if issue := requestProblem.Load(); issue != nil {
		t.Fatal(issue)
	}
	if len(input.Params) != 2 {
		t.Fatal("input mutated")
	}
	_, err := c.Call(context.Background(), "/api/fail", Request{})
	var httpError *HTTPError
	if !errors.As(err, &httpError) || httpError.StatusCode != 503 || calls.Load() != 2 {
		t.Fatalf("error/retry: %v calls=%d", err, calls.Load())
	}
	for _, path := range []string{"https://elsewhere.example/api/test", "//elsewhere/api/test", "/api/../other", "/api/a?secret=1", "/api/%2e%2e/a"} {
		if _, err := c.Call(context.Background(), path, Request{}); err == nil {
			t.Errorf("accepted %s", path)
		}
	}
	if _, err := c.Call(context.Background(), "/api/test", Request{Params: Params{"images": "scalar"}, Arrays: map[string][]string{"images": {"x"}}}); err == nil {
		t.Fatal("array collision")
	}
	if calls.Load() != 2 {
		t.Fatal("invalid request reached server")
	}
}

func TestRedirectAndCancellation(t *testing.T) {
	var redirected atomic.Int32
	c := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/target" {
			redirected.Add(1)
			w.WriteHeader(500)
			return
		}
		http.Redirect(w, r, "/api/target", http.StatusTemporaryRedirect)
	})
	_, err := c.Call(context.Background(), "/api/redirect", Request{})
	var httpError *HTTPError
	if !errors.As(err, &httpError) || httpError.StatusCode != 307 || redirected.Load() != 0 {
		t.Fatalf("redirect: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Call(ctx, "/api/test", Request{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestTypedPayments(t *testing.T) {
	var mode atomic.Int32
	c := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		var result map[string]any
		switch r.URL.Path {
		case "/api/pay/create":
			result = map[string]any{"trade_no": "platform-1", "pay_type": "jump", "pay_info": "https://pay.example/checkout"}
		case "/api/pay/query":
			result = map[string]any{"pid": 1001, "trade_no": "platform-1", "out_trade_no": "order-1", "type": "alipay", "money": "1.00", "status": 1}
			if mode.Load() == 1 {
				result["out_trade_no"] = "another-order"
			}
			if mode.Load() == 2 {
				delete(result, "status")
			}
		case "/api/pay/refund", "/api/pay/refundquery":
			result = map[string]any{"trade_no": "platform-1", "out_trade_no": "order-1", "refund_no": "refund-1", "out_refund_no": "local-refund-1", "money": "1.00", "status": 1}
			if mode.Load() == 3 {
				result["money"] = "2.00"
			}
		case "/api/pay/close":
			result = map[string]any{"msg": "ok"}
		default:
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write(testEnvelope(t, result))
	})
	ctx := context.Background()
	payment := Payment{Type: "alipay", OutTradeNo: "order-1", NotifyURL: "https://merchant.example/notify", ReturnURL: "https://merchant.example/return", Name: "会员 & 服务", Money: "1.00"}
	created, err := c.CreatePayment(ctx, CreatePaymentRequest{Payment: payment, Method: "jump", ClientIP: "127.0.0.1"})
	if err != nil || created.PayType != "jump" {
		t.Fatalf("create: %v", err)
	}
	form, err := c.PagePayment(payment)
	if err != nil || form.Fields.Get("name") != payment.Name {
		t.Fatalf("page: %v", err)
	}
	params := make(Params)
	for key, values := range form.Fields {
		params[key] = values[0]
	}
	if err := verifyParams(&fixtureKeys.merchant.PublicKey, params); err != nil {
		t.Fatal(err)
	}
	ref := OrderRef{OutTradeNo: "order-1"}
	order, err := c.QueryOrder(ctx, ref)
	if err != nil || order.Status != OrderPaid {
		t.Fatalf("query: %v", err)
	}
	if err := c.CloseOrder(ctx, ref); err != nil {
		t.Fatal(err)
	}
	request := RefundRequest{OrderRef: ref, Money: "1.00", OutRefundNo: "local-refund-1"}
	if _, err := c.Refund(ctx, request); err != nil {
		t.Fatal(err)
	}
	refund, err := c.QueryRefund(ctx, RefundRef{OutRefundNo: "local-refund-1"})
	if err != nil || refund.Status != 1 {
		t.Fatalf("refund query: %v", err)
	}
	for _, m := range []int32{1, 2} {
		mode.Store(m)
		if _, err := c.QueryOrder(ctx, ref); err == nil {
			t.Fatal("mismatched/incomplete order accepted")
		}
	}
	mode.Store(3)
	if _, err := c.Refund(ctx, request); err == nil {
		t.Fatal("mismatched refund amount accepted")
	}
	if _, err := c.Refund(ctx, RefundRequest{OrderRef: ref, Money: "1.00"}); err == nil {
		t.Fatal("refund missing id accepted")
	}
	if _, err := c.QueryOrder(ctx, OrderRef{}); err == nil {
		t.Fatal("empty order ref accepted")
	}
	if _, err := c.CreatePayment(ctx, CreatePaymentRequest{Payment: payment, Method: "scan", ClientIP: "127.0.0.1"}); err == nil {
		t.Fatal("scan missing auth code accepted")
	}
}

func TestMultipartImage(t *testing.T) {
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	var requestProblem atomic.Value
	c := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			requestProblem.Store(err.Error())
			w.WriteHeader(400)
			return
		}
		defer func() { _ = r.MultipartForm.RemoveAll() }()
		params := make(Params)
		for key, values := range r.MultipartForm.Value {
			params[key] = values[0]
		}
		if err := verifyParams(&fixtureKeys.merchant.PublicKey, params); err != nil {
			requestProblem.Store(err.Error())
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			requestProblem.Store(err.Error())
			w.WriteHeader(400)
			return
		}
		defer func() { _ = file.Close() }()
		data, _ := io.ReadAll(file)
		if !bytes.Equal(data, pngData.Bytes()) || params["id"] != "10001" {
			requestProblem.Store("incorrect multipart fields")
		}
		_, _ = w.Write(testEnvelope(t, map[string]any{"image_id": "image-1"}))
	})
	id, err := c.UploadComplaintImage(context.Background(), "10001", pngData.Bytes())
	if err != nil || id != "image-1" {
		t.Fatalf("upload: %v", err)
	}
	if issue := requestProblem.Load(); issue != nil {
		t.Fatal(issue)
	}
	if _, err := c.UploadComplaintImage(context.Background(), "10001", []byte("text")); err == nil {
		t.Fatal("nonimage accepted")
	}
}

func TestResponseSizeLimit(t *testing.T) {
	c := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), maxResponseBytes+1))
	})
	if _, err := c.Call(context.Background(), "/api/test", Request{}); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("size limit: %v", err)
	}
}

func TestRequiredScalarCannotHideInUnsignedObject(t *testing.T) {
	c := testClient(t)
	response, err := c.parseResponse(testEnvelope(t, map[string]any{"money": map[string]string{"value": "1.00"}}))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]json.RawMessage
	if err := response.decodeRequired(&result, "money"); err == nil {
		t.Fatal("required scalar accepted as unsigned object")
	}
}

func TestClientAcceptsDeploymentPath(t *testing.T) {
	testClient(t)
	config := Config{
		MerchantID:         "1001",
		MerchantPrivateKey: []byte(base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PrivateKey(fixtureKeys.merchant))),
		PlatformPublicKey:  []byte(base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&fixtureKeys.platform.PublicKey))),
	}
	for base, want := range map[string]string{
		"https://pay.example.com":         "https://pay.example.com/api/pay/query",
		"https://pay.example.com/":        "https://pay.example.com/api/pay/query",
		"https://pay.example.com/pay":     "https://pay.example.com/pay/api/pay/query",
		"https://pay.example.com/pay/":    "https://pay.example.com/pay/api/pay/query",
		"https://pay.example.com/a/b/pay": "https://pay.example.com/a/b/pay/api/pay/query",
	} {
		config.BaseURL = base
		c, err := NewClient(config)
		if err != nil {
			t.Fatalf("%s: %v", base, err)
		}
		if endpoint, err := c.endpoint("/api/pay/query"); err != nil || endpoint != want {
			t.Fatalf("%s: endpoint=%q err=%v", base, endpoint, err)
		}
	}
	for _, base := range []string{
		"http://pay.example.com/pay", "https://pay.example.com/pay?x=1", "https://pay.example.com/pay#f",
		"https://user@pay.example.com/pay", "https://pay.example.com/a/../pay", "https://pay.example.com/a//pay",
		"https://pay.example.com/p%2Fay", "https://pay.example.com/./pay",
		"https://pay.example.com/pay%23checkout", "https://pay.example.com/pay%3Froute=wrong",
		"https://pay.example.com/pay%20space", "https://pay.example.com/pay%09tab",
		"https://pay.example.com/pay%0Aline", "https://pay.example.com/pay%0Dline",
		"https://pay.example.com//", "https://pay.example.com///", "https://pay.example.com/pay//",
	} {
		config.BaseURL = base
		if _, err := NewClient(config); err == nil {
			t.Errorf("invalid base URL accepted: %q", base)
		}
	}
}
