package repo

import (
	"context"
	"testing"

	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"github.com/perfect-panel/server/pkg/cache"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRuntimeSnapshotExcludesDormantLegacyRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:ordered-platform-snapshot?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&system.System{}); err != nil {
		t.Fatal(err)
	}
	for _, item := range []system.System{{Key: "Block", Category: "server", Value: `["geosite:old"]`, Type: "string", Desc: "old"}, {Key: "Outbound", Category: "server", Value: `[]`, Type: "string", Desc: "old"}, {Key: "Routing", Category: "server", Value: `{}`, Type: "string", Desc: "v1"}} {
		value := item
		if err := db.Create(&value).Error; err != nil {
			t.Fatal(err)
		}
	}
	store := NewSystemRepo(cache.NewConn(db, nil))
	active, err := store.GetNodeConfigUncached(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].Key != "Routing" {
		t.Fatalf("runtime read dormant rows %#v", active)
	}
	legacy, err := store.GetLegacyNodeConfigUncached(context.Background())
	if err != nil || len(legacy) != 3 {
		t.Fatalf("preflight cannot read legacy inputs %#v %v", legacy, err)
	}
}
