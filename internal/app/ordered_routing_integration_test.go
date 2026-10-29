package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route/param"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/perfect-panel/server/internal/module/network"
	netdto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	adminserverhttp "github.com/perfect-panel/server/internal/module/network/transport/http/admin/server"
	nodehttp "github.com/perfect-panel/server/internal/module/network/transport/http/server"
	"github.com/perfect-panel/server/internal/module/platform"
	platformdto "github.com/perfect-panel/server/internal/module/platform/contract"
	adminsystemhttp "github.com/perfect-panel/server/internal/module/platform/transport/http/admin/system"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOrderedAuthoritativeSnapshotMySQL(t *testing.T) {
	routingIntegration(t, "mysql", os.Getenv("PPANEL_TEST_ORDERED_ROUTING_MYSQL_DSN"))
}
func TestOrderedAuthoritativeSnapshotPostgres(t *testing.T) {
	routingIntegration(t, "postgres", os.Getenv("PPANEL_TEST_ORDERED_ROUTING_POSTGRES_DSN"))
}
func routingIntegration(t *testing.T, dialect, dsn string) {
	t.Helper()
	if dsn == "" {
		t.Skip("set dedicated loopback ordered-routing test DSN")
	}
	var address, name string
	if dialect == "mysql" {
		cfg, err := mysqlDriver.ParseDSN(dsn)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Net != "tcp" {
			t.Fatal("TCP required")
		}
		address, name = cfg.Addr, cfg.DBName
	} else {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		address, name = u.Host, strings.TrimPrefix(u.Path, "/")
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		t.Fatal("non-loopback database refused")
	}
	if !strings.HasSuffix(name, "_test") {
		t.Fatal("dedicated _test database required")
	}
	mini := miniredis.RunT(t)
	rds := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer rds.Close()
	open := func(rds *redis.Client) *repository.GormStore {
		var dialector gorm.Dialector
		if dialect == "mysql" {
			dialector = mysql.Open(dsn)
		} else {
			dialector = postgres.Open(dsn)
		}
		db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal(err)
		}
		pool, _ := db.DB()
		t.Cleanup(func() { pool.Close() })
		return NewStore(db, rds)
	}
	a, b := open(rds), open(rds)
	ctx := context.Background()
	policy := `{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct"}`
	if err := a.InPlatformTx(ctx, func(tx repository.PlatformStore) error {
		for key, value := range map[string]string{"Routing": policy, "RoutingRevision": "1", "RoutingCutover": "complete", "Block": `["geosite:dormant"]`, "NodeSecret": "known-test-secret", "NodePullInterval": "61", "NodePushInterval": "62", "TrafficReportThreshold": "63", "DNS": `[{"proto":"udp","address":"1.1.1.1","domains":[]}]`} {
			typ := "string"
			if key == "RoutingRevision" || key == "NodePullInterval" || key == "NodePushInterval" || key == "TrafficReportThreshold" {
				typ = "int64"
			}
			if err := tx.System().UpdateValueByCategoryKey(ctx, "server", key, value, typ); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	p1 := platform.New(platform.Deps{System: a.System(), Store: a})
	p2 := platform.New(platform.Deps{System: b.System(), Store: b})
	server := &node.Server{Name: "snapshot fixture", Address: "127.0.0.1", Protocols: "[]"}
	if err := a.Node().InsertServer(ctx, server); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = a.Node().DeleteServerConfigOverride(ctx, server.Id)
		_ = a.Node().DeleteServer(ctx, server.Id)
	}()
	makeNetwork := func(store *repository.GormStore, p platform.Service) network.Service {
		return network.New(network.Deps{Store: store, Redis: rds, ReadGlobalNodeConfig: p.ReadNodeConfigSnapshot, Config: func() network.Snapshot { panic("routing used process config") }})
	}
	n1, n2 := makeNetwork(a, p1), makeNetwork(b, p2)
	// Real handler -> module -> database: return the committed ID, then
	// verify it can be used immediately for the copy workflow.
	create := func(body string) *app.RequestContext {
		r := app.NewContext(0)
		r.Request.Header.SetContentTypeBytes([]byte("application/json"))
		r.Request.SetBodyString(body)
		adminserverhttp.CreateServerHandler(n1)(ctx, r)
		return r
	}
	created := create(`{"name":"copy target","address":"127.0.0.1","country":"local","city":"local","protocols":[]}`)
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Response.Body(), &envelope); err != nil || envelope.Code != 200 || envelope.Data.ID <= 0 {
		t.Fatalf("create response %s: %v", created.Response.Body(), err)
	}
	createdID := envelope.Data.ID
	defer func() {
		_ = a.Node().DeleteServerConfigOverride(ctx, createdID)
		_ = a.Node().DeleteServer(ctx, createdID)
	}()
	stored, err := a.Node().FindOneServer(ctx, createdID)
	if err != nil || stored.Id != createdID {
		t.Fatalf("committed server missing: %#v %v", stored, err)
	}
	// Actual handler -> service -> DB: no enabled invalid listener may be
	// created, updated, or distributed as a fake zero-protocol candidate.
	invalidProtocols := []string{`{"type":"vless","enable":true,"port":0,"transport":"tcp"}`, `{"type":"naive","enable":true,"port":443}`, `{"type":"shadowsocks","enable":true,"port":443,"cipher":"aes-128-gcm","plugin":"obfs"}`, `{"type":"vless","enable":true,"port":443,"transport":"xhttp","xhttp_mode":"bogus"}`, `{"type":"vless","enable":true,"port":443,"transport":"xhttp","xhttp_extra":"{\"xmux\":\"oops\"}"}`}
	for _, protocol := range invalidProtocols {
		failed := create(fmt.Sprintf(`{"name":"bad protocol","address":"127.0.0.1","country":"local","city":"local","protocols":[%s]}`, protocol))
		if failed.Response.StatusCode() != 422 || !strings.Contains(string(failed.Response.Body()), `"code":140004`) {
			t.Fatalf("create did not reject protocol: %d %s", failed.Response.StatusCode(), failed.Response.Body())
		}
		request := app.NewContext(0)
		request.Request.Header.SetContentTypeBytes([]byte("application/json"))
		request.Request.SetBodyString(fmt.Sprintf(`{"id":%d,"name":"bad update","address":"127.0.0.1","country":"local","city":"local","protocols":[%s]}`, createdID, protocol))
		adminserverhttp.UpdateServerHandler(n1)(ctx, request)
		if request.Response.StatusCode() != 422 || !strings.Contains(string(request.Response.Body()), `"code":140004`) {
			t.Fatalf("update did not reject protocol: %d %s", request.Response.StatusCode(), request.Response.Body())
		}
		current, err := a.Node().FindOneServer(ctx, createdID)
		if err != nil || current.Protocols != "[]" || current.Name != "copy target" {
			t.Fatalf("invalid update mutated server: %#v %v", current, err)
		}
		current.Protocols = "[" + protocol + "]"
		if err := a.Node().UpdateServer(ctx, current); err != nil {
			t.Fatal(err)
		}
		if got, err := n1.QueryServerProtocolConfig(ctx, &netdto.QueryServerConfigRequest{ServerID: createdID}); err == nil {
			t.Fatalf("invalid enabled listener delivered: %#v", got)
		}
		nodeRequest := app.NewContext(0)
		nodeRequest.Params = param.Params{{Key: "server_id", Value: fmt.Sprint(createdID)}}
		nodeRequest.Request.SetRequestURI(fmt.Sprintf("/v2/server/%d?secret_key=known-test-secret", createdID))
		nodeRequest.Request.Header.Set("X-PPanel-Routing-Version", "1")
		nodehttp.QueryServerProtocolConfigHandler(n1, func() string { return "known-test-secret" })(ctx, nodeRequest)
		if strings.Contains(string(nodeRequest.Response.Body()), `"code":200`) || strings.Contains(string(nodeRequest.Response.Body()), `"protocols":[]`) {
			t.Fatalf("invalid listener produced successful zero protocols: %s", nodeRequest.Response.Body())
		}
		current.Protocols = "[]"
		if err := a.Node().UpdateServer(ctx, current); err != nil {
			t.Fatal(err)
		}
	}
	disabled, err := a.Node().FindOneServer(ctx, createdID)
	if err != nil {
		t.Fatal(err)
	}
	disabled.Protocols = `[{"type":"vless","enable":false,"port":0}]`
	if err := a.Node().UpdateServer(ctx, disabled); err != nil {
		t.Fatal(err)
	}
	disabledResult, err := n1.QueryServerProtocolConfig(ctx, &netdto.QueryServerConfigRequest{ServerID: createdID})
	if err != nil || disabledResult.Protocols == nil || len(disabledResult.Protocols) != 0 {
		t.Fatalf("explicitly disabled listener not zero: %#v %v", disabledResult, err)
	}
	disabled.Protocols = "[]"
	if err := a.Node().UpdateServer(ctx, disabled); err != nil {
		t.Fatal(err)
	}
	cacheKey := fmt.Sprintf("%s%d:vless", node.ServerUserListCacheKey, createdID)
	if err := rds.Set(ctx, cacheKey, "authorization-cache", node.ServerCacheTTL).Err(); err != nil {
		t.Fatal(err)
	}
	validUpdate := app.NewContext(0)
	validUpdate.Request.Header.SetContentTypeBytes([]byte("application/json"))
	validUpdate.Request.SetBodyString(fmt.Sprintf(`{"id":%d,"name":"copy target","address":"127.0.0.1","country":"local","city":"local","protocols":[]}`, createdID))
	adminserverhttp.UpdateServerHandler(n1)(ctx, validUpdate)
	if validUpdate.Response.StatusCode() != 200 || !strings.Contains(string(validUpdate.Response.Body()), `"code":200`) {
		t.Fatalf("valid server update: %s", validUpdate.Response.Body())
	}
	if cached, err := rds.Get(ctx, cacheKey).Result(); err != nil || cached != "authorization-cache" {
		t.Fatalf("server-only update touched unrelated user cache: %q %v", cached, err)
	}
	if ttl, err := rds.TTL(ctx, cacheKey).Result(); err != nil || ttl <= 0 || ttl > node.ServerCacheTTL {
		t.Fatalf("cache TTL %v %v", ttl, err)
	}
	initial, err := n1.GetServerNodeConfig(ctx, &netdto.GetServerNodeConfigRequest{ServerID: createdID})
	if err != nil || initial.RoutingStatus.Known || initial.RoutingStatus.Report != nil {
		t.Fatalf("new server must have unknown status: %#v %v", initial, err)
	}
	// Copy the complete desired configuration before any node has reported.
	updateCopy := func(body string) *app.RequestContext {
		r := app.NewContext(0)
		r.Request.Header.SetContentTypeBytes([]byte("application/json"))
		r.Request.SetBodyString(body)
		adminserverhttp.UpdateServerNodeConfigHandler(n1)(ctx, r)
		return r
	}
	source := &node.Server{Name: "copy source", Address: "127.0.0.1", Protocols: "[]"}
	if err := a.Node().InsertServer(ctx, source); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = a.Node().DeleteServerConfigOverride(ctx, source.Id)
		_ = a.Node().DeleteServer(ctx, source.Id)
	}()
	sourceBody := fmt.Sprintf(`{"server_id":%d,"inherit_ip_strategy":false,"ip_strategy":"prefer_ipv4","inherit_dns":false,"dns":[{"proto":"udp","address":"9.9.9.9","domains":[]}],"inherit_routing":false,"routing_revision":0,"routing":%s}`, source.Id, policy)
	sourceResponse := updateCopy(sourceBody)
	if sourceResponse.Response.StatusCode() != 200 || !strings.Contains(string(sourceResponse.Response.Body()), `"code":200`) {
		t.Fatalf("offline source save failed: %s", sourceResponse.Response.Body())
	}
	sourceConfig, err := n1.GetServerNodeConfig(ctx, &netdto.GetServerNodeConfigRequest{ServerID: source.Id})
	if err != nil || sourceConfig.RoutingStatus.Known || sourceConfig.RoutingStatus.Report != nil || sourceConfig.Override.Routing == nil {
		t.Fatalf("offline source readback: %#v %v", sourceConfig, err)
	}
	// A copy takes the actual source content, but the new server starts with
	// its own independent revision 0, not the source's revision.
	copyJSON, err := json.Marshal(map[string]any{"server_id": createdID, "inherit_ip_strategy": sourceConfig.Override.InheritIPStrategy, "ip_strategy": sourceConfig.Override.IPStrategy, "inherit_dns": sourceConfig.Override.InheritDNS, "dns": sourceConfig.Override.DNS, "inherit_routing": sourceConfig.Override.InheritRouting, "routing_revision": 0, "routing": sourceConfig.Override.Routing})
	if err != nil {
		t.Fatal(err)
	}
	copyBody := string(copyJSON)
	copyResponse := updateCopy(copyBody)
	if copyResponse.Response.StatusCode() != 200 || !strings.Contains(string(copyResponse.Response.Body()), `"code":200`) {
		t.Fatalf("offline copy failed: %d %s", copyResponse.Response.StatusCode(), copyResponse.Response.Body())
	}
	copied, err := n1.GetServerNodeConfig(ctx, &netdto.GetServerNodeConfigRequest{ServerID: createdID})
	if err != nil || copied.Override.InheritRouting || copied.Override.RoutingRevision != 1 || copied.Effective.RoutingMode != "local-v1" || copied.Override.IPStrategy != "prefer_ipv4" || len(copied.Override.DNS) != 1 || copied.Override.DNS[0].Address != "9.9.9.9" || copied.RoutingStatus.Known || copied.RoutingStatus.Report != nil {
		t.Fatalf("offline copy readback: %#v %v", copied, err)
	}
	sourceRouting, _ := json.Marshal(sourceConfig.Override.Routing)
	copiedRouting, _ := json.Marshal(copied.Override.Routing)
	if string(sourceRouting) != string(copiedRouting) {
		t.Fatalf("copy changed routing content: source=%s copy=%s", sourceRouting, copiedRouting)
	}
	staleCopy := updateCopy(copyBody)
	if staleCopy.Response.StatusCode() != 409 || !strings.Contains(string(staleCopy.Response.Body()), `"code":140002`) {
		t.Fatalf("offline stale copy did not conflict: %d %s", staleCopy.Response.StatusCode(), staleCopy.Response.Body())
	}
	invalidReference := updateCopy(fmt.Sprintf(`{"server_id":%d,"routing_revision":1,"routing":{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"missing"}}`, createdID))
	if strings.Contains(string(invalidReference.Response.Body()), `"code":200`) {
		t.Fatalf("invalid reference accepted: %s", invalidReference.Response.Body())
	}
	unchanged, err := n1.GetServerNodeConfig(ctx, &netdto.GetServerNodeConfigRequest{ServerID: createdID})
	if err != nil || unchanged.Override.RoutingRevision != 1 || unchanged.Override.InheritRouting {
		t.Fatalf("invalid request changed copy: %#v %v", unchanged, err)
	}
	inheritResponse := updateCopy(fmt.Sprintf(`{"server_id":%d,"inherit_routing":true,"routing_revision":1}`, createdID))
	if inheritResponse.Response.StatusCode() != 200 || !strings.Contains(string(inheritResponse.Response.Body()), `"code":200`) {
		t.Fatalf("offline inheritance failed: %d %s", inheritResponse.Response.StatusCode(), inheritResponse.Response.Body())
	}
	inherited, err := n1.GetServerNodeConfig(ctx, &netdto.GetServerNodeConfigRequest{ServerID: createdID})
	if err != nil || !inherited.Override.InheritRouting || inherited.Override.RoutingRevision != 2 || inherited.Effective.RoutingMode != "global-v1" || inherited.Effective.Routing.LocalTransitionRevision != 2 || inherited.Override.IPStrategy != "prefer_ipv4" || len(inherited.Override.DNS) != 1 || inherited.RoutingStatus.Known || inherited.RoutingStatus.Report != nil {
		t.Fatalf("offline inheritance readback: %#v %v", inherited, err)
	}
	before, err := a.Node().QueryServerSorts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tooLong := fmt.Sprintf(`{"name":%q,"address":"127.0.0.1","country":"local","city":"local","protocols":[]}`, strings.Repeat("x", 101))
	for _, badBody := range []string{`{"name":"invalid copy","address":"127.0.0.1","country":"local","city":"local","protocols":[{"type":""}]}`, `{"name":"invalid copy","address":"127.0.0.1","country":"local","city":"local","protocols":[{"type":""}]}`, tooLong, tooLong, `  {"name":"ambiguous","protocols":[{"type":"vless","security":"none","Security":"tls"}]}`} {
		failed := create(badBody)
		var bad struct {
			Code int             `json:"code"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(failed.Response.Body(), &bad); err != nil || bad.Code == 200 || len(bad.Data) > 0 && string(bad.Data) != "null" {
			t.Fatalf("failed create reported success/ID: %s %v", failed.Response.Body(), err)
		}
	}
	after, err := a.Node().QueryServerSorts(ctx)
	if err != nil || len(after) != len(before) {
		t.Fatalf("repeated failed create changed DB: %d -> %d %v", len(before), len(after), err)
	}
	dnsOverride := `[{"proto":"udp","address":"8.8.8.8","domains":[]}]`
	if err := a.Node().SaveServerRoutingOverride(ctx, &node.ServerConfigOverride{ServerId: server.Id, DNS: &dnsOverride}); err != nil {
		t.Fatal(err)
	}
	// Exercise real handlers, service methods and DB without relying on the
	// caller to perform a prior content-type or presence check.
	for _, tc := range []struct{ contentType, body string }{{"", ""}, {"application/json", ""}, {"application/x-www-form-urlencoded", "node_secret=overwritten&node_push_interval=0"}, {"text/plain", `{"node_secret":"overwritten"}`}, {"application/json", `null`}, {"application/json", `{"routing_revision":null}`}, {"application/json", `{"node_pull_interval":-1}`}, {"application/json", `{"node_push_interval":86401}`}, {"application/json", `{"node_push_interval":9223372036854775807}`}, {"application/json", `{"dns":[{"proto":"tls","address":"1.1.1.1","domains":[]}]}`}, {"application/json", `{"dns":[{"address":"1.1.1.1","domains":["regexp:("]}]}`}, {"application/json", `{"routing_revision":1,"routing":{"version":1,"outbounds":[{"id":"ob_11111111-1111-4111-8111-111111111111","name":"unsafe","protocol":"direct","spider_x":"/?p=4097"}],"rules":[],"default_outbound_id":"direct"}}`}} {
		request := app.NewContext(0)
		request.Request.Header.SetContentTypeBytes([]byte(tc.contentType))
		request.Request.SetRequestURI("/v1/admin/system/node_config?node_secret=overwritten&node_push_interval=0")
		request.Request.SetBodyString(tc.body)
		adminsystemhttp.UpdateNodeConfigHandler(p1)(ctx, request)
		if request.Response.StatusCode() == 200 && !strings.Contains(string(request.Response.Body()), `"code":400`) {
			t.Fatalf("unsafe global request accepted: %s", request.Response.Body())
		}
	}
	for _, tc := range []struct{ contentType, body string }{{"", ""}, {"application/json", ""}, {"application/x-www-form-urlencoded", fmt.Sprintf("server_id=%d&inherit_dns=true", server.Id)}, {"text/plain", fmt.Sprintf(`{"server_id":%d}`, server.Id)}, {"application/json", fmt.Sprintf(`{"server_id":%d,"routing_revision":null}`, server.Id)}, {"application/json", fmt.Sprintf(`{"server_id":%d,"inherit_dns":false,"dns":[{"address":"https://resolver.example/dns-query","domains":[]}]}`, server.Id)}, {"application/json", fmt.Sprintf(`{"server_id":%d,"routing_revision":0,"routing":{"version":1,"outbounds":[{"id":"ob_11111111-1111-4111-8111-111111111111","name":"unsafe","protocol":"direct","spider_x":"/?c=9"}],"rules":[],"default_outbound_id":"direct"}}`, server.Id)}} {
		request := app.NewContext(0)
		request.Request.Header.SetContentTypeBytes([]byte(tc.contentType))
		request.Request.SetRequestURI(fmt.Sprintf("/v1/admin/server/node_config/update?server_id=%d&inherit_dns=true", server.Id))
		request.Request.SetBodyString(tc.body)
		adminserverhttp.UpdateServerNodeConfigHandler(n1)(ctx, request)
		if request.Response.StatusCode() == 200 && !strings.Contains(string(request.Response.Body()), `"code":400`) {
			t.Fatalf("unsafe server request accepted: %s", request.Response.Body())
		}
	}
	globalAfter, err := p1.GetNodeConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if globalAfter.NodeSecret != "known-test-secret" || globalAfter.NodePullInterval != 61 || globalAfter.NodePushInterval != 62 || globalAfter.TrafficReportThreshold != 63 || globalAfter.RoutingRevision != 1 || len(globalAfter.DNS) != 1 || globalAfter.DNS[0].Address != "1.1.1.1" {
		t.Fatalf("invalid request changed global settings %#v", globalAfter)
	}
	localAfter, err := a.Node().FindServerRoutingOverride(ctx, server.Id)
	if err != nil || localAfter == nil || localAfter.DNS == nil || *localAfter.DNS != dnsOverride || localAfter.RoutingRevision != 0 {
		t.Fatalf("invalid request changed server settings %#v %v", localAfter, err)
	}
	update := func(p platform.Service, def string) error {
		var req platformdto.NodeConfig
		if err := json.Unmarshal([]byte(fmt.Sprintf(`{"routing_revision":1,"routing":{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"%s"}}`, def)), &req); err != nil {
			return err
		}
		return p.UpdateNodeConfig(ctx, &req)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i, p := range []platform.Service{p1, p2} {
		wg.Add(1)
		go func(p platform.Service, i int) {
			defer wg.Done()
			def := "direct"
			if i == 1 {
				def = "block"
			}
			errs <- update(p, def)
		}(p, i)
	}
	wg.Wait()
	close(errs)
	ok, conflict := 0, 0
	for err := range errs {
		if err == nil {
			ok++
			continue
		}
		var c *xerr.CodeError
		if errors.As(errors.Cause(err), &c) && c.GetErrCode() == xerr.PlatformRoutingRevisionConflict {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("concurrency success=%d conflict=%d", ok, conflict)
	}
	for _, svc := range []network.Service{n1, n2} {
		got, err := svc.QueryServerProtocolConfig(ctx, &netdto.QueryServerConfigRequest{ServerID: server.Id})
		if err != nil {
			t.Fatal(err)
		}
		if got.Routing == nil || got.Routing.Mode != "ordered-v1-inherited" || got.Routing.SourceRevision != 2 || got.Protocols == nil {
			t.Fatalf("snapshot %#v", got)
		}
	}
	// No node status is reported; revision CAS must still authorize the save.
	var req netdto.UpdateServerNodeConfigRequest
	body := fmt.Sprintf(`{"server_id":%d,"routing_revision":0,"routing":%s}`, server.Id, policy)
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	if err := n1.UpdateServerNodeConfig(ctx, &req); err != nil {
		t.Fatal(err)
	}
	statusAfterSave, err := n1.GetServerNodeConfig(ctx, &netdto.GetServerNodeConfigRequest{ServerID: server.Id})
	if err != nil || statusAfterSave.RoutingStatus.Known || statusAfterSave.RoutingStatus.Report != nil {
		t.Fatalf("offline save fabricated status: %#v %v", statusAfterSave, err)
	}
	for _, svc := range []network.Service{n1, n2} {
		got, err := svc.QueryServerProtocolConfig(ctx, &netdto.QueryServerConfigRequest{ServerID: server.Id})
		if err != nil {
			t.Fatal(err)
		}
		if got.Routing == nil || got.Routing.Mode != "ordered-v1-local" || got.Routing.SourceRevision != 1 {
			t.Fatalf("local %#v", got)
		}
	}
	// Competing local writes use the network owner's row lock and revision;
	// they do not need a platform/network transaction or advisory lock.
	localErrs := make(chan error, 2)
	wg = sync.WaitGroup{}
	for i, svc := range []network.Service{n1, n2} {
		wg.Add(1)
		go func(svc network.Service, i int) {
			defer wg.Done()
			def := "direct"
			if i == 1 {
				def = "block"
			}
			var r netdto.UpdateServerNodeConfigRequest
			payload := fmt.Sprintf(`{"server_id":%d,"routing_revision":1,"routing":{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"%s"}}`, server.Id, def)
			if err := json.Unmarshal([]byte(payload), &r); err != nil {
				localErrs <- err
				return
			}
			localErrs <- svc.UpdateServerNodeConfig(ctx, &r)
		}(svc, i)
	}
	wg.Wait()
	close(localErrs)
	ok, conflict = 0, 0
	for err := range localErrs {
		if err == nil {
			ok++
			continue
		}
		var c *xerr.CodeError
		if errors.As(errors.Cause(err), &c) && c.GetErrCode() == xerr.NetworkRoutingRevisionConflict {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("local concurrency success=%d conflict=%d", ok, conflict)
	}
	for _, svc := range []network.Service{n1, n2} {
		got, err := svc.QueryServerProtocolConfig(ctx, &netdto.QueryServerConfigRequest{ServerID: server.Id})
		if err != nil {
			t.Fatal(err)
		}
		if got.Routing == nil || got.Routing.Mode != "ordered-v1-local" || got.Routing.SourceRevision != 2 {
			t.Fatalf("local revision %#v", got)
		}
	}
	// Override writes have no dependency on the node user-list cache. Redis
	// loss cannot turn this authoritative DB write into a post-commit failure.
	transient := miniredis.RunT(t)
	unstable := redis.NewClient(&redis.Options{Addr: transient.Addr(), MaxRetries: -1, DialTimeout: 100 * time.Millisecond, ReadTimeout: 100 * time.Millisecond, WriteTimeout: 100 * time.Millisecond})
	defer unstable.Close()
	txStore := open(unstable)
	outageSvc := network.New(network.Deps{Store: txStore, Redis: unstable, ReadGlobalNodeConfig: p1.ReadNodeConfigSnapshot})
	userCacheKey := fmt.Sprintf("%s%d:vless", node.ServerUserListCacheKey, server.Id)
	if err := unstable.Set(ctx, userCacheKey, "unrelated-authorized-users", node.ServerCacheTTL).Err(); err != nil {
		t.Fatal(err)
	}
	transient.Close()
	request := app.NewContext(0)
	request.Request.Header.SetContentTypeBytes([]byte("application/json"))
	request.Request.SetBodyString(fmt.Sprintf(`{"server_id":%d,"inherit_dns":false,"dns":[{"proto":"udp","address":"9.9.9.9","domains":[]}]}`, server.Id))
	adminserverhttp.UpdateServerNodeConfigHandler(outageSvc)(ctx, request)
	if !strings.Contains(string(request.Response.Body()), `"code":200`) {
		t.Fatalf("committed write misreported as failure: %s", request.Response.Body())
	}
	readback, err := outageSvc.GetServerNodeConfig(ctx, &netdto.GetServerNodeConfigRequest{ServerID: server.Id})
	if err != nil || len(readback.Override.DNS) != 1 || readback.Override.DNS[0].Address != "9.9.9.9" {
		t.Fatalf("authoritative readback failed: %#v %v", readback, err)
	}
	if err := transient.Restart(); err != nil {
		t.Fatal(err)
	}
	if value, err := unstable.Get(ctx, userCacheKey).Result(); err != nil || value != "unrelated-authorized-users" {
		t.Fatalf("unrelated user cache changed: %q %v", value, err)
	}
	if ttl, err := unstable.TTL(ctx, userCacheKey).Result(); err != nil || ttl <= 0 || ttl > node.ServerCacheTTL {
		t.Fatalf("user cache TTL %v %v", ttl, err)
	}
	if generation, err := txStore.Node().ServerCacheGeneration(ctx, server.Id); err != nil || generation != 0 {
		t.Fatalf("override invalidated unrelated cache: %d %v", generation, err)
	}
}
