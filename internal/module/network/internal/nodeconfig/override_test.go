package nodeconfig

import (
	"strings"
	"testing"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
)

func TestOrderedNonRoutingOverrideAndInheritance(t *testing.T) {
	global := GlobalValues(config.NodeConfig{IPStrategy: "prefer_ipv4", DNS: []config.NodeDNS{{Proto: "udp", Address: "8.8.8.8:53", Domains: []string{"geosite:cn"}}}, RoutingRevision: 3})
	model, err := OverrideModel(7, dto.ServerNodeConfigOverride{InheritIPStrategy: false, IPStrategy: "prefer_ipv6", InheritDNS: false, DNS: []dto.NodeDNS{}})
	if err != nil {
		t.Fatal(err)
	}
	values := CloneValues(global)
	if err := ApplyOverride(&values, model); err != nil {
		t.Fatal(err)
	}
	if values.IPStrategy != "prefer_ipv6" || values.DNS == nil || len(values.DNS) != 0 {
		t.Fatalf("effective %#v", values)
	}
	projection, err := OverrideResponse(model)
	if err != nil {
		t.Fatal(err)
	}
	if projection.InheritIPStrategy || projection.InheritDNS || !projection.InheritRouting {
		t.Fatalf("override %#v", projection)
	}
}
func TestOrderedDNSLimits(t *testing.T) {
	_, err := OverrideModel(1, dto.ServerNodeConfigOverride{InheritIPStrategy: true, DNS: []dto.NodeDNS{{Proto: "https", Address: strings.Repeat("x", config.MaxPersistedJSONBytes)}}})
	if err == nil {
		t.Fatal("oversize DNS accepted")
	}
}
