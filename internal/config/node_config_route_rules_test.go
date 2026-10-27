package config

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNodeConfigStructuredRouteRulesJSONAndYAMLRoundTrip(t *testing.T) {
	original := NodeConfig{
		Block:          []string{"geosite:ads"},
		BlockIPs:       []string{"geoip:private", "10.0.0.0/8"},
		BlockProtocols: []string{"bittorrent"},
		Outbound: []NodeOutbound{{
			Name: "cn", Protocol: "direct", Rules: []string{"geosite:cn"}, IPRules: []string{"geoip:cn"}, ProtocolRules: []string{"bittorrent"},
		}},
	}

	jsonData, err := json.Marshal(&original)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"block_ips"`, `"block_protocols"`, `"ip_rules"`, `"protocol_rules"`} {
		if !strings.Contains(string(jsonData), field) {
			t.Fatalf("JSON %s does not contain %s", jsonData, field)
		}
	}
	var fromJSON NodeConfig
	if err := json.Unmarshal(jsonData, &fromJSON); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromJSON.BlockIPs, original.BlockIPs) || !reflect.DeepEqual(fromJSON.Outbound[0].IPRules, original.Outbound[0].IPRules) {
		t.Fatalf("JSON round trip = %#v", fromJSON)
	}

	yamlData, err := yaml.Marshal(&original)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"block_ips:", "block_protocols:", "ip_rules:", "protocol_rules:"} {
		if !strings.Contains(string(yamlData), field) {
			t.Fatalf("YAML %s does not contain %s", yamlData, field)
		}
	}
	var fromYAML NodeConfig
	if err := yaml.Unmarshal(yamlData, &fromYAML); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromYAML.BlockProtocols, original.BlockProtocols) || !reflect.DeepEqual(fromYAML.Outbound[0].ProtocolRules, original.Outbound[0].ProtocolRules) {
		t.Fatalf("YAML round trip = %#v", fromYAML)
	}
}
