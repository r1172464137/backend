package schema

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/perfect-panel/server/pkg/orm"
)

// The dedicated environment variables are intentionally separate from the
// general migration tests: these cases exercise destructive down/up. They
// accept only a loopback server and a database whose name ends in _test.
// Prepare an isolated 02179 database for a real backup-and-restore rehearsal.
func TestPrepareCutoverBackupMySQL(t *testing.T) {
	prepareCutoverBackup(t, orm.DriverMySQL, os.Getenv("PPANEL_TEST_PREPARE_CUTOVER_MYSQL_DSN"))
}
func TestPrepareCutoverBackupPostgres(t *testing.T) {
	prepareCutoverBackup(t, orm.DriverPostgres, os.Getenv("PPANEL_TEST_PREPARE_CUTOVER_POSTGRES_DSN"))
}
func prepareCutoverBackup(t *testing.T, driver, dsn string) {
	t.Helper()
	if dsn == "" {
		t.Skip("dedicated loopback cutover backup DSN required")
	}
	proof := os.Getenv("PPANEL_NODE_PROOF")
	if proof == "" {
		t.Fatal("PPANEL_NODE_PROOF is required before creating the fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, proof)
	cmd.Stdin = strings.NewReader(`{"kind":"spider_uri","uri":"/"}`)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("node proof precheck: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	var result struct {
		OK bool `json:"ok"`
	}
	if json.Unmarshal(lines[len(lines)-1], &result) != nil || !result.OK {
		t.Fatal("node proof precheck failed")
	}
	if os.Getenv("PPANEL_TEST_XRAY") == "" {
		t.Fatal("PPANEL_TEST_XRAY required before creating the fixture")
	}
	if _, err := exec.LookPath(os.Getenv("PPANEL_TEST_XRAY")); err != nil {
		t.Fatal(err)
	}
	if driver == orm.DriverMySQL {
		cfg, err := mysqlDriver.ParseDSN(dsn)
		if err != nil {
			t.Fatal(err)
		}
		verifyDisposableDatabase(t, cfg.Net, cfg.Addr, cfg.DBName)
	} else {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		verifyDisposableDatabase(t, "tcp", u.Host, strings.TrimPrefix(u.Path, "/"))
	}
	config := orm.ParseDSN(dsn)
	if config == nil {
		t.Fatal("invalid DSN")
	}
	config.Driver = driver
	db, err := orm.ConnectDatabase(orm.Mysql{Config: *config})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := RequireNoBusinessObjects(context.Background(), db, false); err != nil {
		pool.Close()
		t.Fatalf("backup fixture must be empty: %v", err)
	}
	pool.Close()
	m := Migrate(driver, dsn)
	defer m.Close()
	version, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		t.Fatal(err)
	}
	if err == nil && (dirty || version != 2179) {
		t.Fatalf("backup fixture must start empty or at 02179; got %d dirty=%v", version, dirty)
	}
	if errors.Is(err, migrate.ErrNilVersion) {
		if err := m.Migrate(2179); err != nil {
			t.Fatal(err)
		}
	}
	version, dirty, err = m.Version()
	if err != nil || dirty || version != 2179 {
		t.Fatalf("expected clean 02179, got %d dirty=%v error=%v", version, dirty, err)
	}
	seedDB, err := orm.ConnectDatabase(orm.Mysql{Config: *config})
	if err != nil {
		t.Fatal(err)
	}
	seedPool, err := seedDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer seedPool.Close()
	if err := CreateAdminUser("local-backup@example.invalid", "local-test-password", seedDB); err != nil {
		t.Fatal(err)
	}
}

func TestOrderedRoutingDownMySQL(t *testing.T) {
	runOrderedRoutingDown(t, orm.DriverMySQL, os.Getenv("PPANEL_TEST_ORDERED_ROUTING_MYSQL_DSN"))
}
func TestOrderedRoutingDownPostgres(t *testing.T) {
	runOrderedRoutingDown(t, orm.DriverPostgres, os.Getenv("PPANEL_TEST_ORDERED_ROUTING_POSTGRES_DSN"))
}

func runOrderedRoutingDown(t *testing.T, dialect, dsn string) {
	t.Helper()
	if dsn == "" {
		t.Skip("set the dedicated loopback-only ordered-routing test DSN")
	}
	var db *sql.DB
	var err error
	if dialect == orm.DriverMySQL {
		cfg, parseErr := mysqlDriver.ParseDSN(dsn)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		verifyDisposableDatabase(t, cfg.Net, cfg.Addr, cfg.DBName)
		cfg.MultiStatements = true
		db, err = sql.Open("mysql", cfg.FormatDSN())
	} else {
		u, parseErr := url.Parse(dsn)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		verifyDisposableDatabase(t, "tcp", u.Host, strings.TrimPrefix(u.Path, "/"))
		db, err = sql.Open("pgx", dsn)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := Up(dialect, dsn); err != nil && !errors.Is(err, NoChange) {
		t.Fatal(err)
	}
	up, err := sqlFiles.ReadFile(fmt.Sprintf("database/%s/02180_ordered_node_routing.up.sql", dialect))
	if err != nil {
		t.Fatal(err)
	}
	down, err := sqlFiles.ReadFile(fmt.Sprintf("database/%s/02180_ordered_node_routing.down.sql", dialect))
	if err != nil {
		t.Fatal(err)
	}
	identifier := func(value string) string {
		if dialect == orm.DriverMySQL {
			return "`" + value + "`"
		}
		return `"` + value + `"`
	}
	placeholder := func(n int) string {
		if dialect == orm.DriverMySQL {
			return "?"
		}
		return fmt.Sprintf("$%d", n)
	}
	key, value, systemTable := identifier("key"), identifier("value"), identifier("system")
	const overrideID = int64(876543219)
	const extraKey = "__ordered_routing_test_large_value__"
	cleanup := func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM server_config_overrides WHERE server_id = "+placeholder(1), overrideID)
		_, _ = db.ExecContext(ctx, "DELETE FROM "+systemTable+" WHERE "+key+" = "+placeholder(1), extraKey)
		_, _ = db.ExecContext(ctx, "UPDATE "+systemTable+" SET "+value+" = '' WHERE "+key+" = 'Routing'")
	}
	defer cleanup()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("exec %s: %v", query, err)
		}
	}
	assertColumnsExist := func() {
		t.Helper()
		rows, err := db.QueryContext(ctx, "SELECT routing, routing_revision FROM server_config_overrides LIMIT 1")
		if err != nil {
			t.Fatalf("guard changed schema: %v", err)
		}
		rows.Close()
	}
	blocked := func(label string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, string(down)); err == nil {
			t.Fatalf("%s failed to block down", label)
		}
		assertColumnsExist()
	}
	cleanup()
	exec("UPDATE "+systemTable+" SET "+value+" = "+placeholder(1)+" WHERE "+key+" = 'Routing'", `{"version":1}`)
	blocked("global v1")
	exec("UPDATE "+systemTable+" SET "+value+" = "+placeholder(1)+" WHERE "+key+" = 'Routing'", " ")
	blocked("non-empty invalid global routing")
	exec("UPDATE " + systemTable + " SET " + value + " = '' WHERE " + key + " = 'Routing'")
	local := `{"data":"` + strings.Repeat("x", 1024*1024-len(`{"data":""}`)) + `"}`
	exec("INSERT INTO server_config_overrides (server_id, routing) VALUES ("+placeholder(1)+", "+placeholder(2)+")", overrideID, local)
	var localRoundtrip string
	if err := db.QueryRowContext(ctx, "SELECT routing FROM server_config_overrides WHERE server_id = "+placeholder(1), overrideID).Scan(&localRoundtrip); err != nil || localRoundtrip != local {
		t.Fatalf("1 MiB override did not round-trip: %v", err)
	}
	blocked("local v1")
	exec("DELETE FROM server_config_overrides WHERE server_id = "+placeholder(1), overrideID)
	if dialect == orm.DriverMySQL {
		large := strings.Repeat("x", 1024*1024)
		exec("INSERT INTO "+systemTable+" (category, "+key+", "+value+", type, "+identifier("desc")+") VALUES ("+placeholder(1)+", "+placeholder(2)+", "+placeholder(3)+", "+placeholder(4)+", "+placeholder(5)+")", "server", extraKey, large, "string", "test")
		var roundtrip string
		if err := db.QueryRowContext(ctx, "SELECT "+value+" FROM "+systemTable+" WHERE "+key+" = "+placeholder(1), extraKey).Scan(&roundtrip); err != nil || roundtrip != large {
			t.Fatalf("1 MiB value did not round-trip: %v", err)
		}
		blocked("oversize system.value")
		boundary := strings.Repeat("中", 65535/3)
		exec("UPDATE "+systemTable+" SET "+value+" = "+placeholder(1)+" WHERE "+key+" = "+placeholder(2), boundary, extraKey)
	}
	if _, err := db.ExecContext(ctx, string(down)); err != nil {
		t.Fatalf("safe down: %v", err)
	}
	if rows, err := db.QueryContext(ctx, "SELECT routing FROM server_config_overrides LIMIT 1"); err == nil {
		rows.Close()
		t.Fatal("down did not remove routing column")
	}
	if dialect == orm.DriverMySQL {
		var boundary string
		if err := db.QueryRowContext(ctx, "SELECT "+value+" FROM "+systemTable+" WHERE "+key+" = "+placeholder(1), extraKey).Scan(&boundary); err != nil || boundary != strings.Repeat("中", 65535/3) {
			t.Fatalf("65535-byte value changed on down: %v", err)
		}
		exec("DELETE FROM "+systemTable+" WHERE "+key+" = "+placeholder(1), extraKey)
	}
	if _, err := db.ExecContext(ctx, string(up)); err != nil {
		t.Fatalf("reapply up: %v", err)
	}
	assertColumnsExist()
}

func verifyDisposableDatabase(t *testing.T, network, address, name string) {
	t.Helper()
	if network != "tcp" || !strings.HasSuffix(name, "_test") {
		t.Fatal("destructive migration test requires loopback TCP and a database name ending in _test")
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		t.Fatal("destructive migration test refuses non-loopback databases")
	}
}
