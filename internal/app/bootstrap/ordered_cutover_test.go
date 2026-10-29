package bootstrap

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"github.com/perfect-panel/server/internal/repository"
)

type cutoverFakeStore struct {
	repository.Store
	system repository.SystemRepo
	nodes  repository.NodeRepo
}

func (s cutoverFakeStore) System() repository.SystemRepo { return s.system }
func (s cutoverFakeStore) Node() repository.NodeRepo     { return s.nodes }

type cutoverFakeSystem struct {
	repository.SystemRepo
	entries []*system.System
}

func (s cutoverFakeSystem) GetLegacyNodeConfigUncached(context.Context) ([]*system.System, error) {
	return s.entries, nil
}

type cutoverFakeNodes struct {
	repository.NodeRepo
	overrides map[int64]*node.ServerConfigOverride
	protocols map[int64]string
}

func (s cutoverFakeNodes) QueryServerSorts(context.Context) ([]node.SortItem, error) {
	return []node.SortItem{{Id: 1}, {Id: 2}, {Id: 3}, {Id: 4}, {Id: 5}}, nil
}
func (s cutoverFakeNodes) QueryServerList(_ context.Context, ids []int64) ([]*node.Server, error) {
	out := make([]*node.Server, 0, len(ids))
	for _, id := range ids {
		raw := s.protocols[id]
		if raw == "" {
			raw = "[]"
		}
		out = append(out, &node.Server{Id: id, Protocols: raw})
	}
	return out, nil
}
func (s cutoverFakeNodes) FindServerConfigOverride(_ context.Context, id int64) (*node.ServerConfigOverride, error) {
	return s.overrides[id], nil
}
func TestCutoverPreflightFourLegacyCombinationsIsDeterministic(t *testing.T) {
	items := []*system.System{{Key: "Block", Type: "string", Value: `["geosite:global"]`}, {Key: "BlockIPs", Type: "string", Value: `["geoip:private"]`}, {Key: "BlockProtocols", Type: "string", Value: `["bittorrent"]`}, {Key: "Outbound", Type: "string", Value: `[{"name":"global","protocol":"direct","rules":["geosite:route"],"ip_rules":["10.0.0.0/8"],"protocol_rules":["bittorrent"]}]`}}
	block := `["geosite:local"]`
	outbound := `[{"name":"local","protocol":"direct","rules":["geosite:local-route"],"ip_rules":[],"protocol_rules":[]}]`
	dns := `[]`
	store := cutoverFakeStore{system: cutoverFakeSystem{entries: items}, nodes: cutoverFakeNodes{overrides: map[int64]*node.ServerConfigOverride{2: {Block: &block}, 3: {Outbound: &outbound}, 4: {Block: &block, Outbound: &outbound}, 5: {DNS: &dns}}}}
	first, err := preflightOrderedCutover(context.Background(), store, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := preflightOrderedCutover(context.Background(), store, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || len(first.Local) != 3 {
		t.Fatalf("nondeterministic plan %#v %#v", first, second)
	}
	var global config.NodeRouting
	if err := json.Unmarshal([]byte(first.Global), &global); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, rule := range global.Rules {
		kinds = append(kinds, rule.Kind)
	}
	if !reflect.DeepEqual(kinds, []string{"protocol", "ip", "domain", "domain", "ip", "protocol"}) {
		t.Fatalf("global legacy order %v", kinds)
	}
	for _, id := range []int64{2, 3, 4} {
		var local config.NodeRouting
		if err := json.Unmarshal([]byte(first.Local[id]), &local); err != nil {
			t.Fatal(err)
		}
		want := "geosite:global"
		if id == 2 || id == 4 {
			want = "geosite:local"
		}
		found := false
		for _, rule := range local.Rules {
			if rule.OutboundID == "block" && rule.Kind == "domain" && len(rule.Values) > 0 && rule.Values[0] == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("server %d missing %s", id, want)
		}
	}
}
func TestCutoverRejectsAmbiguousLegacyJSON(t *testing.T) {
	for _, raw := range []string{`[{"name":"one","name":"two","protocol":"direct"}]`, `[{"name":"one","protocol":"direct","security":"none","Security":"tls"}]`, `[{"name":"one","protocol":"direct","future":true}]`, `[] {}`} {
		_, err := decodeLegacyRouting([]*system.System{{Key: "Outbound", Type: "string", Value: raw}})
		if err == nil {
			t.Errorf("accepted ambiguous legacy JSON %s", raw)
		}
	}
}

func TestCutoverPreflightRejectsConflictBeforeAnyWrites(t *testing.T) {
	store := cutoverFakeStore{system: cutoverFakeSystem{entries: []*system.System{{Key: "Outbound", Type: "string", Value: `[{"name":"Default","protocol":"direct","rules":["geosite:cn"]}]`}}}, nodes: cutoverFakeNodes{overrides: map[int64]*node.ServerConfigOverride{}}}
	plan, err := preflightOrderedCutover(context.Background(), store, false)
	if err == nil || plan != nil || !strings.Contains(err.Error(), "internal Xray tag") {
		t.Fatalf("preflight %#v %v", plan, err)
	}
}
