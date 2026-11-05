package protocol

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const testTimestamp = "1700000000"

var fixtureKeys struct {
	sync.Once
	merchant *rsa.PrivateKey
	platform *rsa.PrivateKey
	err      error
}

func testClient(t *testing.T) *Client {
	t.Helper()
	fixtureKeys.Do(func() {
		fixtureKeys.merchant, fixtureKeys.err = rsa.GenerateKey(rand.Reader, 2048)
		if fixtureKeys.err == nil {
			fixtureKeys.platform, fixtureKeys.err = rsa.GenerateKey(rand.Reader, 2048)
		}
	})
	if fixtureKeys.err != nil {
		t.Fatal(fixtureKeys.err)
	}
	public, err := x509.MarshalPKIXPublicKey(&fixtureKeys.platform.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(Config{
		BaseURL:            "https://pay.example.com",
		MerchantID:         "1001",
		MerchantPrivateKey: pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(fixtureKeys.merchant)}),
		PlatformPublicKey:  pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: public}),
	})
	if err != nil {
		t.Fatal(err)
	}
	client.now = func() time.Time { return time.Unix(1700000000, 900000000) }
	return client
}

// Test platform signer uses crypto/rsa directly, independently of signParams.
func testSignature(t *testing.T, key *rsa.PrivateKey, fields map[string]any) string {
	t.Helper()
	var parts []string
	for name, value := range fields {
		if name == "sign" || name == "sign_type" || value == nil {
			continue
		}
		var scalar string
		switch v := value.(type) {
		case string:
			scalar = v
		case int:
			scalar = fmt.Sprint(v)
		case json.Number:
			scalar = v.String()
		case bool:
			if v {
				scalar = "1"
			}
		default:
			continue
		}
		if strings.Trim(scalar, " \t\r\n\x00\x0b") != "" {
			parts = append(parts, name+"="+scalar)
		}
	}
	sort.Strings(parts)
	hash := sha256.Sum256([]byte(strings.Join(parts, "&")))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(signature)
}

func testEnvelope(t *testing.T, fields map[string]any) []byte {
	t.Helper()
	if _, ok := fields["code"]; !ok {
		fields["code"] = 0
	}
	if _, ok := fields["timestamp"]; !ok {
		fields["timestamp"] = testTimestamp
	}
	fields["sign_type"] = "RSA"
	fields["sign"] = testSignature(t, fixtureKeys.platform, fields)
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func testNotice(t *testing.T, changes Params) url.Values {
	t.Helper()
	fields := map[string]any{
		"pid": "1001", "trade_no": "platform-1", "out_trade_no": "order-1",
		"money": "1.00", "type": "alipay", "trade_status": "TRADE_SUCCESS",
		"timestamp": testTimestamp, "sign_type": "RSA", "future_zero": "0",
	}
	for key, value := range changes {
		fields[key] = value
	}
	values := make(url.Values)
	for key, value := range fields {
		values.Set(key, value.(string))
	}
	values.Set("sign", testSignature(t, fixtureKeys.platform, fields))
	return values
}

func TestCanonicalString(t *testing.T) {
	input := Params{
		"name": " 会员 A&B=+ ", "money": "1.00", "fee_mode": "0", "zero": "0",
		"sign": "ignored", "sign_type": "RSA", "empty": "", "blank": " \r\n\t\x00\x0b",
		"formfeed": "\x0c", // PHP trim does not remove form feed.
	}
	got, err := CanonicalString(input)
	want := "fee_mode=0&formfeed=\x0c&money=1.00&name= 会员 A&B=+ &zero=0"
	if err != nil || got != want {
		t.Fatalf("got %q (%v), want %q", got, err, want)
	}
	if _, err := CanonicalString(Params{"money&other": "1"}); err == nil {
		t.Fatal("accepted ambiguous key")
	}
}

func TestKeyFormatsAndSignature(t *testing.T) {
	c := testClient(t)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(fixtureKeys.merchant)
	if err != nil {
		t.Fatal(err)
	}
	pkcs1 := x509.MarshalPKCS1PrivateKey(fixtureKeys.merchant)
	for _, der := range [][]byte{pkcs1, pkcs8} {
		for _, input := range [][]byte{
			[]byte(base64.StdEncoding.EncodeToString(der)),
			pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}),
		} {
			key, err := parsePrivateKey(input)
			if err != nil || key.N.Cmp(c.privateKey.N) != 0 {
				t.Fatalf("private key parse: %v", err)
			}
		}
	}
	pkix, err := x509.MarshalPKIXPublicKey(&fixtureKeys.platform.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, der := range [][]byte{pkix, x509.MarshalPKCS1PublicKey(&fixtureKeys.platform.PublicKey)} {
		for _, input := range [][]byte{[]byte(base64.StdEncoding.EncodeToString(der)), pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})} {
			if _, err := parsePublicKey(input); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, input := range [][]byte{nil, []byte("invalid"), append(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}), 'x')} {
		if _, err := parsePrivateKey(input); err == nil {
			t.Fatal("accepted invalid key")
		}
	}
	params := Params{"money": "1.00", "name": "会员 A&B", "fee_mode": "0", "sign_type": "RSA"}
	signature, err := signParams(c.privateKey, params)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("fee_mode=0&money=1.00&name=会员 A&B"))
	raw, _ := base64.StdEncoding.DecodeString(signature)
	if err := rsa.VerifyPKCS1v15(&fixtureKeys.merchant.PublicKey, crypto.SHA256, digest[:], raw); err != nil {
		t.Fatal(err)
	}
	params["sign"] = signature
	if err := verifyParams(&fixtureKeys.platform.PublicKey, params); err == nil {
		t.Fatal("wrong key accepted")
	}
}

