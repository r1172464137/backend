package repo

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"github.com/perfect-panel/server/internal/module/platform/internal/systemsetting"
	"github.com/perfect-panel/server/pkg/cache"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestInterfaceConfigQueryQuotesReservedKey(t *testing.T) {
	var logs bytes.Buffer
	db, err := gorm.Open(mysql.New(mysql.Config{DSN: "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=true", SkipInitializeWithVersion: true}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: gormlogger.New(log.New(&logs, "", 0), gormlogger.Config{LogLevel: gormlogger.Info})})
	if err != nil {
		t.Fatal(err)
	}
	store := NewSystemRepo(cache.NewConn(db, nil))
	if _, err := store.GetInterfaceConfig(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"FROM `system`", "`category` = 'interface'", "`key` = 'AdminHiddenMenuIds'"} {
		if !strings.Contains(logs.String(), fragment) {
			t.Fatalf("query missing %q:\n%s", fragment, logs.String())
		}
	}
}

func TestInterfaceConfigDatabaseRoundTrip(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&system.System{}); err != nil {
		t.Fatal(err)
	}
	unrelated := system.System{Category: "site", Key: "SiteName", Value: "keep", Type: "string", Desc: "site"}
	if err = db.Create(&unrelated).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewSystemRepo(cache.NewConn(db, nil))
	service := systemsetting.NewService(systemsetting.Deps{System: repo})
	ctx := context.Background()
	got, err := service.GetInterfaceConfig(ctx)
	if err != nil || got.HiddenMenuIds == nil || len(got.HiddenMenuIds) != 0 {
		t.Fatalf("default: %#v %v", got, err)
	}
	for _, bad := range [][]string{nil, {"/dashboard/system"}, {"/dashboard/nope"}, {"/dashboard", "/dashboard"}, {""}} {
		if err := service.UpdateInterfaceConfig(ctx, &dto.InterfaceConfig{HiddenMenuIds: bad}); err == nil {
			t.Fatalf("accepted invalid %v", bad)
		}
	}
	if err := service.UpdateInterfaceConfig(ctx, &dto.InterfaceConfig{HiddenMenuIds: []string{"/dashboard/servers", "/dashboard"}}); err != nil {
		t.Fatal(err)
	}
	got, err = service.GetInterfaceConfig(ctx)
	if err != nil || len(got.HiddenMenuIds) != 2 || got.HiddenMenuIds[0] != "/dashboard/servers" {
		t.Fatalf("read: %#v %v", got, err)
	}
	if err := service.UpdateInterfaceConfig(ctx, &dto.InterfaceConfig{HiddenMenuIds: []string{}}); err != nil {
		t.Fatal(err)
	}
	got, err = service.GetInterfaceConfig(ctx)
	if err != nil || got.HiddenMenuIds == nil || len(got.HiddenMenuIds) != 0 {
		t.Fatalf("restore: %#v %v", got, err)
	}
	var saved system.System
	if err := db.Where("key = ?", "SiteName").First(&saved).Error; err != nil || saved.Value != "keep" {
		t.Fatalf("unrelated changed: %#v %v", saved, err)
	}
	if err := db.Model(&system.System{}).Where("key = ?", "AdminHiddenMenuIds").Update("value", "bad json").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetInterfaceConfig(ctx); err == nil {
		t.Fatal("accepted corrupt stored configuration")
	}
}
