package nodeconfig

import (
	"encoding/json"
	"testing"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
)

// A server either inherits the global package or holds a complete local one.
// Legacy DNS/IP columns left on the row are rollback material: they are not
// shown and do not change the result.
func TestOverrideResponseIgnoresLegacyColumns(t *testing.T) {
	legacyDNS, legacyIP := `[{"proto":"udp","address":"8.8.8.8:53"}]`, "prefer_ipv6"
	resp, err := OverrideResponse(&node.ServerConfigOverride{ServerId: 7, DNS: &legacyDNS, IPStrategy: &legacyIP, RoutingRevision: 2})
	if err != nil || !resp.InheritRouting || resp.Routing != nil || resp.RoutingRevision != 2 {
		t.Fatalf("inherit with legacy columns: %#v %v", resp, err)
	}
	body, _ := json.Marshal(resp)
	var fields map[string]any
	_ = json.Unmarshal(body, &fields)
	for _, legacy := range []string{"dns", "ip_strategy", "inherit_dns", "inherit_ip_strategy"} {
		if _, ok := fields[legacy]; ok {
			t.Fatalf("override response exposes %s: %s", legacy, body)
		}
	}

	local, _ := json.Marshal(config.DefaultNodeRouting("server-7"))
	raw := string(local)
	resp, err = OverrideResponse(&node.ServerConfigOverride{ServerId: 7, Routing: &raw, RoutingRevision: 3})
	if err != nil || resp.InheritRouting || resp.Routing == nil || resp.Routing.Version != config.RoutingVersion {
		t.Fatalf("local package: %#v %v", resp, err)
	}

	v1 := `{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct"}`
	if _, err := OverrideResponse(&node.ServerConfigOverride{ServerId: 7, Routing: &v1, RoutingRevision: 3}); err == nil {
		t.Fatal("format 1 local routing accepted at runtime")
	}
	if resp, err := OverrideResponse(nil); err != nil || !resp.InheritRouting {
		t.Fatalf("no row: %#v %v", resp, err)
	}
}
