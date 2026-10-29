package config

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNodeConfigOrderedRoutingJSONAndYAMLRoundTrip(t *testing.T) {
	enabled := false
	original := NodeConfig{RoutingRevision: 7, Routing: &NodeRouting{Version: 1, Outbounds: []NodeOutbound{}, Rules: []NodeRoute{{ID: "rr_11111111-1111-4111-8111-111111111111", Enabled: &enabled, Kind: "domain", Values: []string{"geosite:cn"}, OutboundID: "block"}}, DefaultOutboundID: "direct"}}
	body, err := json.Marshal(&original)
	if err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{`"Block"`, `"block_ips"`, `"block_protocols"`, `"Outbound"`} {
		if strings.Contains(string(body), removed) {
			t.Fatalf("runtime exposed legacy %s", body)
		}
	}
	var decoded NodeConfig
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.Routing, original.Routing) || decoded.Routing.Rules[0].Enabled == nil || *decoded.Routing.Rules[0].Enabled {
		t.Fatalf("JSON %#v", decoded)
	}
	yamlBody, err := yaml.Marshal(&original)
	if err != nil {
		t.Fatal(err)
	}
	var fromYAML NodeConfig
	if err := yaml.Unmarshal(yamlBody, &fromYAML); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromYAML.Routing, original.Routing) {
		t.Fatalf("YAML %#v", fromYAML)
	}
}
