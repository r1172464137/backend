// Package deviceauth authenticates the optional device transport envelope.
// A shared transport key does not attest a physical device or an official app.
package deviceauth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	MaxAge     = 5 * time.Minute
	FutureSkew = 30 * time.Second
)

var ErrInvalidEnvelope = errors.New("invalid device envelope")

type Envelope struct {
	Data string `json:"data"`
	Time string `json:"time"`
	Sign string `json:"sign"`
}

type ReplayStore interface {
	SetNX(context.Context, string, any, time.Duration) *redis.BoolCmd
}

// Sign binds the ciphertext to its HTTP operation and envelope location.
// scope is one of "query", "body", or "response".
func Sign(secret, method, path, scope, data, timestamp string) string {
	key := sha256.Sum256([]byte("ppanel:device-envelope:v1:" + secret))
	mac := hmac.New(sha256.New, key[:])
	mac.Write([]byte(strings.Join([]string{"v1", method, path, scope, timestamp, data}, "\n")))
	return hex.EncodeToString(mac.Sum(nil))
}

func (e Envelope) Open(secret, method, path, scope string, now time.Time) (string, error) {
	if secret == "" || e.Data == "" || len(e.Sign) != sha256.Size*2 {
		return "", ErrInvalidEnvelope
	}
	nanos, err := strconv.ParseInt(e.Time, 16, 64)
	if err != nil || nanos <= 0 || strconv.FormatInt(nanos, 16) != e.Time {
		return "", ErrInvalidEnvelope
	}
	at := time.Unix(0, nanos)
	if at.Before(now.Add(-MaxAge)) || at.After(now.Add(FutureSkew)) {
		return "", ErrInvalidEnvelope
	}
	signature, err := hex.DecodeString(e.Sign)
	if err != nil {
		return "", ErrInvalidEnvelope
	}
	expected, _ := hex.DecodeString(Sign(secret, method, path, scope, e.Data, e.Time))
	if !hmac.Equal(signature, expected) {
		return "", ErrInvalidEnvelope
	}
	plain, err := Decrypt(e.Data, secret, e.Time)
	if err != nil {
		return "", ErrInvalidEnvelope
	}
	return plain, nil
}

// Consume reserves the nonce across all server instances. Redis errors fail
// closed; the TTL spans the entire acceptance window including clock skew.
func (e Envelope) Consume(ctx context.Context, store ReplayStore, secret string) error {
	if store == nil {
		return errors.New("device replay store is unavailable")
	}
	namespace := sha256.Sum256([]byte(secret))
	key := "auth:device_nonce:" + hex.EncodeToString(namespace[:]) + ":" + e.Time
	// A small margin also covers the inclusive timestamp boundary and Redis
	// TTL precision; a still-acceptable envelope must not outlive its nonce.
	ok, err := store.SetNX(ctx, key, "1", MaxAge+FutureSkew+time.Second).Result()
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidEnvelope
	}
	return nil
}
