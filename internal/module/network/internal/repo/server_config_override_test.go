package repo

import (
	"context"
	"testing"

	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestServerRoutingWritePreservesDormantLegacyColumns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:server-config-override?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&node.ServerConfigOverride{}); err != nil {
		t.Fatal(err)
	}
	block, ips, protocols, outbound := `["full:example.com"]`, `["geoip:private"]`, `["bittorrent"]`, `[]`
	if err := db.Create(&node.ServerConfigOverride{ServerId: 17, Block: &block, BlockIPs: &ips, BlockProtocols: &protocols, Outbound: &outbound}).Error; err != nil {
		t.Fatal(err)
	}
	store := NewNodeRepo(db, nil)
	routing := `{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct"}`
	if err := store.SaveServerRoutingOverride(context.Background(), &node.ServerConfigOverride{ServerId: 17, Routing: &routing, RoutingRevision: 3}); err != nil {
		t.Fatal(err)
	}
	legacy, err := store.FindServerConfigOverride(context.Background(), 17)
	if err != nil || legacy == nil || legacy.Block == nil || *legacy.Block != block || legacy.BlockIPs == nil || *legacy.BlockIPs != ips || legacy.BlockProtocols == nil || *legacy.BlockProtocols != protocols {
		t.Fatalf("dormant legacy %#v %v", legacy, err)
	}
	active, err := store.FindServerRoutingOverride(context.Background(), 17)
	if err != nil || active == nil || active.Routing == nil || *active.Routing != routing || active.RoutingRevision != 3 || active.Block != nil {
		t.Fatalf("active %#v %v", active, err)
	}
}
