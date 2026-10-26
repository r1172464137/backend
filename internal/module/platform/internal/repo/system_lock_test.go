package repo

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	"github.com/perfect-panel/server/pkg/cache"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestGetNodeConfigForUpdateUsesUncachedRowLock(t *testing.T) {
	var logs bytes.Buffer
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN: "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=true", SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: gormlogger.New(log.New(&logs, "", 0), gormlogger.Config{LogLevel: gormlogger.Info})})
	if err != nil {
		t.Fatal(err)
	}
	store := NewSystemRepo(cache.NewConn(db, nil))
	if _, err := store.GetNodeConfigForUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"FROM `system`", "category = 'server'", "ORDER BY id ASC", "FOR UPDATE"} {
		if !strings.Contains(logs.String(), fragment) {
			t.Fatalf("locking SQL missing %q:\n%s", fragment, logs.String())
		}
	}
}

func TestUpdateValueByCategoryKeyUsesAtomicUpsert(t *testing.T) {
	var logs bytes.Buffer
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN: "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=true", SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: gormlogger.New(log.New(&logs, "", 0), gormlogger.Config{LogLevel: gormlogger.Info})})
	if err != nil {
		t.Fatal(err)
	}
	store := NewSystemRepo(cache.NewConn(db, nil))
	if err := store.UpdateValueByCategoryKey(context.Background(), "server", "BlockIPs", `[]`, "string"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("upsert SQL is not atomic:\n%s", logs.String())
	}
}
