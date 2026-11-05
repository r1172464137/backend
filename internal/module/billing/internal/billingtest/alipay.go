package billingtest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

var (
	alipayKeyOnce sync.Once
	alipayKey     *rsa.PrivateKey
	alipayKeyErr  error
)

// AlipayKey is the RSA key pair every Alipay test shares as both the
// merchant and the gateway key; it only exists so the SDK's signing and
// verification succeed.
func AlipayKey(t testing.TB) *rsa.PrivateKey {
	t.Helper()
	alipayKeyOnce.Do(func() { alipayKey, alipayKeyErr = rsa.GenerateKey(rand.Reader, 2048) })
	if alipayKeyErr != nil {
		t.Fatalf("generate RSA key: %v", alipayKeyErr)
	}
	return alipayKey
}

// AlipayKeys returns AlipayKey encoded as the payment configuration stores
// it: the PKCS#1 private key and the PKIX public key, base64.
func AlipayKeys(t testing.TB) (private, public string) {
	t.Helper()
	key := AlipayKey(t)
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	return base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PrivateKey(key)), base64.StdEncoding.EncodeToString(publicDER)
}

// AlipaySign signs content as the gateway does: RSA-SHA256, base64.
func AlipaySign(t testing.TB, content string) string {
	t.Helper()
	digest := sha256.Sum256([]byte(content))
	signature, err := rsa.SignPKCS1v15(rand.Reader, AlipayKey(t), crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return base64.StdEncoding.EncodeToString(signature)
}

// AlipayConfig is a face-to-face payment configuration of appID whose
// sandbox gateway is gatewayURL.
func AlipayConfig(t testing.TB, appID, gatewayURL string) string {
	t.Helper()
	private, public := AlipayKeys(t)
	return fmt.Sprintf(`{"app_id":%q,"private_key":%q,"public_key":%q,"sandbox":true,"gateway":%q}`, appID, private, public, gatewayURL)
}

// AlipayAnswer is the gateway's answer to one call: the business JSON, and
// whether it carries the gateway signature. Unsigned answers mimic gateway
// business failures.
type AlipayAnswer struct {
	Biz    string
	Signed bool
}

// FakeAlipay impersonates the Alipay OpenAPI gateway. Respond receives the
// method, its per-method call number and the request's business content.
type FakeAlipay struct {
	t testing.TB
	// URL is the gateway address to configure.
	URL     string
	respond func(method string, call int, biz map[string]any) AlipayAnswer

	mu    sync.Mutex
	calls map[string]int
	last  map[string]map[string]any
}

// NewFakeAlipay starts a fake gateway for the test.
func NewFakeAlipay(t testing.TB, respond func(method string, call int, biz map[string]any) AlipayAnswer) *FakeAlipay {
	t.Helper()
	g := &FakeAlipay{t: t, respond: respond, calls: map[string]int{}, last: map[string]map[string]any{}}
	AlipayKey(t)
	server := httptest.NewServer(g)
	t.Cleanup(server.Close)
	g.URL = server.URL
	return g
}

func (g *FakeAlipay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		g.t.Errorf("alipay gateway: parse form: %v", err)
		return
	}
	method := r.Form.Get("method")
	biz := map[string]any{}
	if content := r.Form.Get("biz_content"); content != "" {
		if err := json.Unmarshal([]byte(content), &biz); err != nil {
			g.t.Errorf("alipay gateway: decode biz content: %v", err)
		}
	}
	g.mu.Lock()
	g.calls[method]++
	call := g.calls[method]
	g.last[method] = biz
	g.mu.Unlock()

	answer := g.respond(method, call, biz)
	field := strings.ReplaceAll(method, ".", "_") + "_response"
	if !answer.Signed {
		_, _ = fmt.Fprintf(w, `{%q:%s}`, field, answer.Biz)
		return
	}
	_, _ = fmt.Fprintf(w, `{%q:%s,"sign":%q}`, field, answer.Biz, AlipaySign(g.t, answer.Biz))
}

// Calls counts the calls of method.
func (g *FakeAlipay) Calls(method string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls[method]
}

// LastBiz is the business content of the last call of method.
func (g *FakeAlipay) LastBiz(method string) map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.last[method]
}
