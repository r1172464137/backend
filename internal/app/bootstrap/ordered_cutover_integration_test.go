package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/billing"
	"github.com/perfect-panel/server/internal/module/identity"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/notification"
	"github.com/perfect-panel/server/internal/module/platform"
	"github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/internal/module/support"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/orm"
	"github.com/redis/go-redis/v9"
)

func TestOrderedCutoverMySQL(t *testing.T) {
	runCutoverIntegration(t, "mysql", os.Getenv("PPANEL_TEST_ORDERED_ROUTING_MYSQL_DSN"))
}
func TestOrderedCutoverPostgres(t *testing.T) {
	runCutoverIntegration(t, "postgres", os.Getenv("PPANEL_TEST_ORDERED_ROUTING_POSTGRES_DSN"))
}
func runCutoverIntegration(t *testing.T, dialect, dsn string) {
	t.Helper()
	if dsn == "" {
		t.Skip("dedicated loopback cutover database required")
	}
	// Proof tools are mandatory and exercised before touching any fixture or
	// migration. A missing/broken helper must not leave a changed database.
	requireNodeProof(t)
	if proof := runNodeProof(t, map[string]any{"kind": "spider_uri", "uri": "/"}); !proof.OK {
		t.Fatalf("node proof precheck failed: %s", proof.Error)
	}
	xray := os.Getenv("PPANEL_TEST_XRAY")
	if xray == "" {
		t.Fatal("PPANEL_TEST_XRAY is required before cutover fixtures")
	}
	if _, err := exec.LookPath(xray); err != nil {
		t.Fatalf("xray precheck: %v", err)
	}
	var addr, name string
	if dialect == "mysql" {
		cfg, err := mysqlDriver.ParseDSN(dsn)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Net != "tcp" {
			t.Fatal("TCP required")
		}
		addr, name = cfg.Addr, cfg.DBName
	} else {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		addr, name = u.Host, strings.TrimPrefix(u.Path, "/")
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		t.Fatal("non-loopback refused")
	}
	if !strings.HasSuffix(name, "_test") {
		t.Fatal("dedicated _test DB required")
	}
	ctx := context.Background()
	cfg := orm.ParseDSN(dsn)
	if cfg == nil {
		t.Fatal("invalid DSN")
	}
	cfg.Driver = dialect
	version, exists, err := schemaVersion(ctx, *cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !exists || version != 2179 {
		t.Fatalf("expected dedicated genuine 02179 fixture; never downgrade a 02180 DB, got %d", version)
	}
	db, err := orm.ConnectDatabase(orm.Mysql{Config: *cfg})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	defer pool.Close()
	mini := miniredis.RunT(t)
	rds := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer rds.Close()
	store := repository.NewGormStoreWithBuilders(db, rds, repository.Builders{Platform: platform.NewRepoBuilder(), Network: network.NewRepoBuilder(rds), Billing: billing.NewRepoBuilder(), Subscription: subscription.NewRepoBuilder(), Identity: identity.NewRepoBuilder(), Support: support.NewRepoBuilder(), Notification: notification.NewRepoBuilder()})
	set := func(key, value string) {
		t.Helper()
		if err := store.System().UpdateValueByCategoryKey(ctx, "server", key, value, "string"); err != nil {
			t.Fatal(err)
		}
	}
	set("Block", `["full:global.example"]`)
	set("BlockIPs", `["10.0.0.0/8"]`)
	set("BlockProtocols", `["bittorrent"]`)
	set("Outbound", `[{"name":"Default","protocol":"direct","rules":["geosite:example"]}]`)
	server := &node.Server{Name: "cutover fixture", Address: "127.0.0.1", Protocols: "[]"}
	if err := store.Node().InsertServer(ctx, server); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = store.Node().DeleteServerConfigOverride(ctx, server.Id)
		_ = store.Node().DeleteServer(ctx, server.Id)
	}()
	if err := db.Table("server_config_overrides").Create(map[string]any{"server_id": server.Id, "block": `["full:local.example"]`, "dns": `[]`}).Error; err != nil {
		t.Fatal(err)
	}
	c := config.Config{}
	c.SetDatabaseConfig(*cfg)
	c.Administrator.Email = "local-cutover@example.invalid"
	c.Administrator.Password = "local-test-only"
	deps := &Dependencies{Config: func() config.Config { return c }, Store: store}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("conflicting tag did not stop cutover")
			}
		}()
		Migrate(deps)
	}()
	version, exists, err = schemaVersion(ctx, *cfg)
	if err != nil || !exists || version != 2179 {
		t.Fatalf("preflight modified migration version: %d %v %v", version, exists, err)
	}
	if db.Migrator().HasColumn("server_config_overrides", "routing") {
		t.Fatal("preflight performed DDL")
	}
	set("Outbound", `[{"name":"egress","protocol":"socks","address":"127.0.0.1","port":1080,"rules":["full:route.example"],"ip_rules":["10.0.0.0/8"],"protocol_rules":["bittorrent"]}]`)
	if err := db.Table("server_config_overrides").Where("server_id = ?", server.Id).Update("outbound", `[{"name":"dns_out","protocol":"direct"}]`).Error; err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("local conflicting tag did not stop cutover")
			}
		}()
		Migrate(deps)
	}()
	version, exists, err = schemaVersion(ctx, *cfg)
	if err != nil || !exists || version != 2179 || db.Migrator().HasColumn("server_config_overrides", "routing") {
		t.Fatalf("local blocker occurred after DDL: version=%d err=%v", version, err)
	}
	if err := db.Table("server_config_overrides").Where("server_id = ?", server.Id).Update("outbound", nil).Error; err != nil {
		t.Fatal(err)
	}
	assertBlockedBeforeDDL := func(label string) {
		t.Helper()
		blocked := false
		func() { defer func() { blocked = recover() != nil }(); Migrate(deps) }()
		if !blocked {
			t.Fatalf("%s was not blocked", label)
		}
		v, exists, err := schemaVersion(ctx, *cfg)
		if err != nil || !exists || v != 2179 || db.Migrator().HasColumn("server_config_overrides", "routing") {
			t.Fatalf("%s blocker after DDL: %d %v", label, v, err)
		}
	}
	for _, bad := range []struct{ name, protocols string }{
		{"unsupported listener", `[{"type":"naive","port":443,"enable":true}]`},
		{"shadowsocks plugin", `[{"type":"shadowsocks","port":443,"enable":true,"cipher":"aes-128-gcm","plugin":"obfs"}]`},
		{"unsupported UoT", `[{"type":"shadowsocks","port":443,"enable":true,"cipher":"aes-128-gcm","uot_version":2}]`},
		{"case-folded protocol field", `[{"type":"vless","port":443,"enable":true,"transport":"tcp","security":"none","Security":"tls"}]`},
		{"XHTTP mode", `[{"type":"vless","port":443,"enable":true,"transport":"xhttp","xhttp_mode":"bogus"}]`},
		{"XHTTP xmux type", `[{"type":"vless","port":443,"enable":true,"transport":"xhttp","xhttp_extra":"{\"xmux\":\"oops\"}"}]`},
	} {
		if err := db.Table("servers").Where("id = ?", server.Id).Update("protocols", bad.protocols).Error; err != nil {
			t.Fatal(err)
		}
		assertBlockedBeforeDDL(bad.name)
	}
	if err := db.Table("servers").Where("id = ?", server.Id).Update("protocols", `[]`).Error; err != nil {
		t.Fatal(err)
	}
	set("Outbound", `[{"name":"tuic","protocol":"tuic","address":"127.0.0.1","port":443,"uuid":"00000000-0000-0000-0000-000000000001","password":"secret"}]`)
	assertBlockedBeforeDDL("TUIC outbound")
	set("Outbound", `[{"name":"bad-spider","protocol":"vless","address":"example.com","port":443,"uuid":"00000000-0000-0000-0000-000000000001","security":"reality","transport":"tcp","reality_public_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","spider_x":"/%"}]`)
	assertBlockedBeforeDDL("malformed REALITY spider")
	set("Outbound", `[{"name":"bad-spider-number","protocol":"vless","address":"example.com","port":443,"uuid":"00000000-0000-0000-0000-000000000001","security":"reality","transport":"tcp","reality_public_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","spider_x":"/?p=4097"}]`)
	assertBlockedBeforeDDL("REALITY spider numeric limit")
	set("Outbound", `[{"name":"raw-uot","protocol":"blackhole","settings":"{\"response\":{\"type\":\"none\",\"uot\":true}}"}]`)
	assertBlockedBeforeDDL("unconsumed raw UoT")
	set("Outbound", `[{"name":"egress","protocol":"socks","address":"127.0.0.1","port":1080,"rules":["full:route.example"],"ip_rules":["10.0.0.0/8"],"protocol_rules":["bittorrent"]}]`)
	set("DNS", `[{"proto":"tls","address":"1.1.1.1","domains":[]}]`)
	assertBlockedBeforeDDL("global DNS protocol")
	set("DNS", `[{"proto":"udp","address":"1.1.1.1","domains":["regexp:("]}]`)
	assertBlockedBeforeDDL("global DNS invalid regexp")
	set("DNS", `[]`)
	if err := db.Table("server_config_overrides").Where("server_id = ?", server.Id).Update("dns", `[{"address":"https://resolver.example/dns-query","domains":[]}]`).Error; err != nil {
		t.Fatal(err)
	}
	assertBlockedBeforeDDL("local DNS address")
	if err := db.Table("server_config_overrides").Where("server_id = ?", server.Id).Update("dns", `[]`).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.System().UpdateValueByCategoryKey(ctx, "server", "NodePullInterval", "86401", "int"); err != nil {
		t.Fatal(err)
	}
	assertBlockedBeforeDDL("invalid pull interval")
	if err := store.System().UpdateValueByCategoryKey(ctx, "server", "NodePullInterval", "10", "int"); err != nil {
		t.Fatal(err)
	}
	legacyEntries, err := store.System().GetLegacyNodeConfigUncached(ctx)
	if err != nil {
		t.Fatal(err)
	}
	globalLegacy, err := decodeLegacyRouting(legacyEntries)
	if err != nil {
		t.Fatal(err)
	}
	legacyOverride, err := store.Node().FindServerConfigOverride(ctx, server.Id)
	if err != nil {
		t.Fatal(err)
	}
	localLegacy, _, err := effectiveLegacyRouting(globalLegacy, legacyOverride)
	if err != nil {
		t.Fatal(err)
	}
	first, err := preflightOrderedCutover(ctx, store, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := preflightOrderedCutover(ctx, store, false)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("nondeterministic plans %v", err)
	}
	Migrate(deps)
	version, _, err = schemaVersion(ctx, *cfg)
	if err != nil || version != 2180 {
		t.Fatalf("version %d %v", version, err)
	}
	entries, err := store.System().GetNodeConfigUncached(ctx)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, e := range entries {
		values[e.Key] = e.Value
	}
	if values["RoutingCutover"] != "complete" || values["RoutingRevision"] != "1" || values["Routing"] != first.Global {
		t.Fatalf("global cutover not committed")
	}
	local, err := store.Node().FindServerConfigOverride(ctx, server.Id)
	if err != nil || local == nil || local.Block == nil || *local.Block != `["full:local.example"]` || local.DNS == nil || *local.DNS != `[]` {
		t.Fatalf("legacy backup changed: %#v %v", local, err)
	}
	active, err := store.Node().FindServerRoutingOverride(ctx, server.Id)
	if err != nil || active == nil || active.Routing == nil || *active.Routing != first.Local[server.Id] || active.RoutingRevision != 1 {
		t.Fatalf("local cutover missing: %#v %v", active, err)
	}
	var global config.NodeRouting
	if err := json.Unmarshal([]byte(values["Routing"]), &global); err != nil {
		t.Fatal(err)
	}
	kinds := []string{}
	for _, r := range global.Rules {
		kinds = append(kinds, r.Kind)
	}
	if !reflect.DeepEqual(kinds, []string{"protocol", "ip", "domain", "domain", "ip", "protocol"}) {
		t.Fatalf("legacy OR order %v", kinds)
	}
	compareEffectiveXrayRules(t, &global, globalLegacy)
	var localPolicy config.NodeRouting
	if err := json.Unmarshal([]byte(*active.Routing), &localPolicy); err != nil {
		t.Fatal(err)
	}
	compareEffectiveXrayRules(t, &localPolicy, localLegacy)
	// Simulate interruption after per-domain writes but before the final marker.
	if err := store.System().UpdateValueByCategoryKey(ctx, "server", "RoutingCutover", "pending", "string"); err != nil {
		t.Fatal(err)
	}
	Migrate(deps)
	entries, err = store.System().GetNodeConfigUncached(ctx)
	if err != nil {
		t.Fatal(err)
	}
	completed := false
	for _, e := range entries {
		if e.Key == "RoutingCutover" && e.Value == "complete" {
			completed = true
		}
	}
	if !completed {
		t.Fatal("interrupted cutover did not resume")
	}
}

