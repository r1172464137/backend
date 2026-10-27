package repo

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestFindOneServerForUpdateUsesRowLock(t *testing.T) {
	var logs bytes.Buffer
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN: "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=true", SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, Logger: gormlogger.New(log.New(&logs, "", 0), gormlogger.Config{LogLevel: gormlogger.Info})})
	if err != nil {
		t.Fatal(err)
	}
	store := NewNodeRepo(db, nil)
	if _, err := store.FindOneServerForUpdate(context.Background(), 42); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"FROM `servers`", "WHERE id = 42", "FOR UPDATE"} {
		if !strings.Contains(logs.String(), fragment) {
			t.Fatalf("locking SQL missing %q:\n%s", fragment, logs.String())
		}
	}
}
