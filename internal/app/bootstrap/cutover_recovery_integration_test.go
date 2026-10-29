package bootstrap

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/perfect-panel/server/internal/app/migration/schema"
	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/billing"
	"github.com/perfect-panel/server/internal/module/identity"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/internal/module/notification"
	"github.com/perfect-panel/server/internal/module/platform"
	"github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/internal/module/support"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/conf"
	"github.com/perfect-panel/server/pkg/orm"
	"github.com/redis/go-redis/v9"
)

func TestEmptyMigrationRecoveryMySQL(t *testing.T) {
	testFreshRecovery(t, "mysql", "empty", os.Getenv("PPANEL_TEST_EMPTY_MYSQL_DSN"))
}
func TestEmptyMigrationRecoveryPostgres(t *testing.T) {
	testFreshRecovery(t, "postgres", "empty", os.Getenv("PPANEL_TEST_EMPTY_POSTGRES_DSN"))
}
func TestDirtyFirstMigrationRecoveryMySQL(t *testing.T) {
	testFreshRecovery(t, "mysql", "dirty", os.Getenv("PPANEL_TEST_DIRTY_MYSQL_DSN"))
}
func TestDirtyFirstMigrationRecoveryPostgres(t *testing.T) {
	testFreshRecovery(t, "postgres", "dirty", os.Getenv("PPANEL_TEST_DIRTY_POSTGRES_DSN"))
}
func TestAdminSeedCrashRecoveryMySQL(t *testing.T) {
	testFreshRecovery(t, "mysql", "seed", os.Getenv("PPANEL_TEST_SEED_MYSQL_DSN"))
}
func TestAdminSeedCrashRecoveryPostgres(t *testing.T) {
	testFreshRecovery(t, "postgres", "seed", os.Getenv("PPANEL_TEST_SEED_POSTGRES_DSN"))
}

