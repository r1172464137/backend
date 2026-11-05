package protocol

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Params contains scalar protocol values before URL encoding. Money stays a string.
type Params map[string]string

var parameterName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// CanonicalString follows the reference PHP SDK: retain "0", omit whitespace-only
// values, and preserve every byte of nonempty values. Do not URL-encode here.
func CanonicalString(params Params) (string, error) {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if !parameterName.MatchString(key) {
			return "", fmt.Errorf("epay: invalid scalar parameter name %q", key)
		}
		if key == "sign" || key == "sign_type" || strings.Trim(value, " \t\n\r\x00\x0b") == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+params[key])
	}
	return strings.Join(parts, "&"), nil
}

func signParams(key *rsa.PrivateKey, params Params) (string, error) {
	canonical, err := CanonicalString(params)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(canonical))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("epay: sign: %w", err)
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func verifyParams(key *rsa.PublicKey, params Params) error {
	if params["sign_type"] != "RSA" {
		return errors.New("epay: missing or unsupported sign_type")
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(params["sign"])
	if err != nil || len(signature) == 0 {
		return errors.New("epay: missing or invalid base64 signature")
	}
	canonical, err := CanonicalString(params)
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(canonical))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return errors.New("epay: signature verification failed")
	}
	return nil
}

func keyDER(input []byte) ([]byte, error) {
	text := strings.TrimSpace(string(input))
	if strings.HasPrefix(text, "-----BEGIN ") {
		block, rest := pem.Decode([]byte(text))
		if block == nil || len(strings.TrimSpace(string(rest))) != 0 || len(block.Headers) != 0 {
			return nil, errors.New("epay: expected a single unencrypted PEM key")
		}
		return block.Bytes, nil
	}
	// The merchant console and PHP SDK also use bare Base64 DER keys.
	return base64.StdEncoding.DecodeString(strings.Join(strings.Fields(text), ""))
}

func parsePrivateKey(input []byte) (*rsa.PrivateKey, error) {
	der, err := keyDER(input)
	if err != nil {
		return nil, fmt.Errorf("epay: merchant private key: %w", err)
	}
	var key *rsa.PrivateKey
	if candidate, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		key, _ = candidate.(*rsa.PrivateKey)
	} else {
		key, _ = x509.ParsePKCS1PrivateKey(der)
	}
	if key == nil || key.N.BitLen() < 2048 {
		return nil, errors.New("epay: expected an RSA private key of at least 2048 bits (PKCS#1 or PKCS#8)")
	}
	if err := key.Validate(); err != nil {
		return nil, fmt.Errorf("epay: invalid private key: %w", err)
	}
	return key, nil
}

func parsePublicKey(input []byte) (*rsa.PublicKey, error) {
	der, err := keyDER(input)
	if err != nil {
		return nil, fmt.Errorf("epay: platform public key: %w", err)
	}
	var key *rsa.PublicKey
	if candidate, err := x509.ParsePKIXPublicKey(der); err == nil {
		key, _ = candidate.(*rsa.PublicKey)
	} else {
		key, _ = x509.ParsePKCS1PublicKey(der)
	}
	if key == nil || key.N.BitLen() < 2048 || key.E < 3 || key.E%2 == 0 {
		return nil, errors.New("epay: expected an RSA public key of at least 2048 bits (PKIX or PKCS#1)")
	}
	return key, nil
}
