package app

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/pkg/logger/logtest"
	"github.com/perfect-panel/server/pkg/orm"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// testStartupRetry is a short policy for tests: the production defaults would
// make each test wait 30 s.
func testStartupRetry(budget time.Duration) startupRetry {
	return startupRetry{
		budget:         budget,
		attemptTimeout: 100 * time.Millisecond,
		initialDelay:   10 * time.Millisecond,
		maxDelay:       40 * time.Millisecond,
	}
}

// unusedTCPAddress reserves a loopback port and closes it, so dialing the
// address is refused until the test listens on it.
func unusedTCPAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

// delayedTCPListener is a loopback address that refuses connections until
// the first failed dial, then starts accepting. It simulates a container
// restarting: the first start attempt is refused and a later one succeeds.
type delayedTCPListener struct {
	addr        string
	refused     chan struct{}
	refusedOnce sync.Once
	started     chan net.Listener
}

func newDelayedTCPListener(t *testing.T) *delayedTCPListener {
	t.Helper()
	l := &delayedTCPListener{
		addr:    unusedTCPAddress(t),
		refused: make(chan struct{}),
		started: make(chan net.Listener, 1),
	}
	go func() {
		<-l.refused
		listener, err := net.Listen("tcp", l.addr)
		if err != nil {
			l.started <- nil
			return
		}
		l.started <- listener
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() {
		select {
		case listener := <-l.started:
			if listener != nil {
				_ = listener.Close()
			}
		case <-time.After(2 * time.Second):
		}
	})
	return l
}

func (l *delayedTCPListener) Dial(ctx context.Context) (net.Conn, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", l.addr)
	if err != nil {
		l.refusedOnce.Do(func() { close(l.refused) })
	}
	return conn, err
}

// The start must wait for a database that is refused at first and then
// comes back, rather than crash-looping the process.
func TestConnectAtStartupRetriesUntilTheDatabaseAccepts(t *testing.T) {
	logs := logtest.NewCollector(t)
	listener := newDelayedTCPListener(t)
	var attempts atomic.Int32

	connected, err := connectAtStartup(context.Background(), "database", listener.addr, testStartupRetry(2*time.Second),
		func(ctx context.Context) (string, error) {
			attempts.Add(1)
			conn, err := listener.Dial(ctx)
			if err != nil {
				return "", err
			}
			_ = conn.Close()
			return "connected", nil
		})

	if err != nil {
		t.Fatalf("connectAtStartup returned %v, want success once the listener accepts", err)
	}
	if connected != "connected" {
		t.Fatalf("connectAtStartup returned %q, want the successful attempt's value", connected)
	}
	if attempts.Load() < 2 {
		t.Fatalf("the connection took %d attempt(s), want a refusal followed by a successful retry", attempts.Load())
	}
	if !strings.Contains(logs.String(), "retrying") {
		t.Fatalf("the retry was not logged:\n%s", logs.String())
	}
}

// The retry budget bounds the wait: a database that never comes up returns
// an error naming the dependency and its last failure, not a silent start.
func TestConnectAtStartupGivesUpWhenTheBudgetIsSpent(t *testing.T) {
	logs := logtest.NewCollector(t)
	addr := unusedTCPAddress(t)
	var attempts atomic.Int32
	start := time.Now()

	_, err := connectAtStartup(context.Background(), "database", addr, testStartupRetry(150*time.Millisecond),
		func(ctx context.Context) (struct{}, error) {
			attempts.Add(1)
			var dialer net.Dialer
			conn, dialErr := dialer.DialContext(ctx, "tcp", addr)
			if dialErr == nil {
				_ = conn.Close()
			}
			return struct{}{}, dialErr
		})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("connectAtStartup returned nil although the address never accepted")
	}
	if attempts.Load() < 2 {
		t.Fatalf("the connection took %d attempt(s), want the retries the budget allows", attempts.Load())
	}
	if elapsed > time.Second {
		t.Fatalf("the failed start took %s, want the short test budget", elapsed)
	}
	if !strings.Contains(err.Error(), "database startup connection failed after") {
		t.Fatalf("error = %v, want the dependency and the attempts named", err)
	}
	if !strings.Contains(err.Error(), "within 150ms") {
		t.Fatalf("error = %v, want the spent budget named", err)
	}
	if !strings.Contains(logs.String(), "giving up") {
		t.Fatalf("the final failure was not logged:\n%s", logs.String())
	}
}

// A signal during start-up cancels the retries instead of holding the
// process until the budget runs out.
func TestConnectAtStartupStopsWhenTheContextIsCanceled(t *testing.T) {
	logtest.Discard(t)
	addr := unusedTCPAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var attempts atomic.Int32
	start := time.Now()

	_, err := connectAtStartup(ctx, "redis", addr, testStartupRetry(5*time.Second),
		func(ctx context.Context) (struct{}, error) {
			attempts.Add(1)
			var dialer net.Dialer
			conn, dialErr := dialer.DialContext(ctx, "tcp", addr)
			if dialErr == nil {
				_ = conn.Close()
			} else {
				// The signal arrives while the attempt is in flight.
				cancel()
			}
			return struct{}{}, dialErr
		})
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("connectAtStartup error = %v, want context.Canceled", err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("the connection took %d attempt(s) after cancellation, want the attempt already in flight", attempts.Load())
	}
	if elapsed > time.Second {
		t.Fatalf("the canceled start took %s, want an immediate return", elapsed)
	}
}

