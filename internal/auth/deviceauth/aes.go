package deviceauth

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5" //nolint:gosec // G501: the device clients derive the CBC IV with MD5; the envelope format must not change
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// errPadding reports a ciphertext whose length or PKCS#7 padding is not
// that of a CBC message; the message is not described further, so a wrong
// key and a tampered message read the same.
var errPadding = errors.New("invalid ciphertext")

// Encrypt encrypts plainText for the device transport with AES-256-CBC and
// PKCS#7 padding. It returns the base64 ciphertext and the nonce, the current
// time in hexadecimal nanoseconds. The IV is derived from the nonce and the
// key instead of being sent, so the receiver needs the nonce to decrypt; the
// envelope carries it as its time.
func Encrypt(plainText []byte, keyStr string) (string, string, error) {
	nonce := fmt.Sprintf("%x", time.Now().UnixNano())
	ciphertext, err := encryptWithNonce(plainText, keyStr, nonce)
	return ciphertext, nonce, err
}

// encryptWithNonce is Encrypt with the nonce given, so the output is
// reproducible; the device clients derive the same bytes.
func encryptWithNonce(plainText []byte, keyStr, nonce string) (string, error) {
	block, err := aes.NewCipher(generateKey(keyStr))
	if err != nil {
		return "", err
	}
	padded := pkcs7Pad(plainText, block.BlockSize())
	dst := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, generateIv(nonce, keyStr)[:block.BlockSize()]).CryptBlocks(dst, padded)
	return base64.StdEncoding.EncodeToString(dst), nil
}

// Decrypt reverses Encrypt: cipherText is the base64 ciphertext and ivStr the
// nonce Encrypt returned with it.
func Decrypt(cipherText string, keyStr string, ivStr string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(cipherText)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(generateKey(keyStr))
	if err != nil {
		return "", err
	}
	if len(decoded) == 0 || len(decoded)%block.BlockSize() != 0 {
		return "", errPadding
	}
	dst := make([]byte, len(decoded))
	cipher.NewCBCDecrypter(block, generateIv(ivStr, keyStr)[:block.BlockSize()]).CryptBlocks(dst, decoded)
	plain, err := pkcs7Unpad(dst, block.BlockSize())
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// pkcs7Pad appends the PKCS#7 padding that brings data to a whole number of
// blocks; data that already is gets a full block of padding.
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	return append(append(make([]byte, 0, len(data)+padding), data...), bytes.Repeat([]byte{byte(padding)}, padding)...)
}

// pkcs7Unpad strips PKCS#7 padding, refusing padding that is not well formed.
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, errPadding
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize || padding > len(data) {
		return nil, errPadding
	}
	for _, b := range data[len(data)-padding:] {
		if int(b) != padding {
			return nil, errPadding
		}
	}
	return data[:len(data)-padding], nil
}

// generateKey hashes key with SHA-256, so a secret of any length yields the
// 32-byte key AES-256 needs.
func generateKey(key string) []byte {
	hash := sha256.Sum256([]byte(key))
	return hash[:32]
}

// generateIv derives the IV from the nonce iv and the key: the SHA-256 of
// hex(MD5(iv)) followed by the key, of which CBC uses the first 16 bytes. The
// device clients derive it the same way, so it must not change.
func generateIv(iv, key string) []byte {
	h := md5.New() //nolint:gosec // G401: see the import note
	h.Write([]byte(iv))
	return generateKey(hex.EncodeToString(h.Sum(nil)) + key)
}
