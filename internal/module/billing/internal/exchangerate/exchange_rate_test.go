package exchangerate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// rateAPI answers the currency conversion endpoint like the rate provider.
func rateAPI(t *testing.T, result string, calls *int) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		if r.URL.Path != "/currency_data/convert" || r.Header.Get("apikey") != "access-key" {
			t.Errorf("unexpected request %s apikey=%q", r.URL.Path, r.Header.Get("apikey"))
		}
		if q := r.URL.Query(); q.Get("from") != "USD" || q.Get("to") != "CNY" || q.Get("amount") != "1" {
			t.Errorf("unexpected query %v", q)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(result))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestConvertReadsTheProviderResult(t *testing.T) {
	calls := 0
	server := rateAPI(t, `{"success":true,"result":7.1234}`, &calls)
	rate, err := convert(server.URL, "USD", "CNY", "access-key", 1)
	if err != nil || rate != 7.1234 || calls != 1 {
		t.Fatalf("convert = (%v, %v) after %d calls", rate, err, calls)
	}
	failing := rateAPI(t, `{"success":false}`, &calls)
	if _, err := convert(failing.URL, "USD", "CNY", "access-key", 1); err == nil {
		t.Fatal("an unsuccessful answer must be an error")
	}
}

func TestSourcePrefersTheRefreshedRate(t *testing.T) {
	calls := 0
	server := rateAPI(t, `{"success":true,"result":7.2}`, &calls)
	cache := NewCache(6.9)
	source := Source{Cache: cache, AccessKey: "access-key", BaseURL: server.URL}
	if rate, err := source.Rate(context.Background(), "USD", "CNY"); err != nil || rate != 6.9 || calls != 0 {
		t.Fatalf("cached rate = (%v, %v), api calls %d", rate, err, calls)
	}
	cache.Set(0)
	if rate, err := source.Rate(context.Background(), "USD", "CNY"); err != nil || rate != 7.2 || calls != 1 {
		t.Fatalf("fetched rate = (%v, %v), api calls %d", rate, err, calls)
	}
	if cache.Get() != 7.2 {
		t.Fatalf("fetched rate was not cached: %v", cache.Get())
	}
}

func TestSourceWithoutAccessKeyRefusesToConvert(t *testing.T) {
	if _, err := (Source{Cache: NewCache(0)}).Rate(context.Background(), "USD", "CNY"); err == nil {
		t.Fatal("a missing rate must refuse the conversion")
	}
}
