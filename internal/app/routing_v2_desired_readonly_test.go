package app

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route/param"
	"github.com/perfect-panel/server/internal/module/network"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	adminhttp "github.com/perfect-panel/server/internal/module/network/transport/http/admin/server"
	nodehttp "github.com/perfect-panel/server/internal/module/network/transport/http/server"
	"github.com/perfect-panel/server/internal/module/platform"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestV2AdminDesiredDoesNotActivateOrObserveUndeliveredSnapshot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:v2-desired-readonly-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&system.System{}, &node.Server{}, &node.ServerConfigOverride{}, &node.Node{}, &subscribe.Subscribe{}); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE user (id INTEGER PRIMARY KEY, enable BOOLEAN, deleted_at DATETIME)`,
		`CREATE TABLE user_subscribe (id INTEGER PRIMARY KEY, user_id INTEGER, subscribe_id INTEGER, uuid TEXT, status INTEGER, start_time DATETIME, expire_time DATETIME, traffic INTEGER, download INTEGER, upload INTEGER, updated_at DATETIME)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	settings := []system.System{
		{Category: "server", Key: "Routing", Type: "string", Value: `{"version":2,"outbounds":[],"rules":[],"default_outbound_id":"direct","user_defaults":[]}`},
		{Category: "server", Key: "RoutingRevision", Type: "int64", Value: "1"},
		{Category: "server", Key: "RoutingCutover", Type: "string", Value: "complete"},
	}
	if err := db.Create(&settings).Error; err != nil {
		t.Fatal(err)
	}
	server := node.Server{Name: "local", Address: "127.0.0.1", Protocols: `[{"type":"vless","port":443,"enable":true,"transport":"tcp"}]`}
	if err := db.Create(&server).Error; err != nil {
		t.Fatal(err)
	}
	enabled := true
	n := node.Node{Name: "vless", ServerId: server.Id, Protocol: "vless", Enabled: &enabled}
	if err := db.Create(&n).Error; err != nil {
		t.Fatal(err)
	}
	plan := subscribe.Subscribe{Name: "scope", Nodes: fmt.Sprint(n.Id), SpeedLimit: 10, DeviceLimit: 2}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO user (id,enable) VALUES (42,1),(43,1)`).Error; err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	expiry := start.Add(48 * time.Hour)
	if err := db.Exec(`INSERT INTO user_subscribe (id,user_id,subscribe_id,uuid,status,start_time,expire_time,traffic,download,upload) VALUES (1,42,?,?,1,?,?,0,0,0)`, plan.Id, "00000000-0000-4000-8000-000000000001", start, expiry).Error; err != nil {
		t.Fatal(err)
	}

	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer redisClient.Close()
	store := NewStore(db, redisClient)
	p := platform.New(platform.Deps{System: store.System(), Store: store})
	svc := network.New(network.Deps{Store: store, Redis: redisClient, ReadGlobalNodeConfig: p.ReadNodeConfigSnapshot})
	ctx := context.Background()
	nodeHandler := nodehttp.QueryServerProtocolConfigHandler(svc, func() string { return "test-secret" })
	pull := func(etag string) (*dto.QueryServerConfigResponse, string) {
		t.Helper()
		c := app.NewContext(0)
		c.Params = param.Params{{Key: "server_id", Value: fmt.Sprint(server.Id)}}
		c.Request.SetRequestURI("/v2/server/1?secret_key=test-secret")
		c.Request.Header.Set("X-PPanel-Routing-Version", "2")
		if etag != "" {
			c.Request.Header.Set("If-None-Match", etag)
		}
		nodeHandler(ctx, c)
		if c.Response.StatusCode() != 200 {
			t.Fatalf("node pull: HTTP %d %s", c.Response.StatusCode(), c.Response.Body())
		}
		var envelope struct {
			Code uint32                         `json:"code"`
			Data *dto.QueryServerConfigResponse `json:"data"`
		}
		if err := json.Unmarshal(c.Response.Body(), &envelope); err != nil || envelope.Code != 200 || envelope.Data == nil || envelope.Data.IdentitySnapshot == nil {
			t.Fatalf("node envelope: %s %v", c.Response.Body(), err)
		}
		return envelope.Data, string(c.Response.Header.Peek("ETag"))
	}
	first, etag := pull("")
	firstID := first.IdentitySnapshot.SnapshotID
	if len(first.IdentitySnapshot.Protocols) != 1 || len(first.IdentitySnapshot.Protocols[0].Users) != 1 || etag == "" {
		t.Fatalf("initial snapshot: %+v", first.IdentitySnapshot)
	}
	status := &dto.ServerPushStatusRequest{ServerCommon: dto.ServerCommon{ServerId: server.Id}, RoutingVersion: 2, InstanceID: "11111111-1111-4111-8111-111111111111", RoutingSequence: 1,
		RoutingApplied: &dto.RoutingCandidateIdentity{Mode: "ordered-v2-inherited", Source: "global", SourceRevision: 1, SnapshotID: firstID}}
	if err := svc.ServerPushStatus(ctx, status); err != nil {
		t.Fatal(err)
	}
	adminHandler := adminhttp.GetServerNodeConfigHandler(svc)
	adminGet := func() *dto.GetServerNodeConfigResponse {
		t.Helper()
		c := app.NewContext(0)
		c.Request.SetRequestURI(fmt.Sprintf("/v1/admin/server/node_config?server_id=%d", server.Id))
		adminHandler(ctx, c)
		if c.Response.StatusCode() != 200 {
			t.Fatalf("admin GET: HTTP %d %s", c.Response.StatusCode(), c.Response.Body())
		}
		var envelope struct {
			Code uint32                           `json:"code"`
			Data *dto.GetServerNodeConfigResponse `json:"data"`
		}
		if err := json.Unmarshal(c.Response.Body(), &envelope); err != nil || envelope.Code != 200 || envelope.Data == nil {
			t.Fatalf("admin envelope: %s %v", c.Response.Body(), err)
		}
		return envelope.Data
	}
	if got := adminGet(); got.DesiredSnapshotID == nil || *got.DesiredSnapshotID != firstID {
		t.Fatalf("initial desired: %+v", got.DesiredSnapshotID)
	}

	pendingStart := start.Add(5 * time.Minute)
	pendingExpiry := expiry.Add(24 * time.Hour)
	if err := db.Exec(`INSERT INTO user_subscribe (id,user_id,subscribe_id,uuid,status,start_time,expire_time,traffic,download,upload) VALUES (2,43,?,?,0,?,?,0,0,0)`, plan.Id, "00000000-0000-4000-8000-000000000002", pendingStart, pendingExpiry).Error; err != nil {
		t.Fatal(err)
	}
	type subscriptionTimes struct {
		Status     int
		StartTime  time.Time
		ExpireTime time.Time
	}
	readPending := func() subscriptionTimes {
		t.Helper()
		var row subscriptionTimes
		if err := db.Table("user_subscribe").Select("status,start_time,expire_time").Where("id = ?", 2).Take(&row).Error; err != nil {
			t.Fatal(err)
		}
		return row
	}
	before := readPending()
	for range 2 {
		if got := adminGet(); got.DesiredSnapshotID != nil {
			t.Fatalf("pending desired should be unknown, got %s", *got.DesiredSnapshotID)
		}
		after := readPending()
		if after.Status != before.Status || !after.StartTime.Equal(before.StartTime) || !after.ExpireTime.Equal(before.ExpireTime) {
			t.Fatalf("admin GET changed pending subscription: before=%+v after=%+v", before, after)
		}
	}

	second, _ := pull(etag)
	secondID := second.IdentitySnapshot.SnapshotID
	if secondID == firstID || len(second.IdentitySnapshot.Protocols[0].Users) != 2 {
		t.Fatalf("node pull did not deliver activated identity: %+v", second.IdentitySnapshot)
	}
	afterPull := readPending()
	if afterPull.Status != 1 || !afterPull.StartTime.Equal(before.StartTime) || !afterPull.ExpireTime.Equal(before.ExpireTime) {
		t.Fatalf("node activation changed unexpected fields: %+v", afterPull)
	}
	if got := adminGet(); got.DesiredSnapshotID == nil || *got.DesiredSnapshotID != secondID {
		t.Fatalf("desired after node pull: %+v", got.DesiredSnapshotID)
	}

	// A read-only recalculation may produce a new hash, but it is not a delivery.
	if err := db.Exec(`UPDATE user_subscribe SET expire_time=? WHERE id=2`, pendingExpiry.Add(time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	observed := adminGet()
	if observed.DesiredSnapshotID == nil || *observed.DesiredSnapshotID == secondID {
		t.Fatalf("desired after renewal: %+v", observed.DesiredSnapshotID)
	}
	filterKey := fmt.Sprintf("server:routing-filter:v2:%d:%s", server.Id, *observed.DesiredSnapshotID)
	if exists, err := redisClient.Exists(ctx, filterKey).Result(); err != nil || exists != 0 {
		t.Fatalf("admin GET recorded undelivered hash: exists=%d err=%v", exists, err)
	}
	third, _ := pull("")
	if third.IdentitySnapshot.SnapshotID != *observed.DesiredSnapshotID {
		t.Fatal("node candidate differs from read-only desired")
	}
	if exists, err := redisClient.Exists(ctx, filterKey).Result(); err != nil || exists != 1 {
		t.Fatalf("real node delivery not recorded: exists=%d err=%v", exists, err)
	}
}