func TestSignedParamsDoNotMutateInput(t *testing.T) {
	c := testClient(t)
	params := Params{"money": "1.00"}
	got, err := c.SignedParams(params)
	if err != nil {
		t.Fatal(err)
	}
	if len(params) != 1 || got["timestamp"] != testTimestamp || got["pid"] != "1001" {
		t.Fatal("bad injected fields or mutation")
	}
	if err := verifyParams(&fixtureKeys.merchant.PublicKey, got); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SignedParams(Params{"pid": "other"}); err == nil {
		t.Fatal("reserved override accepted")
	}
}

func TestVerifyNotification(t *testing.T) {
	c := testClient(t)
	valid := testNotice(t, nil)
	notice, err := c.VerifyNotification(valid)
	if err != nil || notice.OutTradeNo != "order-1" || notice.Fields["future_zero"] != "0" {
		t.Fatalf("valid: %v", err)
	}
	for _, timestamp := range []string{"1699999700", "1700000300"} {
		if _, err := c.VerifyNotification(testNotice(t, Params{"timestamp": timestamp})); err != nil {
			t.Fatalf("boundary %s: %v", timestamp, err)
		}
	}
	for name, changes := range map[string]Params{
		"expired": {"timestamp": "1699999699"}, "future": {"timestamp": "1700000301"},
		"milliseconds": {"timestamp": "1700000000000"}, "missing timestamp": {"timestamp": ""},
		"merchant": {"pid": "1002"}, "missing merchant": {"pid": ""},
		"status": {"trade_status": "WAIT_BUYER_PAY"}, "missing order": {"out_trade_no": ""},
		"zero amount": {"money": "0"}, "fraction": {"money": "1.001"}, "algorithm": {"sign_type": "MD5"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := c.VerifyNotification(testNotice(t, changes)); err == nil {
				t.Fatal("accepted invalid notification")
			}
		})
	}
	valid.Set("money", "2.00")
	if _, err := c.VerifyNotification(valid); err == nil {
		t.Fatal("tamper accepted")
	}
	valid = testNotice(t, nil)
	valid.Add("money", "1.00")
	if _, err := c.VerifyNotification(valid); err == nil {
		t.Fatal("duplicate accepted")
	}
	valid = testNotice(t, nil)
	valid.Del("future_zero")
	if _, err := c.VerifyNotification(valid); err == nil {
		t.Fatal("extension omitted from signature")
	}
}

func TestAmount(t *testing.T) {
	for value, want := range map[string]int64{"0": 0, "1": 100, "1.0": 100, "1.01": 101, "0.01": 1, "92233720368547758.07": 9223372036854775807} {
		got, err := ParseAmount(value)
		if err != nil || got != want {
			t.Fatalf("%s => %d, %v", value, got, err)
		}
	}
	for _, value := range []string{"", "01.00", "1.001", "-1", "+1", "1e2", " 1.00", ".1", "1.", "NaN", "92233720368547758.08", "999999999999999999999"} {
		if _, err := ParseAmount(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}

func TestResponseVerification(t *testing.T) {
	c := testClient(t)
	fields := map[string]any{"code": 0, "future": json.Number("123456789012345678901"), "zero": 0, "trade_no": "platform-1"}
	data := testEnvelope(t, fields)
	response, err := c.parseResponse(data)
	if err != nil {
		t.Fatal(err)
	}
	copyOfJSON := response.JSON()
	copyOfJSON[0] = '!'
	if !json.Valid(response.JSON()) {
		t.Fatal("JSON accessor exposed internal bytes")
	}
	fields["zero"] = 1
	tampered, _ := json.Marshal(fields)
	if _, err := c.parseResponse(tampered); err == nil {
		t.Fatal("tampered response accepted")
	}
	for name, data := range map[string]string{
		"unsigned success": `{"code":0}`, "missing code": `{}`, "null code": `{"code":null}`,
		"duplicate": `{"code":0,"code":1}`, "trailing": `{"code":0}{}`, "array": `[]`,
		"empty signature": `{"code":1,"sign":""}`, "decimal": `{"code":0,"money":1.23}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := c.parseResponse([]byte(data)); err == nil {
				t.Fatal("invalid response accepted")
			}
		})
	}
	for _, signed := range []bool{false, true} {
		data := []byte(`{"code":42,"msg":"declined"}`)
		if signed {
			data = testEnvelope(t, map[string]any{"code": 42, "msg": "declined"})
		}
		_, err := c.parseResponse(data)
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Verified != signed || apiErr.Code != 42 {
			t.Fatalf("unexpected API error: %v", err)
		}
	}
}

func TestNestedFieldsAreExplicitlyUnsigned(t *testing.T) {
	c := testClient(t)
	fields := map[string]any{"data": map[string]any{"money": "1.00"}}
	_ = testEnvelope(t, fields)
	fields["data"] = map[string]any{"money": "2.00"}
	data, _ := json.Marshal(fields)
	response, err := c.parseResponse(data)
	if err != nil || !reflect.DeepEqual(response.UnsignedFields, []string{"data"}) {
		t.Fatalf("unsigned data boundary: %v", err)
	}
}
