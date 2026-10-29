package serverapi

import (
	"context"
	"testing"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/repository"
)

type snapshotNodeRepo struct{ repository.NodeRepo }

func (snapshotNodeRepo) FindOneServer(context.Context, int64) (*node.Server, error) {
	return &node.Server{Id: 1, Protocols: `[]`}, nil
}
func (snapshotNodeRepo) FindServerRoutingOverride(context.Context, int64) (*node.ServerConfigOverride, error) {
	return nil, nil
}

type snapshotStore struct {
	repository.NetworkTransactor
	node repository.NodeRepo
}

func (s snapshotStore) Node() repository.NodeRepo                       { return s.node }
func (snapshotStore) Inbox() repository.InboxRepo                       { return nil }
func (snapshotStore) Subscribe() repository.SubscribeRepo               { return nil }
func (snapshotStore) User() repository.UserRepo                         { return nil }
func (snapshotStore) UserSubscription() repository.UserSubscriptionRepo { return nil }

func TestQueryServerProtocolConfigUsesAuthoritativeDoubleRead(t *testing.T) {
	calls := 0
	deps := Deps{
		Store: snapshotStore{node: snapshotNodeRepo{}},
		ReadGlobalNodeConfig: func(context.Context) (config.NodeConfig, error) {
			calls++
			return config.NodeConfig{TrafficReportThreshold: 1, NodePushInterval: 1, NodePullInterval: 1, RoutingRevision: 1, Routing: &config.NodeRouting{Version: 1, Outbounds: []config.NodeOutbound{}, Rules: []config.NodeRoute{}, DefaultOutboundID: "direct"}}, nil
		},
		Config: func() Snapshot { panic("routing must not use runtime config") },
	}
	response, err := newQueryServerProtocolConfigLogic(context.Background(), deps).QueryServerProtocolConfig(&dto.QueryServerConfigRequest{ServerID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("authoritative global reads = %d, want two per query", calls)
	}
	if response.TrafficReportThreshold != 1 || response.PushInterval != 1 || response.PullInterval != 1 {
		t.Fatalf("response used mixed snapshots: %#v", response)
	}
}
