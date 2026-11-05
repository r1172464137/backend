package app

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	mysqlDSN "github.com/go-sql-driver/mysql"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/repository"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestV2DeliverySnapshotMySQL(t *testing.T) {
	testV2DeliveryIsolation(t, "mysql", os.Getenv("PPANEL_TEST_V2_MYSQL_DSN"))
}
func TestV2DeliverySnapshotPostgres(t *testing.T) {
	testV2DeliveryIsolation(t, "postgres", os.Getenv("PPANEL_TEST_V2_POSTGRES_DSN"))
}

// Only run with the E2E owner's resource lock and a dedicated loopback _test DB.
func testV2DeliveryIsolation(t *testing.T, dialect, dsn string) {
	t.Helper()
	if dsn == "" {
		t.Skip("explicit isolated loopback _test DSN required")
	}
	var address, name string
	if dialect == "mysql" {
		cfg, err := mysqlDSN.ParseDSN(dsn)
		if err != nil || cfg.Net != "tcp" {
			t.Fatal("invalid TCP MySQL DSN")
		}
		address, name = cfg.Addr, cfg.DBName
	} else {
		u, err := url.Parse(dsn)
		if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
			t.Fatal("Postgres URL required")
		}
		address, name = u.Host, strings.TrimPrefix(u.Path, "/")
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal("explicit host and port required")
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		t.Fatal("only loopback DB allowed")
	}
	if !strings.HasSuffix(name, "_test") {
		t.Fatal("dedicated _test database required")
	}
	if host == "localhost" {
		_, port, _ := net.SplitHostPort(address)
		if dialect == "mysql" {
			cfg, _ := mysqlDSN.ParseDSN(dsn)
			cfg.Addr = net.JoinHostPort("127.0.0.1", port)
			dsn = cfg.FormatDSN()
		} else {
			u, _ := url.Parse(dsn)
			u.Host = net.JoinHostPort("127.0.0.1", port)
			dsn = u.String()
		}
	}
	var dialector gorm.Dialector
	if dialect == "mysql" {
		dialector = mysql.Open(dsn)
	} else {
		dialector = postgres.Open(dsn)
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("cannot open isolated test database")
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	row := &node.Server{Name: fmt.Sprintf("v2-isolation-%d", time.Now().UnixNano()), Address: "127.0.0.1", Protocols: `[]`}
	if err = db.WithContext(ctx).Create(row).Error; err != nil {
		t.Fatal("cannot create test row")
	}
	defer db.WithContext(context.Background()).Where("id = ?", row.Id).Delete(&node.Server{})
	store := NewStore(db, nil)
	err = store.InDeliverySnapshot(ctx, func(tx repository.DeliverySnapshot) error {
		first, err := tx.Node().FindOneServer(ctx, row.Id)
		if err != nil {
			return err
		}
		done := make(chan error, 1)
		go func() {
			done <- db.WithContext(ctx).Model(&node.Server{}).Where("id = ?", row.Id).Update("address", "127.0.0.2").Error
		}()
		select {
		case err = <-done:
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
		second, err := tx.Node().FindOneServer(ctx, row.Id)
		if err != nil {
			return err
		}
		if first.Address != "127.0.0.1" || second.Address != first.Address {
			return fmt.Errorf("snapshot changed during concurrent commit")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := store.Node().FindOneServer(ctx, row.Id)
	if err != nil || after.Address != "127.0.0.2" {
		t.Fatal("writer did not commit after snapshot")
	}
}
