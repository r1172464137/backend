package systemsetting

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	appconfig "github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
)

func TestNodeConfigSystemRoundTripIncludesStructuredRules(t *testing.T) {
	entries := []*system.System{
		{Key: "NodeSecret", Value: "secret", Type: "string"},
		{Key: "NodePullInterval", Value: "30", Type: "int64"},
		{Key: "NodePushInterval", Value: "60", Type: "int64"},
		{Key: "TrafficReportThreshold", Value: "1024", Type: "int64"},
		{Key: "IPStrategy", Value: "prefer_ipv4", Type: "string"},
		{Key: "DNS", Value: `[{"proto":"udp","address":"1.1.1.1:53","domains":[]}]`, Type: "string"},
		{Key: "Block", Value: `["geosite:ads"]`, Type: "string"},
		{Key: "BlockIPs", Value: `["geoip:private","10.0.0.0/8"]`, Type: "string"},
		{Key: "BlockProtocols", Value: `["bittorrent"]`, Type: "string"},
		{Key: "Outbound", Value: `[{"name":"cn","protocol":"direct","rules":["geosite:cn"],"ip_rules":["geoip:cn"],"protocol_rules":["bittorrent"],"plugin_opts":{"mode":"test"}}]`, Type: "string"},
	}

	parsed, err := system.ParseNodeConfig(entries)
	if err != nil {
		t.Fatalf("ParseNodeConfig() error = %v", err)
	}
	got := nodeConfigView(parsed)
	if !reflect.DeepEqual(got.BlockIPs, []string{"geoip:private", "10.0.0.0/8"}) || !reflect.DeepEqual(got.BlockProtocols, []string{"bittorrent"}) {
		t.Fatalf("structured block rules = %#v/%#v", got.BlockIPs, got.BlockProtocols)
	}
	if len(got.Outbound) != 1 || !reflect.DeepEqual(got.Outbound[0].IPRules, []string{"geoip:cn"}) ||
		!reflect.DeepEqual(got.Outbound[0].ProtocolRules, []string{"bittorrent"}) {
		t.Fatalf("outbound = %#v, want structured rules", got.Outbound)
	}
	plugin, ok := got.Outbound[0].PluginOptions.(map[string]any)
	if !ok || plugin["mode"] != "test" {
		t.Fatalf("plugin options = %#v, want existing object preserved", got.Outbound[0].PluginOptions)
	}

	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"block"`, `"block_ips"`, `"block_protocols"`, `"ip_rules"`, `"protocol_rules"`} {
		if !strings.Contains(string(encoded), field) {
			t.Errorf("JSON %s does not contain %s", encoded, field)
		}
	}
}

func TestPrepareNodeConfigUpdatePreservesOldClientOmissions(t *testing.T) {
	current := &dto.NodeConfig{
		BlockIPs:       []string{"geoip:private"},
		BlockProtocols: []string{"bittorrent"},
		Outbound: []dto.PlatformNodeOutboundSnapshot{
			{Name: "first", IPRules: []string{"geoip:cn"}, ProtocolRules: []string{"bittorrent"}},
			{Name: "second", IPRules: []string{"192.0.2.0/24"}, ProtocolRules: []string{"bittorrent"}},
		},
	}
	var request dto.NodeConfig
	if err := json.Unmarshal([]byte(`{
		"block":["example.com"],
		"outbound":[
			{"name":"second","protocol":"direct","rules":["geosite:cn"]},
			{"name":"first","protocol":"direct","rules":[]}
		]
	}`), &request); err != nil {
		t.Fatal(err)
	}
	if err := prepareNodeConfigUpdate(&request, current); err != nil {
		t.Fatalf("prepareNodeConfigUpdate() error = %v", err)
	}
	if !reflect.DeepEqual(request.Block, []string{"full:example.com"}) ||
		!reflect.DeepEqual(request.BlockIPs, current.BlockIPs) || !reflect.DeepEqual(request.BlockProtocols, current.BlockProtocols) {
		t.Fatalf("global block merge = %#v/%#v/%#v", request.Block, request.BlockIPs, request.BlockProtocols)
	}
	if !reflect.DeepEqual(request.Outbound[0].IPRules, []string{"192.0.2.0/24"}) ||
		!reflect.DeepEqual(request.Outbound[1].IPRules, []string{"geoip:cn"}) {
		t.Fatalf("outbound hidden rules were not preserved through reorder: %#v", request.Outbound)
	}
}

func TestPrepareNodeConfigUpdateHonorsExplicitEmptyAndRejectsInvalid(t *testing.T) {
	current := &dto.NodeConfig{
		BlockIPs:       []string{"geoip:private"},
		BlockProtocols: []string{"bittorrent"},
		Outbound: []dto.PlatformNodeOutboundSnapshot{{
			Name: "direct", IPRules: []string{"geoip:cn"}, ProtocolRules: []string{"bittorrent"},
		}},
	}
	var request dto.NodeConfig
	if err := json.Unmarshal([]byte(`{
		"block":[],"block_ips":[],"block_protocols":[],
		"outbound":[{"name":"direct","protocol":"direct","rules":[],"ip_rules":[],"protocol_rules":[]}]
	}`), &request); err != nil {
		t.Fatal(err)
	}
	if err := prepareNodeConfigUpdate(&request, current); err != nil {
		t.Fatal(err)
	}
	if request.BlockIPs == nil || request.BlockProtocols == nil || request.Outbound[0].IPRules == nil || request.Outbound[0].ProtocolRules == nil {
		t.Fatalf("explicit empty arrays became nil: %#v", request)
	}
	if len(request.BlockIPs) != 0 || len(request.BlockProtocols) != 0 || len(request.Outbound[0].IPRules) != 0 || len(request.Outbound[0].ProtocolRules) != 0 {
		t.Fatalf("explicit empty arrays were not honored: %#v", request)
	}

	invalid := &dto.NodeConfig{Block: []string{"geoip:private"}}
	if err := prepareNodeConfigUpdate(invalid, current); err == nil || !strings.Contains(err.Error(), `block line 1: unknown domain rule prefix "geoip"`) {
		t.Fatalf("invalid category error = %v", err)
	}
}

func TestGlobalOutboundOmissionMatchingScenarios(t *testing.T) {
	current := &dto.NodeConfig{Outbound: []dto.PlatformNodeOutboundSnapshot{
		{Name: "a", Protocol: "direct", Address: "a.example", Rules: []string{}, IPRules: []string{"geoip:a"}},
		{Name: "b", Protocol: "socks", Address: "b.example", Rules: []string{}, IPRules: []string{"geoip:b"}},
		{Name: "c", Protocol: "direct", Address: "c.example", Rules: []string{}, IPRules: []string{"geoip:c"}},
	}}
	bind := func(t *testing.T, outbound string) dto.NodeConfig {
		t.Helper()
		var req dto.NodeConfig
		if err := json.Unmarshal([]byte(`{"block":[],"block_ips":[],"block_protocols":[],"outbound":`+outbound+`}`), &req); err != nil {
			t.Fatal(err)
		}
		return req
	}

	t.Run("rename and reorder", func(t *testing.T) {
		req := bind(t, `[{"name":"b","protocol":"socks","address":"b.example","rules":[]},{"name":"renamed-a","protocol":"direct","address":"a.example","rules":[]}]`)
		if err := prepareNodeConfigUpdate(&req, current); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(req.Outbound[0].IPRules, []string{"geoip:b"}) || !reflect.DeepEqual(req.Outbound[1].IPRules, []string{"geoip:a"}) {
			t.Fatalf("hidden rules swapped: %#v", req.Outbound)
		}
	})
	t.Run("insertion", func(t *testing.T) {
		req := bind(t, `[{"name":"new","protocol":"vless","address":"new.example","rules":[]},{"name":"a","protocol":"direct","address":"a.example","rules":[]},{"name":"b","protocol":"socks","address":"b.example","rules":[]},{"name":"c","protocol":"direct","address":"c.example","rules":[]}]`)
		if err := prepareNodeConfigUpdate(&req, current); err != nil {
			t.Fatal(err)
		}
		if len(req.Outbound[0].IPRules) != 0 || !reflect.DeepEqual(req.Outbound[1].IPRules, []string{"geoip:a"}) {
			t.Fatalf("insertion inherited unrelated hidden rules: %#v", req.Outbound)
		}
	})
	t.Run("rename plus edit with unchanged anchors", func(t *testing.T) {
		req := bind(t, `[{"name":"a","protocol":"direct","address":"a.example","rules":[]},{"name":"renamed-b","protocol":"socks","address":"edited.example","rules":[]},{"name":"c","protocol":"direct","address":"c.example","rules":[]}]`)
		if err := prepareNodeConfigUpdate(&req, current); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(req.Outbound[1].IPRules, []string{"geoip:b"}) {
			t.Fatalf("safe positional preservation failed: %#v", req.Outbound)
		}
	})
	t.Run("rename plus edit and reorder is rejected", func(t *testing.T) {
		req := bind(t, `[{"name":"renamed-b","protocol":"socks","address":"edited.example","rules":[]},{"name":"a","protocol":"direct","address":"a.example","rules":[]},{"name":"c","protocol":"direct","address":"c.example","rules":[]}]`)
		if err := prepareNodeConfigUpdate(&req, current); err == nil || !strings.Contains(err.Error(), "upgraded client") {
			t.Fatalf("error = %v, want ambiguity rejection", err)
		}
	})
	t.Run("deletion and reorder", func(t *testing.T) {
		req := bind(t, `[{"name":"c","protocol":"direct","address":"c.example","rules":[]},{"name":"a","protocol":"direct","address":"a.example","rules":[]}]`)
		if err := prepareNodeConfigUpdate(&req, current); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(req.Outbound[0].IPRules, []string{"geoip:c"}) || !reflect.DeepEqual(req.Outbound[1].IPRules, []string{"geoip:a"}) {
			t.Fatalf("deletion/reorder hidden rules = %#v", req.Outbound)
		}
	})
	for name, outbound := range map[string]string{
		"blank":     `[{"name":" ","protocol":"direct","rules":[]}]`,
		"duplicate": `[{"name":"a","protocol":"direct","rules":[]},{"name":" a ","protocol":"socks","rules":[]}]`,
	} {
		t.Run(name, func(t *testing.T) {
			req := bind(t, outbound)
			if err := prepareNodeConfigUpdate(&req, current); err == nil {
				t.Fatalf("%s outbound names unexpectedly accepted", name)
			}
		})
	}
}

func TestGlobalConfigRejectsBlankOutboundProtocolBeforeNormalization(t *testing.T) {
	outbound := []dto.PlatformNodeOutboundSnapshot{
		{Name: "first", Protocol: "   ", IPRules: []string{"geoip:first"}},
		{Name: "second", Protocol: "direct", IPRules: []string{"geoip:second"}},
	}
	req := &dto.NodeConfig{Outbound: outbound}
	if err := prepareNodeConfigUpdate(req, &dto.NodeConfig{}); err == nil || !strings.Contains(err.Error(), "outbound line 1: outbound protocol must not be blank") {
		t.Fatalf("blank protocol error = %v", err)
	}
	if outbound[1].Name != "second" || !reflect.DeepEqual(outbound[1].IPRules, []string{"geoip:second"}) {
		t.Fatalf("invalid first outbound shifted the second identity: %#v", outbound)
	}
}

func TestGlobalNodeConfigEnforcesSerializedTextLimit(t *testing.T) {
	rules := make([]string, 900)
	for index := range rules {
		rules[index] = fmt.Sprintf("host-%04d.%s.example", index, strings.Repeat("a", 55))
	}
	req := &dto.NodeConfig{Block: rules}
	if err := prepareNodeConfigUpdate(req, &dto.NodeConfig{}); err == nil || !strings.Contains(err.Error(), "serialized JSON") {
		t.Fatalf("oversize block error = %v", err)
	}
	outbound := &dto.NodeConfig{Outbound: []dto.PlatformNodeOutboundSnapshot{{
		Name: "large", Protocol: "direct", Settings: strings.Repeat("x", appconfig.MaxPersistedJSONBytes),
	}}}
	if err := prepareNodeConfigUpdate(outbound, &dto.NodeConfig{}); err == nil || !strings.Contains(err.Error(), "outbound: serialized JSON") {
		t.Fatalf("oversize outbound error = %v", err)
	}
}

func TestConvertedNodeConfigFieldsExcludePresenceMetadata(t *testing.T) {
	fields := nodeConfigFields(dto.NodeConfig{
		BlockIPs:          []string{"geoip:private"},
		RequestFields:     map[string]struct{}{"block_ips": {}},
		OutboundFieldSets: []map[string]struct{}{{"ip_rules": {}}},
	})
	seen := make(map[string]configFieldValue, len(fields))
	for _, field := range fields {
		seen[field.key] = field
	}
	if _, ok := seen["BlockIPs"]; !ok {
		t.Fatalf("persisted node config fields = %#v", seen)
	}
	if _, ok := seen["RequestFields"]; ok {
		t.Fatalf("RequestFields must not be persisted: %#v", seen)
	}
	if _, ok := seen["OutboundFieldSets"]; ok {
		t.Fatalf("OutboundFieldSets must not be persisted: %#v", seen)
	}
	if seen["BlockIPs"].valueType != "string" || seen["Outbound"].valueType != "string" {
		t.Fatalf("JSON node fields must keep string storage type: %#v", seen)
	}
}
