package bootstrap

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
)

// Verify exact boundary behavior against the actual same-batch node guard,
// not a separate handwritten expectation about what the node might support.
func TestSpiderURIParityWithNode(t *testing.T) {
	requireNodeProof(t)
	cases := []struct {
		value string
		valid bool
	}{
		{"", true}, {"/", true}, {"/path?p=100-200&c=1-2&t=1-4&i=10-100&r=100-500&other=value", true},
		{"/?p=4096&c=8&t=8&i=30000&r=5000", true}, {"/?c=4&t=16", true}, {"/?p=0-0&c=0&t=0&i=0&r=0", true},
		{"/%", false}, {"/?p=%", false}, {"/?p=-1", false}, {"/?t=1-2-3", false}, {"/?i=5-1", false}, {"/?r=+1", false},
		{"/?p=", false}, {"/?p=4097", false}, {"/?c=9", false}, {"/?t=17", false}, {"/?i=30001", false}, {"/?r=5001", false},
		{"/?p=18446744073709551616", false}, {"/?p=1&p=2", false}, {"/?%70=1&p=2", false}, {"/?P=1", false}, {"/?x=1&x=2", false}, {"/?c=8&t=9", false},
		{"/" + strings.Repeat("x", 4095), true}, {"/" + strings.Repeat("x", 4096), false},
		{"/?" + strings.Repeat("k", 129) + "=v", false}, {"/?k=" + strings.Repeat("x", 1025), false},
	}
	keys := make([]string, 0, 33)
	for i := 0; i < 33; i++ {
		keys = append(keys, fmt.Sprintf("x%d=1", i))
	}
	cases = append(cases, struct {
		value string
		valid bool
	}{"/?" + strings.Join(keys, "&"), false})
	for i, tc := range cases {
		t.Run(fmt.Sprintf("case_%02d", i), func(t *testing.T) {
			backend := config.ValidateNodeURI("spider_x", tc.value, true) == nil
			proof := runNodeProof(t, map[string]any{"kind": "spider_uri", "uri": tc.value})
			if backend != tc.valid || proof.OK != tc.valid {
				t.Fatalf("backend=%v node=%v (%s), want %v", backend, proof.OK, proof.Error, tc.valid)
			}
		})
	}
}

// This single fixture is evaluated by both the offline converter/inventory and
// the actual same-batch node validator + Xray builders. A stricter backend is
// intentional for legacy equivalence and unconsumed fields; it may never pass
// a fixture that the node rejects.
func TestNodeCapabilityContractFixtures(t *testing.T) {
	requireNodeProof(t)
	data, err := os.ReadFile("testdata/node-capabilities.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name     string              `json:"name"`
		Kind     string              `json:"kind"`
		Backend  bool                `json:"backend"`
		Node     bool                `json:"node"`
		Outbound config.NodeOutbound `json:"outbound"`
		Protocol node.Protocol       `json:"protocol"`
		DNS      config.NodeDNS      `json:"dns"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			var backend bool
			var result nodeProofResult
			switch tc.Kind {
			case "outbound":
				preview, err := config.ConvertLegacyRouting(config.LegacyRoutingInput{Outbound: []config.NodeOutbound{tc.Outbound}}, "contract")
				if err != nil {
					t.Fatal(err)
				}
				backend = preview.Draft != nil
				result = runNodeProof(t, map[string]any{"kind": "outbound", "outbound": tc.Outbound})
				if backend {
					converted := runNodeProof(t, map[string]any{"kind": "policy", "policy": preview.Draft})
					if !converted.OK {
						t.Fatalf("converted policy rejected by node: %s", converted.Error)
					}
				}
			case "dns":
				_, err := config.NormalizeNodeDNS([]config.NodeDNS{tc.DNS})
				backend = err == nil
				result = runNodeProof(t, map[string]any{"kind": "dns", "dns": tc.DNS})
			case "inbound":
				b, err := json.Marshal([]node.Protocol{tc.Protocol})
				if err != nil {
					t.Fatal(err)
				}
				backend = node.ValidateNodeProtocols(&node.Server{Protocols: string(b)}) == nil
				result = runNodeProof(t, map[string]any{"kind": "inbound", "protocol": tc.Protocol})
			default:
				t.Fatalf("unknown fixture kind %q", tc.Kind)
			}
			if backend != tc.Backend || result.OK != tc.Node {
				t.Fatalf("backend=%v node=%v (%s); want %v/%v", backend, result.OK, result.Error, tc.Backend, tc.Node)
			}
			if backend && !result.OK {
				t.Fatal("converter admitted a configuration the actual node rejects")
			}
		})
	}
}
