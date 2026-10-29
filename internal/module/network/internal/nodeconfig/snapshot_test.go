package nodeconfig

import (
	"context"
	"errors"
	"testing"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
)

type snapshotOverrideReader struct {
	value *node.ServerConfigOverride
	calls int
}

func (s *snapshotOverrideReader) FindServerRoutingOverride(context.Context, int64) (*node.ServerConfigOverride, error) {
	s.calls++
	return s.value, nil
}
func TestSnapshotRetriesRevisionChangesAndFailsClosed(t *testing.T) {
	revisions := []int64{1, 2, 2, 2}
	calls := 0
	reader := &snapshotOverrideReader{}
	global, _, err := ResolveSnapshot(context.Background(), func(context.Context) (config.NodeConfig, error) {
		v := revisions[calls]
		calls++
		return config.NodeConfig{RoutingRevision: v}, nil
	}, reader, 7)
	if err != nil || global.RoutingRevision != 2 || reader.calls != 2 || calls != 4 {
		t.Fatalf("snapshot %v calls %d/%d err %v", global.RoutingRevision, calls, reader.calls, err)
	}
	calls = 0
	_, _, err = ResolveSnapshot(context.Background(), func(context.Context) (config.NodeConfig, error) {
		calls++
		return config.NodeConfig{RoutingRevision: int64(calls)}, nil
	}, reader, 7)
	if err == nil || calls != 8 {
		t.Fatalf("unstable snapshot error %v reads %d", err, calls)
	}
	_, _, err = ResolveSnapshot(context.Background(), func(context.Context) (config.NodeConfig, error) { return config.NodeConfig{}, errors.New("db down") }, reader, 7)
	if err == nil {
		t.Fatal("failed global read fell back")
	}
}
func TestDeliveryPolicyOnlyGlobalAndLocalV1(t *testing.T) {
	raw := `{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct"}`
	parsed, err := config.ParseNodeRouting([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	global := config.NodeConfig{Routing: parsed, RoutingRevision: 4}
	local := &node.ServerConfigOverride{Id: 1, ServerId: 7, RoutingRevision: 3}
	p, err := DeliveryPolicy(global, local, 7)
	if err != nil || p.Mode != "ordered-v1-inherited" || p.SourceRevision != 4 || p.LocalTransitionRevision != 3 {
		t.Fatalf("global %#v %v", p, err)
	}
	local.Routing = &raw
	p, err = DeliveryPolicy(global, local, 7)
	if err != nil || p.Mode != "ordered-v1-local" || p.SourceRevision != 3 || p.SourceServerID != 7 {
		t.Fatalf("local %#v %v", p, err)
	}
	bad := `{`
	local.Routing = &bad
	if _, err = DeliveryPolicy(global, local, 7); err == nil {
		t.Fatal("bad local fell back")
	}
	global.Routing = nil
	local.Routing = &raw
	if _, err = DeliveryPolicy(global, local, 7); err == nil {
		t.Fatal("missing global accepted")
	}
}
