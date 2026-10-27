package system

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/perfect-panel/server/internal/config"
)

func serverRows(values map[string]string) []*System {
	types := map[string]string{
		"NodePullInterval":       "int",
		"NodePushInterval":       "int",
		"TrafficReportThreshold": "int",
	}
	var rows []*System
	for key, value := range values {
		kind := types[key]
		if kind == "" {
			kind = "string"
		}
		rows = append(rows, &System{Category: "server", Key: key, Value: value, Type: kind})
	}
	return rows
}

func TestParseNodeConfig(t *testing.T) {
	parsed, err := ParseNodeConfig(serverRows(map[string]string{
		"NodeSecret":             "secret",
		"NodePullInterval":       "10",
		"NodePushInterval":       "60",
		"TrafficReportThreshold": "1024",
		"IPStrategy":             "prefer_ipv4",
		"DNS":                    `[{"proto":"udp","address":"1.1.1.1:53","domains":["example.com"]}]`,
		"Block":                  `["ads.example","","ads.example","tracker.example"]`,
		"BlockIPs":               `["geoip:private","10.0.0.0/8"]`,
		"BlockProtocols":         `["bittorrent"]`,
		"Outbound":               `[{"name":"warp","protocol":"wireguard","address":"162.159.192.1","port":2408,"password":"k","rules":["geosite:openai"],"ip_rules":["geoip:cn"],"protocol_rules":["bittorrent"]}]`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := config.NodeConfig{
		NodeSecret:             "secret",
		NodePullInterval:       10,
		NodePushInterval:       60,
		TrafficReportThreshold: 1024,
		IPStrategy:             "prefer_ipv4",
		DNS:                    []config.NodeDNS{{Proto: "udp", Address: "1.1.1.1:53", Domains: []string{"example.com"}}},
		Block:                  []string{"ads.example", "tracker.example"},
		BlockIPs:               []string{"geoip:private", "10.0.0.0/8"},
		BlockProtocols:         []string{"bittorrent"},
		Outbound: []config.NodeOutbound{{
			Name: "warp", Protocol: "wireguard", Address: "162.159.192.1", Port: 2408,
			Password: "k", Rules: []string{"geosite:openai"}, IPRules: []string{"geoip:cn"}, ProtocolRules: []string{"bittorrent"},
		}},
	}
	if !reflect.DeepEqual(parsed, want) {
		t.Fatalf("parsed\n%+v\nwant\n%+v", parsed, want)
	}
}

// The rows a fresh installation seeds hold empty documents.
func TestParseNodeConfigEmptyDocuments(t *testing.T) {
	parsed, err := ParseNodeConfig(serverRows(map[string]string{"DNS": "", "Block": "", "BlockIPs": "", "BlockProtocols": "", "Outbound": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.DNS != nil || parsed.Block != nil || parsed.BlockIPs != nil || parsed.BlockProtocols != nil || parsed.Outbound != nil {
		t.Fatalf("empty documents decoded as %+v", parsed)
	}
	// An explicitly empty list stays a list: the admin console receives [].
	parsed, err = ParseNodeConfig(serverRows(map[string]string{"DNS": "[]", "Outbound": "[]"}))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(struct {
		DNS      []config.NodeDNS
		Outbound []config.NodeOutbound
	}{parsed.DNS, parsed.Outbound})
	if string(data) != `{"DNS":[],"Outbound":[]}` {
		t.Fatalf("empty lists encode as %s", data)
	}
}

func TestParseNodeConfigReportsMalformedDocuments(t *testing.T) {
	for _, key := range []string{"DNS", "Block", "BlockIPs", "BlockProtocols", "Outbound"} {
		_, err := ParseNodeConfig(serverRows(map[string]string{key: `{not json`}))
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Fatalf("%s: err = %v, want a decode error naming the setting", key, err)
		}
		var syntax *json.SyntaxError
		if !errors.As(err, &syntax) {
			t.Fatalf("%s: err = %v, want the json.SyntaxError reachable", key, err)
		}
	}
}

func TestParseNodeConfigRejectsPartiallyMalformedBlockLists(t *testing.T) {
	for _, key := range []string{"Block", "BlockIPs", "BlockProtocols"} {
		_, err := ParseNodeConfig(serverRows(map[string]string{key: `["a", 1, "b"]`}))
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Fatalf("%s: err = %v, want strict string-list decoding", key, err)
		}
	}
}
