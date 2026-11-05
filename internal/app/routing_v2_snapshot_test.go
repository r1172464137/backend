package app

import (
	"context"
	"testing"

	"github.com/perfect-panel/server/internal/module/network"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/platform"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// SQLite verifies transaction-bound repository wiring; MySQL/Postgres isolation
// still needs the existing dedicated database integration fixtures.
func TestV2SnapshotReadsAuthoritativeRepositories(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:v2-snapshot?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&system.System{}, &node.Server{}, &node.ServerConfigOverride{}); err != nil {
		t.Fatal(err)
	}
	rows := []system.System{
		{Category: "server", Key: "Routing", Type: "string", Value: `{"version":2,"outbounds":[],"rules":[],"default_outbound_id":"direct","user_defaults":[]}`},
		{Category: "server", Key: "RoutingRevision", Type: "int64", Value: "1"},
		{Category: "server", Key: "RoutingCutover", Type: "string", Value: "complete"},
	}
	if err = db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	server := node.Server{Name: "local", Address: "127.0.0.1", Protocols: `[]`}
	if err = db.Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	store := NewStore(db, nil)
	p := platform.New(platform.Deps{System: store.System(), Store: store})
	svc := network.New(network.Deps{Store: store, ReadGlobalNodeConfig: p.ReadNodeConfigSnapshot})
	request := &dto.QueryServerConfigRequest{ServerID: server.Id, RoutingVersion: 2}
	first, err := svc.QueryServerProtocolConfig(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.IdentitySnapshot == nil || len(first.IdentitySnapshot.SnapshotID) != 64 || first.Routing.Version != 2 {
		t.Fatalf("snapshot %+v", first)
	}
	second, err := svc.QueryServerProtocolConfig(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if second.IdentitySnapshot.SnapshotID != first.IdentitySnapshot.SnapshotID {
		t.Fatal("identical state changed snapshot ID")
	}
}
