package adminserver

import (
	"encoding/json"
	"testing"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
)

func TestRealityXverUpdateOmissionAndExplicitReset(t *testing.T) {
	existing := node.Protocol{Type: "vless", Security: "reality", RealityXver: 2, AcceptProxyProtocol: true}
	for _, tc := range []struct {
		body string
		want int
	}{
		{`{"type":"vless","security":"reality"}`, 2},
		{`{"type":"vless","security":"reality","reality_xver":0}`, 0},
		{`{"type":"vless","security":"reality","reality_xver":1}`, 1},
		{`{"type":"vless","security":"reality","reality_xver":2}`, 2},
	} {
		var item dto.Protocol
		if err := json.Unmarshal([]byte(tc.body), &item); err != nil {
			t.Fatal(err)
		}
		var next node.Protocol
		mapping.DeepCopy(&next, item)
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(tc.body), &raw); err != nil {
			t.Fatal(err)
		}
		provided := make(map[string]struct{})
		for key := range raw {
			provided[key] = struct{}{}
		}
		merged, err := mergeMissingProtocolFields(next, existing, provided)
		if err != nil {
			t.Fatal(err)
		}
		ensureRealityProtocolDefaults(&merged)
		merged, err = node.NormalizeProtocolForStorage(merged)
		if err != nil {
			t.Fatal(err)
		}
		if merged.RealityXver != tc.want || !merged.AcceptProxyProtocol {
			t.Fatalf("%s: merged = %+v", tc.body, merged)
		}
	}
	next, err := mergeMissingProtocolFields(node.Protocol{Type: "vless", Security: "tls"}, existing, map[string]struct{}{"type": {}, "security": {}})
	if err != nil {
		t.Fatal(err)
	}
	next, err = node.NormalizeProtocolForStorage(next)
	if err != nil || next.RealityXver != 0 {
		t.Fatalf("security change: %+v, %v", next, err)
	}
}
