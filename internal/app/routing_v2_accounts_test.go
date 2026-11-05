package app

import (
	"context"
	"encoding/json"
	"github.com/cloudwego/hertz/pkg/app"
	adminserver "github.com/perfect-panel/server/internal/module/network/transport/http/admin/server"
	adminsystem "github.com/perfect-panel/server/internal/module/platform/transport/http/admin/system"
	"strconv"
	"testing"

	"github.com/perfect-panel/server/internal/module/network"
	netdto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/platform"
	pdto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestV2AccountValidationUsesTransaction(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:v2-accounts?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	defer pool.Close()
	if err = db.AutoMigrate(&system.System{}, &node.Server{}, &node.ServerConfigOverride{}); err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`CREATE TABLE user (id INTEGER PRIMARY KEY, enable BOOLEAN, deleted_at DATETIME)`).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`INSERT INTO user (id,enable) VALUES (42,0)`).Error; err != nil {
		t.Fatal(err)
	}
	rows := []system.System{{Category: "server", Key: "Routing", Type: "string", Value: `{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct"}`}, {Category: "server", Key: "RoutingRevision", Type: "int64", Value: "1"}, {Category: "server", Key: "RoutingCutover", Type: "string", Value: "complete"}}
	if err = db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	server := node.Server{Name: "local", Address: "127.0.0.1", Protocols: `[]`}
	if err = db.Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	store := NewStore(db, nil)
	p := platform.New(platform.Deps{System: store.System(), Store: store, RoutingUsers: store.User()})
	ctx := context.Background()
	var global pdto.NodeConfig
	if err = json.Unmarshal([]byte(`{"routing_revision":1,"routing":{"version":2,"outbounds":[],"rules":[],"default_outbound_id":"direct","user_defaults":[{"user_id":42,"outbound_id":"direct"}]}}`), &global); err != nil {
		t.Fatal(err)
	}
	if err = p.UpdateNodeConfig(ctx, &global); err != nil {
		t.Fatalf("disabled existing account should be selectable: %v", err)
	}
	svc := network.New(network.Deps{Store: store, ReadGlobalNodeConfig: p.ReadNodeConfigSnapshot})
	var local netdto.UpdateServerNodeConfigRequest
	raw := `{"server_id":` + strconv.FormatInt(server.Id, 10) + `,"routing_revision":0,"routing":{"version":2,"outbounds":[],"rules":[],"default_outbound_id":"direct","user_defaults":[{"user_id":42,"outbound_id":"direct"}]}}`
	if err = json.Unmarshal([]byte(raw), &local); err != nil {
		t.Fatal(err)
	}
	if err = svc.UpdateServerNodeConfig(ctx, &local); err != nil {
		t.Fatalf("copy inherited accounts: %v", err)
	}
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
	globalHandler := adminsystem.UpdateNodeConfigHandler(p)
	check(globalHandler, `{"routing_revision":2,"routing":{"version":2,"outbounds":[],"rules":[],"default_outbound_id":"direct","user_defaults":[{"user_id":99,"outbound_id":"direct"}]}}`, 200, 400)
	check(globalHandler, `{"routing_revision":1,"routing":{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct"}}`, 409, 160001)
	check(globalHandler, `{"routing_revision":2,"routing":{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct"}}`, 409, 160004)
	serverHandler := adminserver.UpdateServerNodeConfigHandler(svc)
	check(serverHandler, `{"server_id":`+strconv.FormatInt(server.Id, 10)+`,"routing_revision":1,"routing":{"version":2,"outbounds":[],"rules":[],"default_outbound_id":"direct","user_defaults":[{"user_id":99,"outbound_id":"direct"}]}}`, 200, 400)
}
