package serverapi

import (
	"context"
	"testing"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
)

type snapshotServerReader struct{}

func (snapshotServerReader) FindOneServer(context.Context, int64) (*node.Server, error) {
	return &node.Server{Id: 1, Protocols: `[]`}, nil
}

type snapshotOverrideReader struct{}

func (snapshotOverrideReader) FindServerConfigOverride(context.Context, int64) (*node.ServerConfigOverride, error) {
	return nil, nil
}

func TestQueryServerProtocolConfigUsesOneConfigSnapshot(t *testing.T) {
	calls := 0
	service := NewService(Deps{
		Servers:   snapshotServerReader{},
		Overrides: snapshotOverrideReader{},
		Config: func() Snapshot {
			calls++
			return Snapshot{Node: config.NodeConfig{
				TrafficReportThreshold: int64(calls),
				NodePushInterval:       int64(calls),
				NodePullInterval:       int64(calls),
			}}
		},
	})
	response, err := service.QueryServerProtocolConfig(context.Background(), &dto.QueryServerConfigRequest{ServerID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("Config() calls = %d, want one per query", calls)
	}
	if response.TrafficReportThreshold != 1 || response.PushInterval != 1 || response.PullInterval != 1 {
		t.Fatalf("response used mixed snapshots: %#v", response)
	}
}
