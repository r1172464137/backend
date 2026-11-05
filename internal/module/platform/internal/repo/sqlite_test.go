package repo

import (
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// openTestDB opens an in-memory SQLite database of the test's own and
// migrates models into it. The database name is unique per call: a
// shared-cache database lives as long as a connection to it is open, and the
// pool keeps one, so a fixed name hands a repeated run of the test (go test
// -count=2) the rows of the previous run.
func openTestDB(t *testing.T, name string, models ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", name, time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}
