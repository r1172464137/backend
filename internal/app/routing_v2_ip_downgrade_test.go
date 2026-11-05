package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/network"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	adminserver "github.com/perfect-panel/server/internal/module/network/transport/http/admin/server"
	"github.com/perfect-panel/server/internal/module/platform"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	adminsystem "github.com/perfect-panel/server/internal/module/platform/transport/http/admin/system"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestV2IPRuleDowngradeHTTPAndInheritance(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "routing.sqlite")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	pool.SetMaxOpenConns(1)
	if err = db.AutoMigrate(&system.System{}, &node.Server{}, &node.ServerConfigOverride{}); err != nil {
		t.Fatal(err)
	}
	rows := []system.System{
		{Category: "server", Key: "Routing", Type: "string", Value: `{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct"}`},
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
	ctx := context.Background()
	global := adminsystem.UpdateNodeConfigHandler(p)
	local := adminserver.UpdateServerNodeConfigHandler(svc)
	const rule = `{"id":"rr_11111111-1111-4111-8111-111111111111","enabled":true,"remark":"","kind":"ip","values":["198.51.100.0/24"],"outbound_id":"block"}`
	v1 := `{"version":1,"outbounds":[],"rules":[` + rule + `],"default_outbound_id":"direct"}`
	v2 := `{"version":2,"outbounds":[],"rules":[` + rule + `],"default_outbound_id":"direct","user_defaults":[]}`
	check := func(handler app.HandlerFunc, body string, status int, code uint32) {
		t.Helper()
		c := app.NewContext(0)
		c.Request.Header.SetContentTypeBytes([]byte("application/json"))
		c.Request.SetBodyString(body)
		handler(ctx, c)
		var result struct {
			Code uint32 `json:"code"`
		}
		if err := json.Unmarshal(c.Response.Body(), &result); err != nil || c.Response.StatusCode() != status || result.Code != code {
			t.Fatalf("HTTP %d code %d body %s err %v", c.Response.StatusCode(), result.Code, c.Response.Body(), err)
		}
	}
	check(global, `{"routing_revision":1,"routing":`+v2+`}`, 200, 200)
	check(global, `{"node_push_interval":60}`, 200, 200)
	check(global, `{"routing_revision":2,"routing":`+v1+`}`, 409, 160004)
	current, err := p.GetNodeConfig(ctx)
	if err != nil || current.Routing.Version != 2 || current.RoutingRevision != 2 {
		t.Fatalf("blocked global downgrade: %+v %v", current, err)
	}
	check(global, `{"routing_revision":2,"routing_downgrade":true,"routing":`+v1+`}`, 200, 200)
	check(global, `{"routing_revision":3,"routing":`+v2+`}`, 200, 200)

	serverID := server.Id
	check(local, fmt.Sprintf(`{"server_id":%d,"routing_revision":0,"routing":%s}`, serverID, v2), 200, 200)
	check(local, fmt.Sprintf(`{"server_id":%d,"ip_strategy":"prefer_ipv4","inherit_ip_strategy":false}`, serverID), 200, 200)
	check(local, fmt.Sprintf(`{"server_id":%d,"routing_revision":1,"routing":%s}`, serverID, v1), 409, 140005)
	state, err := svc.GetServerNodeConfig(ctx, &dto.GetServerNodeConfigRequest{ServerID: serverID})
	if err != nil || state.Override.Routing == nil || state.Override.Routing.Version != 2 || state.Override.RoutingRevision != 1 {
		t.Fatalf("blocked local downgrade: %+v %v", state, err)
	}
	check(local, fmt.Sprintf(`{"server_id":%d,"routing_revision":1,"routing_downgrade":true,"routing":%s}`, serverID, v1), 200, 200)
	check(local, fmt.Sprintf(`{"server_id":%d,"routing_revision":2,"routing":%s}`, serverID, v2), 200, 200)
	check(local, fmt.Sprintf(`{"server_id":%d,"routing_revision":3,"inherit_routing":true}`, serverID), 200, 200)
	state, err = svc.GetServerNodeConfig(ctx, &dto.GetServerNodeConfigRequest{ServerID: serverID})
	if err != nil || !state.Override.InheritRouting || state.Override.RoutingRevision != 4 || state.Effective.Routing.Version != 2 {
		t.Fatalf("v2 inheritance: %+v %v", state, err)
	}
	// Inheriting a v1 global package is a downgrade of the same IP-only local v2.
	check(local, fmt.Sprintf(`{"server_id":%d,"routing_revision":4,"routing":%s}`, serverID, v2), 200, 200)
	check(global, `{"routing_revision":4,"routing_downgrade":true,"routing":`+v1+`}`, 200, 200)
	check(local, fmt.Sprintf(`{"server_id":%d,"routing_revision":5,"inherit_routing":true}`, serverID), 409, 140005)
	check(local, fmt.Sprintf(`{"server_id":%d,"routing_revision":5,"routing_downgrade":true,"inherit_routing":true}`, serverID), 200, 200)
}
