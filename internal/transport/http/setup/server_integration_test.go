package setup

import (
	"context"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/perfect-panel/server/pkg/orm"
)

func TestDirectSetupEmptyMySQL(t *testing.T) {
	directSetupEmpty(t, "mysql", os.Getenv("PPANEL_TEST_SETUP_EMPTY_MYSQL_DSN"))
}
func TestDirectSetupEmptyPostgres(t *testing.T) {
	directSetupEmpty(t, "postgres", os.Getenv("PPANEL_TEST_SETUP_EMPTY_POSTGRES_DSN"))
}

func directSetupEmpty(t *testing.T, driver, dsn string) {
	t.Helper()
	if dsn == "" {
		t.Skip("dedicated empty loopback database required")
	}
	var addr, name, user, password string
	if driver == "mysql" {
		cfg, err := mysqlDriver.ParseDSN(dsn)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Net != "tcp" {
			t.Fatal("TCP required")
		}
		addr, name, user, password = cfg.Addr, cfg.DBName, cfg.User, cfg.Passwd
	} else {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		addr, name = u.Host, strings.TrimPrefix(u.Path, "/")
		if u.User != nil {
			user = u.User.Username()
			password, _ = u.User.Password()
		}
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		t.Fatal("non-loopback refused")
	}
	if !strings.HasSuffix(name, "_test") {
		t.Fatal("dedicated _test DB required")
	}
	cfgLock := orm.ParseDSN(dsn)
	if cfgLock == nil {
		t.Fatal("invalid DSN")
	}
	cfgLock.Driver = driver
	lockDB, err := orm.ConnectDatabase(orm.Mysql{Config: *cfgLock})
	if err != nil {
		t.Fatal(err)
	}
	lockPool, _ := lockDB.DB()
	conn1, err := lockPool.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	conn2, err := lockPool.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release, err := lockSetupDatabase(context.Background(), conn1, *cfgLock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockSetupDatabase(context.Background(), conn2, *cfgLock); err == nil {
		t.Fatal("second installation acquired same DB lock")
	}
	release()
	release2, err := lockSetupDatabase(context.Background(), conn2, *cfgLock)
	if err != nil {
		t.Fatalf("setup lock did not release: %v", err)
	}
	release2()
	conn1.Close()
	conn2.Close()
	lockPool.Close()
	oldPath := configPath
	configPath = filepath.Join(t.TempDir(), "config.yaml")
	t.Cleanup(func() { configPath = oldPath })
	if err := os.WriteFile(configPath, []byte(""), 0600); err != nil {
		t.Fatal(err)
	}
	request := map[string]string{"adminEmail": "setup@example.invalid", "adminPassword": "temporary-setup-password", "databaseDriver": driver, "mysqlHost": host, "mysqlPort": port, "mysqlDatabase": name, "mysqlUser": user, "mysqlPassword": password, "redisHost": "127.0.0.1", "redisPort": "6379"}
	body, _ := json.Marshal(request)
	done := make(chan bool, 1)
	go func() { done <- <-initStatus }()
	start := make(chan struct{})
	responses := make(chan *app.RequestContext, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := app.NewContext(0)
			ctx.Request.Header.SetContentTypeBytes([]byte("application/json"))
			ctx.Request.SetBody(body)
			<-start
			handleInitConfig(context.Background(), ctx)
			responses <- ctx
		}()
	}
	close(start)
	wg.Wait()
	close(responses)
	success, conflict := 0, 0
	for ctx := range responses {
		switch {
		case ctx.Response.StatusCode() == 200 && strings.Contains(string(ctx.Response.Body()), `"code":200`):
			success++
		case ctx.Response.StatusCode() == 409:
			conflict++
		default:
			t.Fatalf("concurrent setup status=%d body=%s", ctx.Response.StatusCode(), ctx.Response.Body())
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrent setup success=%d conflict=%d", success, conflict)
	}
	if !<-done {
		t.Fatal("setup did not signal completion")
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "temporary-setup-password") || strings.Contains(string(content), "PasswordHash") {
		t.Fatal("staged credentials retained after successful installation")
	}
	info, err := os.Stat(configPath)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("config permissions %v %v", info, err)
	}
	cfg := orm.ParseDSN(dsn)
	cfg.Driver = driver
	db, err := orm.ConnectDatabase(orm.Mysql{Config: *cfg})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	defer pool.Close()
	var count int64
	if err := db.Table("user").Where("is_admin = ?", true).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("admin count %d: %v", count, err)
	}
}

