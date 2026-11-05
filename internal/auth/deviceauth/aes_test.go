package deviceauth

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAes(t *testing.T) {
	params := map[string]any{
		"method":   "email",
		"account":  "admin@ppanel.dev",
		"password": "password",
	}
	marshal, _ := json.Marshal(params)
	jsonStr := string(marshal)
	encrypt, iv, err := Encrypt([]byte(jsonStr), "123456")
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	decrypt, err := Decrypt(encrypt, "123456", iv)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	assert.Equal(t, jsonStr, decrypt, "decrypt failed")

}

// The device clients derive the same key, IV and padding, so the ciphertext
// of a given key, nonce and plaintext is fixed. These vectors were produced
// by the AES-256-CBC/PKCS#7 implementation the transport shipped with
// (github.com/forgoer/openssl); the standard-library implementation must
// reproduce them byte for byte, or every deployed client breaks.
func TestEncryptMatchesTheKnownAnswers(t *testing.T) {
	for _, tc := range []struct {
		name, key, nonce, plain, cipher string
	}{
		{
			name:   "login body",
			key:    "123456",
			nonce:  "18b1c2d3e4f5a6b7",
			plain:  `{"method":"email","account":"admin@ppanel.dev","password":"password"}`,
			cipher: "TrBFefup68CrNdeIDRdfMQTmQmeozx5j6Yfcy9i9fov6Hwq5d00yM/Cd41QeARYffm5ZR0LzQNWLlHu6h6qHnOQYZg4qtZb3u5VRIhzuxow=",
		},
		{name: "short query", key: "device-secret", nonce: "17f3a9c0", plain: `{"page":2}`, cipher: "5Bg5Mk8vBrzm3rWL/uG8hg=="},
		{name: "empty plaintext pads a whole block", key: "k", nonce: "0", plain: "", cipher: "AYklRxDfQMULuP8l37rPQA=="},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := encryptWithNonce([]byte(tc.plain), tc.key, tc.nonce)
			if err != nil || got != tc.cipher {
				t.Fatalf("encryptWithNonce() = %q, %v; want %q", got, err, tc.cipher)
			}
			plain, err := Decrypt(tc.cipher, tc.key, tc.nonce)
			if err != nil || plain != tc.plain {
				t.Fatalf("Decrypt() = %q, %v; want %q", plain, err, tc.plain)
			}
		})
	}
}

// A wrong key, a wrong nonce, a truncated or tampered ciphertext and a
// ciphertext that is not base64 are all refused without revealing which.
func TestDecryptRefusesTamperedInput(t *testing.T) {
	const key, nonce = "123456", "18b1c2d3e4f5a6b7"
	valid, err := encryptWithNonce([]byte(`{"ok":true}`), key, nonce)
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct{ cipher, key, nonce string }{
		"wrong key":        {valid, "654321", nonce},
		"wrong nonce":      {valid, key, "0"},
		"not base64":       {"%%%", key, nonce},
		"partial block":    {valid[:8], key, nonce},
		"empty":            {"", key, nonce},
		"tampered padding": {strings.Repeat("A", 24), key, nonce},
	} {
		t.Run(name, func(t *testing.T) {
			if plain, err := Decrypt(tc.cipher, tc.key, tc.nonce); err == nil {
				t.Fatalf("Decrypt() = %q, want an error", plain)
			}
		})
	}
}
