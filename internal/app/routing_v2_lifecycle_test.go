package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"encoding/json"
	"github.com/alicebob/miniredis/v2"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route/param"
	serverv1 "github.com/perfect-panel/server/api/server/v1"
	"github.com/perfect-panel/server/internal/module/network"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	nodehttp "github.com/perfect-panel/server/internal/module/network/transport/http/server"
	"github.com/perfect-panel/server/internal/module/platform"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestV2SnapshotLifecycleAndDesired(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:v2-lifecycle-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&system.System{}, &node.Server{}, &node.ServerConfigOverride{}, &node.Node{}, &subscribe.Subscribe{}); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`CREATE TABLE user (id INTEGER PRIMARY KEY, enable BOOLEAN, deleted_at DATETIME)`,
		`CREATE TABLE user_subscribe (id INTEGER PRIMARY KEY, user_id INTEGER, subscribe_id INTEGER, uuid TEXT, status INTEGER, expire_time DATETIME, traffic INTEGER, download INTEGER, upload INTEGER)`,
	} {
		if err = db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	rows := []system.System{
		{Category: "server", Key: "Routing", Type: "string", Value: `{"version":2,"outbounds":[],"rules":[],"default_outbound_id":"direct","user_defaults":[{"user_id":42,"outbound_id":"direct"}]}`},
		{Category: "server", Key: "RoutingRevision", Type: "int64", Value: "1"},
		{Category: "server", Key: "RoutingCutover", Type: "string", Value: "complete"},
	}
	if err = db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	server := node.Server{Name: "local", Address: "127.0.0.1", Protocols: `[{"type":"vless","port":443,"enable":true,"transport":"tcp"}]`}
	if err = db.Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	enabled := true
	n := node.Node{Name: "vless", ServerId: server.Id, Protocol: "vless", Enabled: &enabled}
	if err = db.Create(&n).Error; err != nil {
		t.Fatal(err)
	}
	plan := subscribe.Subscribe{Name: "scope", Nodes: fmt.Sprint(n.Id), SpeedLimit: 10, DeviceLimit: 2}
	if err = db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`INSERT INTO user (id,enable) VALUES (42,1)`).Error; err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().Add(time.Hour).UTC()
	if err = db.Exec(`INSERT INTO user_subscribe (id,user_id,subscribe_id,uuid,status,expire_time,traffic,download,upload) VALUES (1,42,?,?,1,?,0,0,0)`, plan.Id, "00000000-0000-4000-8000-000000000001", expiry).Error; err != nil {
		t.Fatal(err)
	}
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer redisClient.Close()
	store := NewStore(db, redisClient)
	p := platform.New(platform.Deps{System: store.System(), Store: store})
	svc := network.New(network.Deps{Store: store, Redis: redisClient, ReadGlobalNodeConfig: p.ReadNodeConfigSnapshot})
	ctx := context.Background()
	get := func() *dto.QueryServerConfigResponse {
		t.Helper()
		v, err := svc.QueryServerProtocolConfig(ctx, &dto.QueryServerConfigRequest{ServerID: server.Id, RoutingVersion: 2})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	first := get()
	if len(first.IdentitySnapshot.Protocols) != 1 || len(first.IdentitySnapshot.Protocols[0].Users) != 1 || first.IdentitySnapshot.Protocols[0].Users[0].AccountID != 42 {
		t.Fatalf("snapshot %+v", first.IdentitySnapshot)
	}
	id := first.IdentitySnapshot.SnapshotID
	handler := nodehttp.QueryServerProtocolConfigHandler(svc, func() string { return "test-secret" })
	fetch := func(accept, capability, etag string) *app.RequestContext {
		c := app.NewContext(0)
		c.Params = param.Params{{Key: "server_id", Value: fmt.Sprint(server.Id)}}
		c.Request.SetRequestURI("/v2/server/1?secret_key=test-secret")
		c.Request.Header.Set("Accept", accept)
		c.Request.Header.Set("X-PPanel-Routing-Version", capability)
		if etag != "" {
			c.Request.Header.Set("If-None-Match", etag)
		}
		handler(ctx, c)
		return c
	}
	etags := map[string]string{}
	for _, accept := range []string{"application/json", "application/protobuf"} {
		response := fetch(accept, "2", "")
		if response.Response.StatusCode() != 200 {
			t.Fatalf("%s HTTP %d %s", accept, response.Response.StatusCode(), response.Response.Body())
		}
		if accept == "application/protobuf" {
			var m serverv1.QueryServerProtocolConfigResponse
			if err := proto.Unmarshal(response.Response.Body(), &m); err != nil || m.Data.IdentitySnapshot.SnapshotId != id || m.Data.IdentitySnapshot.Protocols[0].Users[0].AccountId != 42 {
				t.Fatalf("proto %#v %v", m.Data, err)
			}
		} else {
			var m struct {
				Data dto.QueryServerConfigResponse `json:"data"`
			}
			if err := json.Unmarshal(response.Response.Body(), &m); err != nil || m.Data.IdentitySnapshot.SnapshotID != id {
				t.Fatalf("JSON %#v %v", m.Data.IdentitySnapshot, err)
			}
		}
		etag := string(response.Response.Header.Peek("ETag"))
		etags[accept] = etag
		if etag == "" {
			t.Fatal("missing etag")
		}
		if got := fetch(accept, "2", etag).Response.StatusCode(); got != 304 {
			t.Fatalf("%s cached HTTP %d", accept, got)
		}
		if got := fetch(accept, "1", etag).Response.StatusCode(); got != 426 {
			t.Fatalf("%s old node HTTP %d", accept, got)
		}
	}
	if v := get().IdentitySnapshot.SnapshotID; v != id {
		t.Fatal("unchanged snapshot changed")
	}
	filtered, err := svc.QueryServerProtocolConfig(ctx, &dto.QueryServerConfigRequest{ServerID: server.Id, RoutingVersion: 2, Protocols: []string{"vless"}})
	if err != nil || filtered.IdentitySnapshot.SnapshotID == id {
		t.Fatalf("filter identity %v %v", filtered, err)
	}
	status := &dto.ServerPushStatusRequest{ServerCommon: dto.ServerCommon{ServerId: server.Id}, RoutingVersion: 2, InstanceID: "11111111-1111-4111-8111-111111111111", RoutingSequence: 1, RoutingApplied: &dto.RoutingCandidateIdentity{Mode: "ordered-v2-inherited", Source: "global", SourceRevision: 1, SnapshotID: id}}
	if err = svc.ServerPushStatus(ctx, status); err != nil {
		t.Fatal(err)
	}
	admin, err := svc.GetServerNodeConfig(ctx, &dto.GetServerNodeConfigRequest{ServerID: server.Id})
	if err != nil {
		t.Fatal(err)
	}
	if admin.DesiredSnapshotID == nil || *admin.DesiredSnapshotID != id {
		t.Fatalf("desired snapshot %+v", admin.DesiredSnapshotID)
	}
	status.RoutingSequence = 2
	status.RoutingApplied.SnapshotID = filtered.IdentitySnapshot.SnapshotID
	if err = svc.ServerPushStatus(ctx, status); err != nil {
		t.Fatal(err)
	}
	admin, err = svc.GetServerNodeConfig(ctx, &dto.GetServerNodeConfigRequest{ServerID: server.Id})
	if err != nil {
		t.Fatal(err)
	}
	if admin.DesiredSnapshotID == nil || *admin.DesiredSnapshotID != filtered.IdentitySnapshot.SnapshotID {
		t.Fatal("desired snapshot did not retain reported filter")
	}
	status.RoutingSequence = 3
	status.RoutingApplied.SnapshotID = id
	if err = svc.ServerPushStatus(ctx, status); err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`UPDATE user_subscribe SET expire_time=? WHERE id=1`, expiry.Add(time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	renewed := get().IdentitySnapshot.SnapshotID
	if renewed == id {
		t.Fatal("renewal did not change snapshot")
	}
	for accept, etag := range etags {
		if got := fetch(accept, "2", etag).Response.StatusCode(); got != 200 {
			t.Fatalf("%s stale ETag returned %d after renewal", accept, got)
		}
	}
	admin, err = svc.GetServerNodeConfig(ctx, &dto.GetServerNodeConfigRequest{ServerID: server.Id})
	if err != nil {
		t.Fatal(err)
	}
	if admin.DesiredSnapshotID == nil || *admin.DesiredSnapshotID != renewed {
		t.Fatal("desired did not follow renewal")
	}
	if err = db.Exec(`UPDATE user_subscribe SET uuid=? WHERE id=1`, "00000000-0000-4000-8000-000000000002").Error; err != nil {
		t.Fatal(err)
	}
	rotated := get().IdentitySnapshot.SnapshotID
	if rotated == renewed {
		t.Fatal("UUID rotation did not change snapshot")
	}
	if err = db.Exec(`UPDATE subscribe SET device_limit=4 WHERE id=?`, plan.Id).Error; err != nil {
		t.Fatal(err)
	}
	updated := get()
	if updated.IdentitySnapshot.SnapshotID == rotated || updated.IdentitySnapshot.Protocols[0].Users[0].DeviceLimit != 4 {
		t.Fatal("device limit did not change snapshot")
	}
	rotated = updated.IdentitySnapshot.SnapshotID
	if err = db.Exec(`UPDATE user SET enable=0 WHERE id=42`).Error; err != nil {
		t.Fatal(err)
	}
	disabled := get()
	if len(disabled.IdentitySnapshot.Protocols[0].Users) != 0 || disabled.IdentitySnapshot.SnapshotID == rotated || len(disabled.Routing.UserDefaults) != 1 {
		t.Fatal("disabled account should retain policy but have no authorized identities")
	}
	if err = db.Exec(`UPDATE user SET enable=1 WHERE id=42`).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`UPDATE user_subscribe SET expire_time=? WHERE id=1`, time.Now().Add(-time.Minute).UTC()).Error; err != nil {
		t.Fatal(err)
	}
	expired := get()
	if len(expired.IdentitySnapshot.Protocols[0].Users) != 0 {
		t.Fatal("expired subscription authorized")
	}
	if err = db.Exec(`UPDATE user_subscribe SET expire_time=?,traffic=100,download=80,upload=20 WHERE id=1`, expiry.Add(time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	if got := get(); len(got.IdentitySnapshot.Protocols[0].Users) != 0 {
		t.Fatal("exhausted subscription authorized")
	}
	if err = db.Exec(`UPDATE user_subscribe SET traffic=0 WHERE id=1`).Error; err != nil {
		t.Fatal(err)
	}
	if got := get(); len(got.IdentitySnapshot.Protocols[0].Users) != 1 {
		t.Fatal("renewal did not restore authorization")
	}
	if err = db.Exec(`UPDATE user SET deleted_at=? WHERE id=42`, time.Now().UTC()).Error; err != nil {
		t.Fatal(err)
	}
	if got := get(); len(got.IdentitySnapshot.Protocols[0].Users) != 0 {
		t.Fatal("deleted account authorized")
	}
}