func testFreshRecovery(t *testing.T, dialect, mode, dsn string) {
	t.Helper()
	if dsn == "" {
		t.Skip("dedicated empty loopback database required")
	}
	var address, name string
	if dialect == "mysql" {
		v, err := mysqlDriver.ParseDSN(dsn)
		if err != nil {
			t.Fatal(err)
		}
		if v.Net != "tcp" {
			t.Fatal("TCP required")
		}
		address, name = v.Addr, v.DBName
	} else {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		address, name = u.Host, strings.TrimPrefix(u.Path, "/")
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		t.Fatal("non-loopback refused")
	}
	if !strings.HasSuffix(name, "_test") {
		t.Fatal("dedicated _test database required")
	}
	cfg := orm.ParseDSN(dsn)
	if cfg == nil {
		t.Fatal("invalid DSN")
	}
	cfg.Driver = dialect
	db, err := orm.ConnectDatabase(orm.Mysql{Config: *cfg})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	defer pool.Close()
	ctx := context.Background()
	if err := schema.RequireNoBusinessObjects(ctx, db, false); err != nil {
		t.Fatalf("fixture not empty: %v", err)
	}
	mini := miniredis.RunT(t)
	rds := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer rds.Close()
	store := repository.NewGormStoreWithBuilders(db, rds, repository.Builders{Platform: platform.NewRepoBuilder(), Network: network.NewRepoBuilder(rds), Billing: billing.NewRepoBuilder(), Subscription: subscription.NewRepoBuilder(), Identity: identity.NewRepoBuilder(), Support: support.NewRepoBuilder(), Notification: notification.NewRepoBuilder()})
	c := config.Config{}
	c.SetDatabaseConfig(*cfg)
	c.Administrator.Email = "recovery@example.invalid"
	c.Administrator.Password = "isolated-test-only"
	stagedPath := ""
	if mode == "seed" {
		c.Administrator.PasswordHash = password.EncodePassWord("isolated-test-only")
		c.Administrator.Password = ""
		stagedPath = filepath.Join(t.TempDir(), "config.yaml")
		content := []byte("Host: 127.0.0.1\nAdministrator:\n  Email: recovery@example.invalid\n  PasswordHash: " + c.Administrator.PasswordHash + "\n")
		if err := os.WriteFile(stagedPath, content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	deps := &Dependencies{Config: func() config.Config { return c }, Store: store, UpdateConfig: func(update func(*config.Config)) { update(&c) }}
	if stagedPath != "" {
		deps.FinalizeStagedAdmin = func(hash string) error { return conf.ClearStagedAdminHash(stagedPath, hash) }
	}
	if mode != "seed" {
		if err := db.Exec("CREATE TABLE schema_migrations (version BIGINT NOT NULL PRIMARY KEY, dirty BOOLEAN NOT NULL)").Error; err != nil {
			t.Fatal(err)
		}
		if mode == "dirty" {
			if err := db.Exec("INSERT INTO schema_migrations (version,dirty) VALUES (1,?)", true).Error; err != nil {
				t.Fatal(err)
			}
		}
		// An unrelated view/sequence must prevent automatic recovery, even if
		// the version table is empty or dirty at version 1.
		if dialect == "mysql" {
			if err := db.Exec("CREATE VIEW existing_probe AS SELECT 1 AS v").Error; err != nil {
				t.Fatal(err)
			}
		} else {
			if err := db.Exec("CREATE SEQUENCE existing_probe").Error; err != nil {
				t.Fatal(err)
			}
		}
		if _, _, _, err := schemaVersionState(ctx, *cfg); err == nil {
			t.Fatal("recovery accepted unrelated schema object")
		}
		if dialect == "mysql" {
			if err := db.Exec("DROP VIEW existing_probe").Error; err != nil {
				t.Fatal(err)
			}
		} else {
			if err := db.Exec("DROP SEQUENCE existing_probe").Error; err != nil {
				t.Fatal(err)
			}
		}
		version, exists, recoverFirst, err := schemaVersionState(ctx, *cfg)
		if err != nil || exists || version != 0 || recoverFirst != (mode == "dirty") {
			t.Fatalf("state %d/%v/%v %v", version, exists, recoverFirst, err)
		}
	} else {
		// Inject a crash after migration and cutover commit, before the admin
		// seed: the next no-change startup must finish provisioning.
		plan, err := preflightOrderedCutover(ctx, store, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.Up(dialect, dsn); err != nil {
			t.Fatal(err)
		}
		if err := applyOrderedCutover(ctx, store, plan); err != nil {
			t.Fatal(err)
		}
		count, err := store.User().QueryResisterUserTotal(ctx)
		if err != nil || count != 0 {
			t.Fatalf("expected no admin before injected crash: %d %v", count, err)
		}
	}
	if stagedPath != "" {
		finish := deps.FinalizeStagedAdmin
		deps.FinalizeStagedAdmin = func(string) error { return errors.New("injected cleanup crash") }
		failed := false
		func() { defer func() { failed = recover() != nil }(); Migrate(deps) }()
		if !failed {
			t.Fatal("injected cleanup failure did not stop startup")
		}
		count, err := store.User().QueryResisterUserTotal(ctx)
		if err != nil || count != 1 {
			t.Fatalf("admin seed did not commit before cleanup crash: %d %v", count, err)
		}
		if body, err := os.ReadFile(stagedPath); err != nil || !strings.Contains(string(body), "PasswordHash") {
			t.Fatal("failed cleanup unexpectedly removed staged hash")
		}
		deps.FinalizeStagedAdmin = finish
	}
	Migrate(deps)
	if stagedPath != "" {
		// Mirrors cmd/run.go after successful seed: only then remove the exact
		// staged hash atomically, including after a no-change crash recovery.
		body, err := os.ReadFile(stagedPath)
		if err != nil || strings.Contains(string(body), "PasswordHash") || c.Administrator.PasswordHash != "" {
			t.Fatalf("staged hash not cleared: %v", err)
		}
	}
	count, err := store.User().QueryResisterUserTotal(ctx)
	if err != nil || count != 1 {
		t.Fatalf("admin seed count %d: %v", count, err)
	}
	var admins, auths int64
	if err := db.Table("user").Where("is_admin = ?", true).Count(&admins).Error; err != nil || admins != 1 {
		t.Fatalf("admin seed rows %d: %v", admins, err)
	}
	if err := db.Table("user_auth_methods").Where("auth_type = ? AND auth_identifier = ?", "email", "recovery@example.invalid").Count(&auths).Error; err != nil || auths != 1 {
		t.Fatalf("admin auth rows %d: %v", auths, err)
	}
	var stored string
	if err := db.Table("user").Select("password").Where("is_admin = ?", true).Row().Scan(&stored); err != nil || !password.MultiPasswordVerify(password.PasswordAlgoArgon2id, "", "isolated-test-only", stored) {
		t.Fatalf("staged admin credentials not usable: %v", err)
	}
	Migrate(deps)
	count, err = store.User().QueryResisterUserTotal(ctx)
	if err != nil || count != 1 {
		t.Fatalf("admin seed repeated: %d %v", count, err)
	}
}