// Compile both the actual legacy builder's ordered categories and the migrated
// package through a real Xray binary, then compare the compiled router rules.
func compareEffectiveXrayRules(t *testing.T, policy *config.NodeRouting, source config.LegacyRoutingInput) {
	t.Helper()
	xray := os.Getenv("PPANEL_TEST_XRAY")
	if xray == "" {
		t.Fatal("PPANEL_TEST_XRAY is required for the cutover integration test")
	}
	if len(policy.Outbounds) != 1 || len(source.Outbound) != 1 {
		t.Fatal("expected one real 02179 outbound")
	}
	newTag := "routing_" + policy.Outbounds[0].ID
	oldTag := strings.TrimSpace(source.Outbound[0].Name)
	dns := map[string]any{"type": "field", "network": "udp", "port": "53", "outboundTag": "dns_out"}
	legacy := []map[string]any{dns}
	appendLegacy := func(kind, tag string, values []string) {
		var normalized []string
		var err error
		switch kind {
		case "domain":
			normalized, err = config.NormalizeDomainRules("legacy", values)
		case "ip":
			normalized, err = config.NormalizeIPRules("legacy", values)
		case "protocol":
			normalized, err = config.NormalizeProtocolRules("legacy", values)
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(normalized) > 0 {
			legacy = append(legacy, map[string]any{"type": "field", kind: normalized, "outboundTag": tag})
		}
	}
	appendLegacy("protocol", "block", source.BlockProtocols)
	appendLegacy("ip", "block", source.BlockIPs)
	appendLegacy("domain", "block", source.Block)
	for _, ob := range source.Outbound {
		tag := strings.TrimSpace(ob.Name)
		appendLegacy("domain", tag, ob.Rules)
		appendLegacy("ip", tag, ob.IPRules)
		appendLegacy("protocol", tag, ob.ProtocolRules)
	}
	ordered := []map[string]any{dns}
	for _, rule := range policy.Rules {
		tag := map[string]string{"direct": "Default", "block": "block", policy.Outbounds[0].ID: newTag}[rule.OutboundID]
		if tag == "" || rule.Enabled == nil || !*rule.Enabled {
			t.Fatalf("unexpected converted rule %#v", rule)
		}
		ordered = append(ordered, map[string]any{"type": "field", rule.Kind: rule.Values, "outboundTag": tag})
	}
	compile := func(rules []map[string]any, customTag string) []map[string]any {
		t.Helper()
		cfg := map[string]any{"log": map[string]any{"loglevel": "none"}, "outbounds": []map[string]any{
			{"tag": "Default", "protocol": "freedom", "settings": map[string]any{}},
			{"tag": "block", "protocol": "blackhole", "settings": map[string]any{}},
			{"tag": "dns_out", "protocol": "dns", "settings": map[string]any{}},
			{"tag": customTag, "protocol": source.Outbound[0].Protocol, "settings": map[string]any{"address": source.Outbound[0].Address, "port": source.Outbound[0].Port}},
		}, "routing": map[string]any{"domainStrategy": "AsIs", "rules": rules}}
		body, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(t.TempDir(), "xray.json")
		if err := os.WriteFile(file, body, 0600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		output, err := exec.CommandContext(ctx, xray, "convert", "pb", "-debug", file).CombinedOutput()
		if err != nil {
			t.Fatalf("compile Xray fixture: %v: %s", err, output)
		}
		start := bytes.Index(output, []byte("{\n"))
		if start < 0 {
			t.Fatalf("Xray debug JSON missing: %s", output)
		}
		var decoded struct {
			App []map[string]json.RawMessage `json:"app"`
		}
		if err := json.Unmarshal(output[start:], &decoded); err != nil {
			t.Fatal(err)
		}
		for _, app := range decoded.App {
			if raw, ok := app["rule"]; ok {
				var result []map[string]any
				if err := json.Unmarshal(raw, &result); err != nil {
					t.Fatal(err)
				}
				return result
			}
		}
		t.Fatal("compiled Xray router missing")
		return nil
	}
	before, after := compile(legacy, oldTag), compile(ordered, newTag)
	for _, rule := range after {
		if target, ok := rule["TargetTag"].(map[string]any); ok && target["Tag"] == newTag {
			target["Tag"] = oldTag
		}
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("compiled Xray rules changed\nlegacy=%#v\nordered=%#v", before, after)
	}
	proof := runNodeProof(t, map[string]any{"kind": "policy", "policy": policy})
	if !proof.OK {
		t.Fatalf("final ppnode builder rejected converted policy: %s", proof.Error)
	}
	if len(proof.Rules) != len(legacy) {
		t.Fatalf("node rule count %d, legacy %d", len(proof.Rules), len(legacy))
	}
	for i, rule := range legacy {
		kind := "system"
		for _, name := range []string{"domain", "ip", "protocol"} {
			if _, ok := rule[name]; ok {
				kind = name
			}
		}
		tag := rule["outboundTag"].(string)
		if tag == oldTag {
			tag = newTag
		}
		if proof.Rules[i].Kind != kind || proof.Rules[i].Tag != tag {
			t.Fatalf("node rule %d = %#v, want %s/%s", i, proof.Rules[i], kind, tag)
		}
	}
}
