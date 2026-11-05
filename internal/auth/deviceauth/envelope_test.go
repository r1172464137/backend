package deviceauth

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestEnvelopeAuthenticatesOperationAndTimestamp(t *testing.T) {
	const secret = "test-key"
	data, timestamp, err := Encrypt([]byte(`{"identifier":"device"}`), secret)
	if err != nil {
		t.Fatal(err)
	}
	e := Envelope{Data: data, Time: timestamp, Sign: Sign(secret, "POST", "/login", "body", data, timestamp)}
	nanos, _ := strconv.ParseInt(timestamp, 16, 64)
	now := time.Unix(0, nanos)
	if plain, err := e.Open(secret, "POST", "/login", "body", now); err != nil || plain != `{"identifier":"device"}` {
		t.Fatalf("valid envelope: %q, %v", plain, err)
	}
	for _, tc := range []struct {
		name, key, method, path, scope string
		now                            time.Time
	}{
		{"wrong key", "wrong", "POST", "/login", "body", now},
		{"wrong method", secret, "GET", "/login", "body", now},
		{"wrong path", secret, "POST", "/register", "body", now},
		{"wrong scope", secret, "POST", "/login", "query", now},
		{"expired", secret, "POST", "/login", "body", now.Add(MaxAge + time.Nanosecond)},
		{"future", secret, "POST", "/login", "body", now.Add(-FutureSkew - time.Nanosecond)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := e.Open(tc.key, tc.method, tc.path, tc.scope, tc.now); err == nil {
				t.Fatal("accepted invalid envelope")
			}
		})
	}
	for _, bad := range []Envelope{
		{Data: data, Time: timestamp},
		{Data: data, Time: timestamp, Sign: strings.Repeat("x", 64)},
		{Data: data + "tampered", Time: timestamp, Sign: e.Sign},
		{Data: data, Time: "0" + timestamp, Sign: e.Sign},
		{Data: data, Time: "invalid", Sign: e.Sign},
	} {
		if _, err := bad.Open(secret, "POST", "/login", "body", now); err == nil {
			t.Fatal("accepted tampered envelope")
		}
	}
}

func TestReplayReservationIsAtomicAndFailsClosed(t *testing.T) {
	rdb := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: rdb.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	e := Envelope{Time: strconv.FormatInt(time.Now().UnixNano(), 16)}
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e.Consume(context.Background(), client, "key") == nil {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatalf("accepted %d identical requests", accepted.Load())
	}
	keys := rdb.Keys()
	if len(keys) != 1 || rdb.TTL(keys[0]) != MaxAge+FutureSkew+time.Second {
		t.Fatalf("invalid replay TTL: %v", keys)
	}
	if err := e.Consume(context.Background(), nil, "key"); err == nil {
		t.Fatal("nil replay store accepted")
	}
	rdb.SetError("unavailable")
	if err := (Envelope{Time: "other"}).Consume(context.Background(), client, "key"); err == nil {
		t.Fatal("Redis failure accepted")
	}
}
