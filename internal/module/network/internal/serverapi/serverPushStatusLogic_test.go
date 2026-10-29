package serverapi

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/network/internal/routingstatus"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type statusNodeRepo struct {
	repository.NodeRepo
	server          *node.Server
	findCalls       int
	statusCalls     int
	protocolUpdates int
	cacheClears     int
}

func (r *statusNodeRepo) FindOneServer(context.Context, int64) (*node.Server, error) {
	r.findCalls++
	copy := *r.server
	return &copy, nil
}

func (r *statusNodeRepo) UpdateStatusCache(context.Context, int64, *node.Status) error {
	r.statusCalls++
	return nil
}

func (r *statusNodeRepo) UpdateServerProtocolsIfCurrent(_ context.Context, _ int64, current, updated string, _ ...*gorm.DB) (bool, error) {
	r.protocolUpdates++
	if r.server.Protocols != current {
		return false, nil
	}
	r.server.Protocols = updated
	return true, nil
}

func (r *statusNodeRepo) ClearServerCache(context.Context, int64) error {
	r.cacheClears++
	return nil
}

type statusStore struct {
	repository.Store
	nodes repository.NodeRepo
}

func (s *statusStore) Node() repository.NodeRepo { return s.nodes }

func TestServerRoutingReportsShareStateAcrossProtocolsAndZeroProtocols(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { client.Close() })
	nodes := &statusNodeRepo{server: &node.Server{Id: 7, Protocols: `[]`}}
	logic := newServerPushStatusLogic(context.Background(), Deps{Store: &statusStore{nodes: nodes}, Redis: client})
	id := "11111111-1111-4111-8111-111111111111"
	first := &dto.ServerPushStatusRequest{ServerCommon: dto.ServerCommon{ServerId: 7}, UpdatedAt: 10, RoutingVersion: 1, RoutingSequence: 2, InstanceID: id, RoutingApplied: &dto.RoutingCandidateIdentity{Mode: "ordered-v1-inherited", Source: "global", SourceRevision: 2}}
	if err := logic.ServerPushStatus(first); err != nil {
		t.Fatal(err)
	}
	older := *first
	older.Protocol = "vless"
	older.UpdatedAt = 9
	older.RoutingSequence = 1
	older.RoutingApplied = &dto.RoutingCandidateIdentity{Mode: "ordered-v1-inherited", Source: "global", SourceRevision: 1}
	if err := logic.ServerPushStatus(&older); err != nil {
		t.Fatal(err)
	}
	status, err := routingstatus.Read(context.Background(), client, 7)
	if err != nil || !status.Known || status.Report.Applied.SourceRevision != 2 {
		t.Fatalf("out-of-order report %#v %v", status, err)
	}
	other := *first
	other.Protocol = "trojan"
	other.InstanceID = "22222222-2222-4222-8222-222222222222"
	if err := logic.ServerPushStatus(&other); err != nil {
		t.Fatal(err)
	}
	status, err = routingstatus.Read(context.Background(), client, 7)
	if err != nil || status.Known {
		t.Fatalf("multiple instances %#v %v", status, err)
	}
}

func TestServerPushStatusDefersLastReportedDatabaseWrite(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	nodes := &statusNodeRepo{server: &node.Server{Id: 7}}
	logic := newServerPushStatusLogic(context.Background(), Deps{Store: &statusStore{nodes: nodes}, Redis: client})
	if err := logic.ServerPushStatus(&dto.ServerPushStatusRequest{ServerCommon: dto.ServerCommon{ServerId: 7}}); err != nil {
		t.Fatalf("ServerPushStatus() error = %v", err)
	}
	if nodes.findCalls != 1 || nodes.statusCalls != 1 {
		t.Fatalf("find/status calls = %d/%d, want 1/1", nodes.findCalls, nodes.statusCalls)
	}
	if nodes.protocolUpdates != 0 || nodes.cacheClears != 0 {
		t.Fatalf("ordinary heartbeat wrote protocols or cleared cache: updates=%d clears=%d", nodes.protocolUpdates, nodes.cacheClears)
	}
	if got := client.HGet(context.Background(), "traffic:server:last_reported", "7").Val(); got == "" {
		t.Fatal("heartbeat did not stage last_reported_at in Redis")
	}
}
