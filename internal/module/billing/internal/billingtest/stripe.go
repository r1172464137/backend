package billingtest

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stripe/stripe-go/v81"
)

// FakeStripe is an in-process Stripe API answering the customer, ephemeral
// key, PaymentIntent and webhook endpoint calls billing makes. It records
// which secret key created each intent, so tests can prove that clients
// never borrow each other's key.
type FakeStripe struct {
	t testing.TB
	// Backends routes a Stripe client to the fake.
	Backends *stripe.Backends

	mu        sync.Mutex
	intents   map[string]map[string]any
	keys      map[string]string // order number -> secret key that created its intent
	created   int
	canceled  []string
	customers int
	webhooks  map[string]string // endpoint id -> url
}

// NewFakeStripe starts a fake Stripe API for the test.
func NewFakeStripe(t testing.TB) *FakeStripe {
	t.Helper()
	fake := &FakeStripe{t: t, intents: map[string]map[string]any{}, keys: map[string]string{}, webhooks: map[string]string{}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	fake.Backends = stripe.NewBackendsWithConfig(&stripe.BackendConfig{
		URL:               stripe.String(server.URL),
		HTTPClient:        server.Client(),
		MaxNetworkRetries: stripe.Int64(0),
		LeveledLogger:     &stripe.LeveledLogger{Level: stripe.LevelNull},
	})
	return fake
}

func (f *FakeStripe) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		f.t.Errorf("parse form: %v", err)
	}
	key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/v1/")
	switch {
	case r.Method == http.MethodGet && path == "customers/search":
		f.write(w, map[string]any{"object": "search_result", "data": []any{}, "has_more": false, "url": "/v1/customers/search"})
	case r.Method == http.MethodPost && path == "customers":
		f.customers++
		f.write(w, map[string]any{"id": fmt.Sprintf("cus_%d", f.customers), "object": "customer", "email": r.PostForm.Get("email")})
	case r.Method == http.MethodPost && path == "ephemeral_keys":
		f.write(w, map[string]any{"id": "ephkey_1", "object": "ephemeral_key", "secret": "ek_test_secret"})
	case r.Method == http.MethodPost && path == "payment_intents":
		orderNo := r.PostForm.Get("metadata[order_no]")
		f.created++
		id := "pi_" + orderNo
		if _, taken := f.intents[id]; taken {
			id = fmt.Sprintf("pi_%s_%d", orderNo, f.created)
		}
		amount, _ := strconv.ParseInt(r.PostForm.Get("amount"), 10, 64)
		intent := newIntent(id, amount, r.PostForm.Get("currency"), "requires_payment_method", orderNo, r.PostForm.Get("payment_method_types[0]"))
		intent["metadata"].(map[string]string)["user_id"] = r.PostForm.Get("metadata[user_id]")
		f.intents[id] = intent
		f.keys[orderNo] = key
		f.write(w, intent)
	case strings.HasPrefix(path, "payment_intents/") && strings.HasSuffix(path, "/cancel"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "payment_intents/"), "/cancel")
		// Like Stripe, a succeeded or already canceled intent cannot be
		// canceled (payment_intent_unexpected_state).
		intent, ok := f.intents[id]
		if !ok || intent["status"] == "succeeded" || intent["status"] == "canceled" {
			w.WriteHeader(http.StatusBadRequest)
			f.write(w, map[string]any{"error": map[string]any{"type": "invalid_request_error", "code": "payment_intent_unexpected_state", "message": "cannot cancel payment_intent"}})
			return
		}
		intent["status"] = "canceled"
		f.canceled = append(f.canceled, id)
		f.write(w, intent)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "payment_intents/"):
		intent, ok := f.intents[strings.TrimPrefix(path, "payment_intents/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			f.write(w, map[string]any{"error": map[string]any{"type": "invalid_request_error", "message": "No such payment_intent"}})
			return
		}
		f.write(w, intent)
	case r.Method == http.MethodPost && path == "webhook_endpoints":
		id := fmt.Sprintf("we_%d", len(f.webhooks)+1)
		f.webhooks[id] = r.PostForm.Get("url")
		f.write(w, map[string]any{"id": id, "object": "webhook_endpoint", "url": r.PostForm.Get("url"), "secret": "whsec_" + id})
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "webhook_endpoints/"):
		id := strings.TrimPrefix(path, "webhook_endpoints/")
		delete(f.webhooks, id)
		f.write(w, map[string]any{"id": id, "object": "webhook_endpoint", "deleted": true})
	default:
		f.t.Errorf("unexpected Stripe request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func newIntent(id string, amount int64, currency, status, orderNo, method string) map[string]any {
	return map[string]any{
		"id": id, "object": "payment_intent", "amount": amount, "currency": currency,
		"client_secret": id + "_secret", "status": status,
		"metadata":             map[string]string{"order_no": orderNo},
		"payment_method_types": []string{method},
	}
}

func (f *FakeStripe) write(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		f.t.Errorf("encode response: %v", err)
	}
}

// Seed stores an existing PaymentIntent of orderNo.
func (f *FakeStripe) Seed(id string, amount int64, currency, status, orderNo, method string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.intents[id] = newIntent(id, amount, currency, status, orderNo, method)
}

// SetStatus changes the status of intent id.
func (f *FakeStripe) SetStatus(id, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.intents[id]["status"] = status
}

// Status reports the status of intent id.
func (f *FakeStripe) Status(id string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	status, _ := f.intents[id]["status"].(string)
	return status
}

// Created counts the intents created through the API.
func (f *FakeStripe) Created() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.created
}

// Keys maps each order number to the secret key that created its intent.
func (f *FakeStripe) Keys() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return maps.Clone(f.keys)
}

// Canceled lists the cancelled intents in order.
func (f *FakeStripe) Canceled() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.canceled)
}

// Customers counts the customers created.
func (f *FakeStripe) Customers() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.customers
}

// Webhooks maps each registered endpoint id to its URL.
func (f *FakeStripe) Webhooks() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return maps.Clone(f.webhooks)
}
