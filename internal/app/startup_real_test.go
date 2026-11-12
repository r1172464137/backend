package app

// Real-dependency startup tests. They run only when the binaries are provided:
//
//	PPANEL_TEST_MYSQL_BASE=/path/with/usr  (usr/sbin/mysqld, usr/bin/mysql, usr/share/mysql)
//	PPANEL_TEST_REDIS_BIN=/path/to/redis-server
//
// The rebuild environment uses the production-image MySQL 8.4.5 / Redis 7.4.5.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/perfect-panel/server/pkg/orm"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

type realMySQL struct {
	base, dir, sock string
	port            int
	cmd             *exec.Cmd
	done            chan struct{}
}

// newRealMySQL initialises a data directory and provisions database "ppanel"
// with a default-plugin (caching_sha2_password) account pp@127.0.0.1, then
// leaves the server stopped.
func newRealMySQL(t *testing.T) *realMySQL {
	t.Helper()
	base := os.Getenv("PPANEL_TEST_MYSQL_BASE")
	if base == "" {
		t.Skip("PPANEL_TEST_MYSQL_BASE not set")
	}
	dir := t.TempDir()
	m := &realMySQL{base: base, dir: dir, sock: filepath.Join(dir, "mysql.sock"), port: freePort(t)}
	if out, err := exec.Command(filepath.Join(base, "sbin/mysqld"), m.common("--initialize-insecure")...).CombinedOutput(); err != nil {
		t.Fatalf("mysqld initialize: %v\n%s", err, out)
	}
	t.Cleanup(m.stop)
	m.start(t)
	m.sql(t, "CREATE DATABASE ppanel; CREATE USER 'pp'@'127.0.0.1' IDENTIFIED BY 'secret-pass'; GRANT ALL ON ppanel.* TO 'pp'@'127.0.0.1';")
	m.stop()
	return m
}

func (m *realMySQL) common(extra ...string) []string {
	args := []string{"--no-defaults", "--basedir=" + m.base, "--datadir=" + filepath.Join(m.dir, "data"),
		"--lc-messages-dir=" + filepath.Join(m.base, "share/mysql"), "--innodb-buffer-pool-size=32M",
		"--innodb-redo-log-capacity=16M", "--performance-schema=OFF", "--mysqlx=OFF", "--skip-log-bin"}
	if os.Geteuid() == 0 {
		args = append(args, "--user=root")
	}
	return append(args, extra...)
}

func (m *realMySQL) start(t *testing.T) {
	m.cmd = exec.Command(filepath.Join(m.base, "sbin/mysqld"), m.common("--bind-address=127.0.0.1",
		fmt.Sprintf("--port=%d", m.port), "--socket="+m.sock, "--pid-file="+filepath.Join(m.dir, "mysql.pid"),
		"--tmpdir="+m.dir, "--secure-file-priv="+m.dir)...)
	if err := m.cmd.Start(); err != nil {
		t.Error(err)
		return
	}
	m.done = make(chan struct{})
	go func(c *exec.Cmd, done chan struct{}) { _ = c.Wait(); close(done) }(m.cmd, m.done)
}

func (m *realMySQL) stop() {
	if m.cmd == nil {
		return
	}
	_ = m.cmd.Process.Signal(os.Interrupt)
	select {
	case <-m.done:
	case <-time.After(30 * time.Second):
		_ = m.cmd.Process.Kill()
		<-m.done
	}
	m.cmd = nil
}

