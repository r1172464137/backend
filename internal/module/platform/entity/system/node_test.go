package system

import (
	"strings"
	"testing"
)

func serverRows(values map[string]string) []*System {
	types := map[string]string{"NodePullInterval": "int", "NodePushInterval": "int", "TrafficReportThreshold": "int", "RoutingRevision": "int64"}
	rows := []*System{}
	for key, value := range values {
		kind := types[key]
		if kind == "" {
			kind = "string"
		}
		rows = append(rows, &System{Category: "server", Key: key, Value: value, Type: kind})
	}
	return rows
}

func validRows() []*System {
	return serverRows(map[string]string{"NodeSecret": "secret", "NodePullInterval": "10", "NodePushInterval": "60", "DNS": `[{"proto":"udp","address":"1.1.1.1:53","domains":["example.com"]}]`, "Routing": `{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct"}`, "RoutingRevision": "2", "RoutingCutover": "complete", "Block": "{broken", "Outbound": "{broken"})
}

func TestParseNodeConfig(t *testing.T) {
	got, err := ParseNodeConfig(validRows())
	if err != nil {
		t.Fatal(err)
	}
	if got.NodeSecret != "secret" || got.NodePullInterval != 10 || got.NodePushInterval != 60 || got.RoutingRevision != 2 || got.Routing == nil || len(got.DNS) != 1 {
		t.Fatalf("unexpected config %+v", got)
	}
}

func TestParseNodeConfigRejectsInvalidActiveSettings(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"DNS", `{bad`}, {"DNS", `[{"proto":"https","address":"1.1.1.1"}]`},
		{"Routing", `{bad`}, {"Routing", `{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct","unknown":true}`},
		{"RoutingRevision", "0"}, {"RoutingRevision", "-1"}, {"RoutingCutover", "pending"}, {"NodePullInterval", "86401"},
	} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			rows := validRows()
			for _, r := range rows {
				if r.Key == tc.key {
					r.Value = tc.value
				}
			}
			if _, err := ParseNodeConfig(rows); err == nil {
				t.Fatal("accepted invalid settings")
			}
		})
	}
	for _, key := range []string{"Routing", "RoutingRevision", "RoutingCutover"} {
		t.Run("missing "+key, func(t *testing.T) {
			var rows []*System
			for _, row := range validRows() {
				if row.Key != key {
					rows = append(rows, row)
				}
			}
			if _, err := ParseNodeConfig(rows); err == nil {
				t.Fatal("accepted missing " + key)
			}
		})
	}
	rows := validRows()
	rows = append(rows, &System{Key: "RoutingRevision", Value: "2", Type: "int64"})
	if _, err := ParseNodeConfig(rows); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate: %v", err)
	}
}
