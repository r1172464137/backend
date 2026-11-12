package app

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/perfect-panel/server/pkg/orm"
)

var fastRetry = RetryPolicy{Total: 2 * time.Second, Attempt: 300 * time.Millisecond, InitialBackoff: 20 * time.Millisecond, MaxBackoff: 80 * time.Millisecond}

func TestConnectWithRetrySucceedsAfterTransientFailures(t *testing.T) {
	var calls atomic.Int32
	v, err := connectWithRetry(context.Background(), "dep", fastRetry, func(context.Context) (int, error) {
		if calls.Add(1) < 3 {
			return 0, errors.New("connection refused")
		}
		return 7, nil
	}, nil)
	if err != nil || v != 7 || calls.Load() != 3 {
		t.Fatalf("v=%d err=%v calls=%d, want 7/nil/3", v, err, calls.Load())
	}
}

func TestConnectWithRetryStopsOnPermanentError(t *testing.T) {
	var calls atomic.Int32
	_, err := connectWithRetry(context.Background(), "database", fastRetry, func(context.Context) (int, error) {
		calls.Add(1)
		return 0, &mysqldriver.MySQLError{Number: 1045, Message: "Access denied"}
	}, permanentDatabaseError)
	var pe *PermanentError
	if !errors.As(err, &pe) || calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d, want one attempt and a PermanentError", err, calls.Load())
	}
	if !strings.Contains(err.Error(), "database") || !strings.Contains(err.Error(), "Access denied") {
		t.Fatalf("error %q does not name the dependency and cause", err)
	}
}

func TestConnectWithRetryReportsBudgetExhaustion(t *testing.T) {
	p := RetryPolicy{Total: 250 * time.Millisecond, Attempt: 50 * time.Millisecond, InitialBackoff: 20 * time.Millisecond, MaxBackoff: 40 * time.Millisecond}
	start := time.Now()
	_, err := connectWithRetry(context.Background(), "redis", p, func(context.Context) (int, error) {
		return 0, errors.New("dial tcp: connection refused")
	}, permanentRedisError)
	if err == nil || !strings.Contains(err.Error(), "not ready after") || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("err=%v, want budget exhaustion naming the last cause", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("took %s, budget was %s", elapsed, p.Total)
	}
}

// Each attempt receives its own deadline, so an attempt that blocks (a server
// that never completes the handshake) is cut off and retried.
func TestConnectWithRetryBoundsEachAttempt(t *testing.T) {
	var calls atomic.Int32
	start := time.Now()
	_, err := connectWithRetry(context.Background(), "database",
		RetryPolicy{Total: 700 * time.Millisecond, Attempt: 150 * time.Millisecond, InitialBackoff: 10 * time.Millisecond, MaxBackoff: 10 * time.Millisecond},
		func(ctx context.Context) (int, error) {
			calls.Add(1)
			<-ctx.Done()
			return 0, ctx.Err()
		}, permanentDatabaseError)
	if err == nil || calls.Load() < 3 || time.Since(start) > 1500*time.Millisecond {
		t.Fatalf("err=%v calls=%d elapsed=%s, want several bounded attempts", err, calls.Load(), time.Since(start))
	}
}

func TestConnectWithRetryCancelledDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := RetryPolicy{Total: time.Minute, Attempt: time.Second, InitialBackoff: 10 * time.Second, MaxBackoff: 10 * time.Second}
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
	start := time.Now()
	_, err := connectWithRetry(ctx, "database", p, func(context.Context) (int, error) {
		return 0, errors.New("connection refused")
	}, nil)
	if !errors.Is(err, context.Canceled) || time.Since(start) > 2*time.Second {
		t.Fatalf("err=%v elapsed=%s, want prompt cancellation", err, time.Since(start))
	}
}

func TestPermanentErrorClassification(t *testing.T) {
	cases := []struct {
		err   error
		db    bool
		redis bool
	}{
		{&mysqldriver.MySQLError{Number: 1045}, true, false},
		{&mysqldriver.MySQLError{Number: 1049}, true, false},
		{&mysqldriver.MySQLError{Number: 1040}, false, false}, // too many connections: transient
		{errors.New("dial tcp 127.0.0.1:3306: connect: connection refused"), false, false},
		{context.DeadlineExceeded, false, false},
		{errors.New("WRONGPASS invalid username-password pair"), false, true},
		{errors.New("NOAUTH Authentication required."), false, true},
		{errors.New("LOADING Redis is loading the dataset in memory"), false, false},
	}
	for _, c := range cases {
		if got := permanentDatabaseError(c.err); got != c.db {
			t.Errorf("permanentDatabaseError(%v)=%v want %v", c.err, got, c.db)
		}
		if got := permanentRedisError(c.err); got != c.redis {
			t.Errorf("permanentRedisError(%v)=%v want %v", c.err, got, c.redis)
		}
	}
}

// A server that accepts TCP but never sends the MySQL greeting must not hold
// OpenDatabase past the caller's deadline.
func TestOpenDatabaseBoundedWhenHandshakeHangs(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	var mu sync.Mutex
	var held []net.Conn
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			held = append(held, c) // accept, then say nothing
			mu.Unlock()
		}
	}()
	defer func() {
		mu.Lock()
		defer mu.Unlock()
		for _, c := range held {
			c.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	db, err := orm.OpenDatabase(ctx, orm.Mysql{Config: orm.Config{Driver: "mysql", Addr: ln.Addr().String(), Username: "u", Password: "p", Dbname: "ppanel", Config: orm.DefaultMySQLConfig, MaxIdleConns: 1, MaxOpenConns: 1}})
	if db != nil || err == nil {
		t.Fatalf("db=%v err=%v, want failure", db, err)
	}
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Fatalf("OpenDatabase returned after %s, deadline was 300ms", elapsed)
	}
}
