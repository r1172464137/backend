package repo

import (
	"context"
	"testing"

	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestServerConfigOverrideRepositoryRoundTripsStructuredBlockColumns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:server-config-override?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&node.ServerConfigOverride{}); err != nil {
		t.Fatal(err)
	}

	block := `["full:example.com"]`
	blockIPs := `["geoip:private"]`
	blockProtocols := `["bittorrent"]`
	outbound := `[]`
	store := NewNodeRepo(db, nil)
	if err := store.SaveServerConfigOverride(context.Background(), &node.ServerConfigOverride{
		ServerId: 17, Block: &block, BlockIPs: &blockIPs, BlockProtocols: &blockProtocols, Outbound: &outbound,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := store.FindServerConfigOverride(context.Background(), 17)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Block == nil || got.BlockIPs == nil || got.BlockProtocols == nil ||
		*got.Block != block || *got.BlockIPs != blockIPs || *got.BlockProtocols != blockProtocols {
		t.Fatalf("stored override = %#v, want all structured block columns", got)
	}
}