// Redis is retried the same way: its container may restart together with
// MySQL, and a ping against the closed port must not fail the start.
func TestConnectAtStartupRetriesUntilRedisAccepts(t *testing.T) {
	logtest.Discard(t)
	addr := unusedTCPAddress(t)
	mini := miniredis.NewMiniRedis()
	t.Cleanup(mini.Close)

	firstFailure := make(chan struct{})
	started := make(chan error, 1)
	var refusedOnce sync.Once
	go func() {
		<-firstFailure
		started <- mini.StartAddr(addr)
	}()

	var attempts atomic.Int32
	rds, err := connectAtStartup(context.Background(), "redis", addr, testStartupRetry(2*time.Second),
		func(ctx context.Context) (*redis.Client, error) {
			attempts.Add(1)
			client, err := connectRedis(ctx, config.RedisConfig{Host: addr})
			if err != nil {
				refusedOnce.Do(func() { close(firstFailure) })
			}
			return client, err
		})

	if err != nil {
		t.Fatalf("connectAtStartup returned %v, want success once Redis starts", err)
	}
	t.Cleanup(func() { _ = rds.Close() })
	select {
	case startErr := <-started:
		if startErr != nil {
			t.Fatalf("start the test Redis at %s: %v", addr, startErr)
		}
	case <-time.After(time.Second):
		t.Fatal("the test Redis did not start")
	}
	if attempts.Load() < 2 {
		t.Fatalf("the Redis connection took %d attempt(s), want a refusal followed by success", attempts.Load())
	}
	if err := rds.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("the returned Redis client does not answer: %v", err)
	}
}

// newApplication is the composition root's start: the retries must complete
// before it assembles the modules, and the application it returns must carry
// the connections the later attempts made.
func TestNewApplicationWaitsForARestartingDatabase(t *testing.T) {
	logs := logtest.NewCollector(t)
	listener := newDelayedTCPListener(t)
	mini := miniredis.RunT(t)

	var c config.Config
	c.AppLocation = orm.DefaultLocation
	c.SetDatabaseConfig(orm.Config{
		Driver: orm.DriverMySQL,
		Addr:   listener.addr,
		Dbname: "ppanel",
		Config: orm.DefaultMySQLQuery(orm.DefaultLocation),
	})
	c.Redis.Host = mini.Addr()
	c.GeoIP.Path = filepath.Join(t.TempDir(), "missing.mmdb")
	c.GeoIP.Download = false

	var attempts atomic.Int32
	openDatabase := func(ctx context.Context, _ orm.Config) (*gorm.DB, error) {
		attempts.Add(1)
		conn, err := listener.Dial(ctx)
		if err != nil {
			return nil, err
		}
		_ = conn.Close()
		// The handshake is replaced with an in-memory GORM connection: this
		// test is about the startup retry, not the MySQL protocol.
		return gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	}

	srv, err := newApplication(context.Background(), c, testStartupRetry(2*time.Second), openDatabase, connectRedis)
	if err != nil {
		t.Fatalf("newApplication returned %v, want the start to survive the first refusal", err)
	}
	if srv == nil || srv.DB == nil || srv.Redis == nil {
		t.Fatalf("newApplication returned %+v, want the assembled application", srv)
	}
	if attempts.Load() < 2 {
		t.Fatalf("the database connection took %d attempt(s), want a refusal followed by success", attempts.Load())
	}
	if !strings.Contains(logs.String(), "retrying") {
		t.Fatalf("the startup retry was not logged:\n%s", logs.String())
	}
	t.Cleanup(func() {
		closeDatabase(srv.DB)
		_ = srv.Redis.Close()
		_ = srv.Queue.Close()
		_ = srv.Inspector.Close()
	})
}

// The database pool a successful first connection opened is closed when the
// Redis connection then exhausts its budget: the process cannot continue, and
// must not leave the pool behind while it exits.
func TestNewApplicationClosesTheDatabaseWhenRedisDoesNotComeUp(t *testing.T) {
	logtest.Discard(t)
	redisAddr := unusedTCPAddress(t)

	var c config.Config
	c.AppLocation = orm.DefaultLocation
	c.SetDatabaseConfig(orm.Config{
		Driver: orm.DriverMySQL,
		Addr:   "127.0.0.1:3306",
		Dbname: "ppanel",
		Config: orm.DefaultMySQLQuery(orm.DefaultLocation),
	})
	c.Redis.Host = redisAddr
	c.GeoIP.Path = filepath.Join(t.TempDir(), "missing.mmdb")
	c.GeoIP.Download = false

	var db *gorm.DB
	openDatabase := func(context.Context, orm.Config) (*gorm.DB, error) {
		var err error
		db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		return db, err
	}

	srv, err := newApplication(context.Background(), c, testStartupRetry(200*time.Millisecond), openDatabase, connectRedis)
	if err == nil || srv != nil {
		t.Fatalf("newApplication = (%+v, %v), want a failed start because Redis never answered", srv, err)
	}
	if !strings.Contains(err.Error(), "redis startup connection failed after") {
		t.Fatalf("error = %v, want Redis and its attempts named", err)
	}
	if db == nil {
		t.Fatal("the database was never opened")
	}
	sqlDB, dbErr := db.DB()
	if dbErr != nil {
		t.Fatalf("database pool: %v", dbErr)
	}
	if pingErr := sqlDB.Ping(); pingErr == nil {
		t.Fatal("the database pool is still open after the failed start")
	}
}