func (m *realMySQL) sql(t *testing.T, stmt string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		out, err := exec.Command(filepath.Join(m.base, "bin/mysql"), "--no-defaults", "-uroot", "--socket="+m.sock, "-e", stmt).CombinedOutput()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("mysql %q: %v\n%s", stmt, err, out)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func (m *realMySQL) config(user, pass, db string) orm.Mysql {
	return orm.Mysql{Config: orm.Config{Driver: "mysql", Addr: fmt.Sprintf("127.0.0.1:%d", m.port), Username: user,
		Password: pass, Dbname: db, Config: orm.DefaultMySQLConfig, MaxIdleConns: 2, MaxOpenConns: 4}}
}

func openDB(cfg orm.Mysql) func(context.Context) (*gorm.DB, error) {
	return func(ctx context.Context) (*gorm.DB, error) { return orm.OpenDatabase(ctx, cfg) }
}

var realPolicy = RetryPolicy{Total: 90 * time.Second, Attempt: 5 * time.Second, InitialBackoff: 200 * time.Millisecond, MaxBackoff: time.Second}

func TestRealMySQLStartup(t *testing.T) {
	m := newRealMySQL(t)

	t.Run("late start is retried", func(t *testing.T) {
		go func() { time.Sleep(2 * time.Second); m.start(t) }()
		start := time.Now()
		db, err := connectWithRetry(context.Background(), "database", realPolicy, openDB(m.config("pp", "secret-pass", "ppanel")), permanentDatabaseError)
		if err != nil {
			t.Fatalf("connect: %v", err)
		}
		if time.Since(start) < 2*time.Second {
			t.Fatalf("connected before the server was started")
		}
		var one int
		if err := db.Raw("SELECT 1").Scan(&one).Error; err != nil || one != 1 {
			t.Fatalf("query after connect: %v (%d)", err, one)
		}
		var version string
		if err := db.Raw("SELECT VERSION()").Scan(&version).Error; err != nil {
			t.Fatal(err)
		}
		t.Logf("connected to MySQL %s with caching_sha2_password over TCP", version)
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})

	t.Run("wrong password fails fast", func(t *testing.T) {
		start := time.Now()
		_, err := connectWithRetry(context.Background(), "database", realPolicy, openDB(m.config("pp", "wrong", "ppanel")), permanentDatabaseError)
		var pe *PermanentError
		if !errors.As(err, &pe) || !strings.Contains(err.Error(), "1045") {
			t.Fatalf("err=%v, want permanent 1045", err)
		}
		if time.Since(start) > 10*time.Second {
			t.Fatalf("took %s; a permanent error must not be retried", time.Since(start))
		}
	})

	t.Run("unknown database fails fast", func(t *testing.T) {
		_, err := connectWithRetry(context.Background(), "database", realPolicy, openDB(m.config("pp", "secret-pass", "missing_db")), permanentDatabaseError)
		var pe *PermanentError
		if !errors.As(err, &pe) || !(strings.Contains(err.Error(), "1049") || strings.Contains(err.Error(), "1044")) {
			t.Fatalf("err=%v, want permanent unknown/denied database", err)
		}
	})

	t.Run("readiness follows the database", func(t *testing.T) {
		db, err := orm.OpenDatabase(context.Background(), m.config("pp", "secret-pass", "ppanel"))
		if err != nil {
			t.Fatal(err)
		}
		rport := freePort(t)
		rcmd := startRedisServer(t, rport, "redis-pass")
		rds := redis.NewClient(&redis.Options{Addr: fmt.Sprintf("127.0.0.1:%d", rport), Password: "redis-pass"})
		defer rds.Close()
		waitRedis(t, rds)
		a := &Application{db: db, Redis: rds}
		if err := a.Ready(context.Background()); err != nil {
			t.Fatalf("Ready with both dependencies up: %v", err)
		}
		_ = rcmd.Process.Kill()
		_, _ = rcmd.Process.Wait()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err = a.Ready(ctx)
		cancel()
		if err == nil || err.Error() != "redis unavailable" {
			t.Fatalf("Ready with Redis down = %v, want \"redis unavailable\"", err)
		}
		m.stop()
		ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
		start := time.Now()
		err = a.Ready(ctx)
		cancel()
		if err == nil || err.Error() != "database unavailable" {
			t.Fatalf("Ready with MySQL down = %v, want \"database unavailable\"", err)
		}
		if time.Since(start) > 4*time.Second {
			t.Fatalf("readiness probe took %s", time.Since(start))
		}
	})
}

func startRedisServer(t *testing.T, port int, pass string) *exec.Cmd {
	t.Helper()
	bin := os.Getenv("PPANEL_TEST_REDIS_BIN")
	if bin == "" {
		t.Skip("PPANEL_TEST_REDIS_BIN not set")
	}
	cmd := exec.Command(bin, "--bind", "127.0.0.1", "--port", fmt.Sprint(port), "--requirepass", pass,
		"--save", "", "--appendonly", "no", "--dir", t.TempDir())
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	return cmd
}

func waitRedis(t *testing.T, rds *redis.Client) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for rds.Ping(context.Background()).Err() != nil {
		if time.Now().After(deadline) {
			t.Fatal("redis did not start")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestRealRedisStartup(t *testing.T) {
	if os.Getenv("PPANEL_TEST_REDIS_BIN") == "" {
		t.Skip("PPANEL_TEST_REDIS_BIN not set")
	}
	t.Run("late start is retried", func(t *testing.T) {
		port := freePort(t)
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		rds := redis.NewClient(&redis.Options{Addr: addr, Password: "redis-pass"})
		defer rds.Close()
		go func() { time.Sleep(time.Second); startRedisServer(t, port, "redis-pass") }()
		start := time.Now()
		_, err := connectWithRetry(context.Background(), "redis", realPolicy, func(ctx context.Context) (struct{}, error) {
			return struct{}{}, rds.Ping(ctx).Err()
		}, permanentRedisError)
		if err != nil || time.Since(start) < time.Second {
			t.Fatalf("err=%v elapsed=%s", err, time.Since(start))
		}
	})

	t.Run("wrong password fails fast", func(t *testing.T) {
		port := freePort(t)
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		startRedisServer(t, port, "redis-pass")
		waitRedis(t, redis.NewClient(&redis.Options{Addr: addr, Password: "redis-pass"}))
		rds := redis.NewClient(&redis.Options{Addr: addr, Password: "wrong"})
		defer rds.Close()
		_, err := connectWithRetry(context.Background(), "redis", realPolicy, func(ctx context.Context) (struct{}, error) {
			return struct{}{}, rds.Ping(ctx).Err()
		}, permanentRedisError)
		var pe *PermanentError
		if !errors.As(err, &pe) {
			t.Fatalf("err=%v, want permanent auth failure", err)
		}
	})
}