func TestDirectSetupRejectsExisting2179MySQL(t *testing.T) {
	directSetupRejectsExisting(t, "mysql", os.Getenv("PPANEL_TEST_SETUP_MYSQL_DSN"))
}
func TestDirectSetupRejectsExisting2179Postgres(t *testing.T) {
	directSetupRejectsExisting(t, "postgres", os.Getenv("PPANEL_TEST_SETUP_POSTGRES_DSN"))
}
func directSetupRejectsExisting(t *testing.T, driver, dsn string) {
	t.Helper()
	if dsn == "" {
		t.Skip("dedicated loopback 02179 database required")
	}
	var hostport, name, user, password string
	if driver == "mysql" {
		cfg, err := mysqlDriver.ParseDSN(dsn)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Net != "tcp" {
			t.Fatal("TCP required")
		}
		hostport, name, user, password = cfg.Addr, cfg.DBName, cfg.User, cfg.Passwd
	} else {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		hostport, name = u.Host, strings.TrimPrefix(u.Path, "/")
		if u.User != nil {
			user = u.User.Username()
			password, _ = u.User.Password()
		}
	}
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		t.Fatal(err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		t.Fatal("non-loopback refused")
	}
	if !strings.HasSuffix(name, "_test") {
		t.Fatal("dedicated _test DB required")
	}
	cfg := orm.ParseDSN(dsn)
	if cfg == nil {
		t.Fatal("invalid DSN")
	}
	cfg.Driver = driver
	db, err := orm.ConnectDatabase(orm.Mysql{Config: *cfg})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	defer pool.Close()
	var version int64
	var dirty bool
	if err := db.Raw("SELECT version,dirty FROM schema_migrations").Row().Scan(&version, &dirty); err != nil || dirty || version != 2179 {
		t.Fatalf("expected 02179 fixture: version=%d dirty=%v err=%v", version, dirty, err)
	}
	oldPath := configPath
	configPath = filepath.Join(t.TempDir(), "config.yaml")
	t.Cleanup(func() { configPath = oldPath })
	if err := os.WriteFile(configPath, []byte(""), 0600); err != nil {
		t.Fatal(err)
	}
	request := map[string]string{"adminEmail": "local@example.invalid", "adminPassword": "local-test", "databaseDriver": driver, "mysqlHost": host, "mysqlPort": port, "mysqlDatabase": name, "mysqlUser": user, "mysqlPassword": password, "redisHost": "127.0.0.1", "redisPort": "6379"}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	ctx := app.NewContext(0)
	ctx.Request.Header.SetContentTypeBytes([]byte("application/json"))
	ctx.Request.SetBody(body)
	// Intentionally do not call /init/database/test.
	handleInitConfig(context.Background(), ctx)
	if ctx.Response.StatusCode() != 409 {
		t.Fatalf("direct submission status=%d body=%s", ctx.Response.StatusCode(), ctx.Response.Body())
	}
	if err := db.Raw("SELECT version,dirty FROM schema_migrations").Row().Scan(&version, &dirty); err != nil || dirty || version != 2179 {
		t.Fatalf("direct setup changed schema version: %d/%v/%v", version, dirty, err)
	}
	if db.Migrator().HasColumn("server_config_overrides", "routing") {
		t.Fatal("direct setup executed 02180 DDL")
	}
	content, err := os.ReadFile(configPath)
	if err != nil || len(content) != 0 {
		t.Fatal("setup changed config file")
	}
}
