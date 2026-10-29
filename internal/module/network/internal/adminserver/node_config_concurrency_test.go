package adminserver

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type serializedAdminNodeStore struct {
	mu              sync.Mutex
	override        *node.ServerConfigOverride
	nodeRepo        *serializedNodeRepo
	cacheClearCalls int
}
type serializedNetworkView struct {
	repository.NetworkStore
	node repository.NodeRepo
}

func (v serializedNetworkView) Node() repository.NodeRepo { return v.node }
func newSerializedAdminNodeStore() *serializedAdminNodeStore {
	s := &serializedAdminNodeStore{}
	s.nodeRepo = &serializedNodeRepo{owner: s}
	return s
}
func (s *serializedAdminNodeStore) InNetworkTx(_ context.Context, fn func(repository.NetworkStore) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fn(serializedNetworkView{node: s.nodeRepo})
}
func (s *serializedAdminNodeStore) Node() repository.NodeRepo                         { return s.nodeRepo }
func (s *serializedAdminNodeStore) UserSubscription() repository.UserSubscriptionRepo { return nil }

type serializedNodeRepo struct {
	repository.NodeRepo
	owner *serializedAdminNodeStore
}

func (r *serializedNodeRepo) FindOneServer(context.Context, int64) (*node.Server, error) {
	return &node.Server{Id: 1}, nil
}
func (r *serializedNodeRepo) FindOneServerForUpdate(context.Context, int64) (*node.Server, error) {
	return &node.Server{Id: 1}, nil
}
func (r *serializedNodeRepo) FindServerConfigOverride(context.Context, int64) (*node.ServerConfigOverride, error) {
	if r.owner.override == nil {
		return nil, nil
	}
	v := *r.owner.override
	return &v, nil
}
func (r *serializedNodeRepo) FindServerRoutingOverride(context.Context, int64) (*node.ServerConfigOverride, error) {
	if r.owner.override == nil {
		return nil, nil
	}
	v := *r.owner.override
	v.Block = nil
	v.BlockIPs = nil
	v.BlockProtocols = nil
	v.Outbound = nil
	return &v, nil
}
func (r *serializedNodeRepo) SaveServerRoutingOverride(_ context.Context, value *node.ServerConfigOverride) error {
	v := *value
	v.Id = 1
	if old := r.owner.override; old != nil {
		v.Block = old.Block
		v.BlockIPs = old.BlockIPs
		v.BlockProtocols = old.BlockProtocols
		v.Outbound = old.Outbound
	}
	r.owner.override = &v
	return nil
}
func (r *serializedNodeRepo) DeleteServerConfigOverride(context.Context, int64, ...*gorm.DB) error {
	r.owner.override = nil
	return nil
}
func (r *serializedNodeRepo) ClearServerCache(context.Context, int64) error {
	r.owner.cacheClearCalls++
	return nil
}

func TestConcurrentOrderedOverrideRejectsStaleRevision(t *testing.T) {
	store := newSerializedAdminNodeStore()
	old := `{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct"}`
	legacy := `["geosite:cn"]`
	store.override = &node.ServerConfigOverride{Id: 1, ServerId: 1, Routing: &old, RoutingRevision: 2, Block: &legacy}
	deps := Deps{Store: store}
	bind := func(def string) *dto.UpdateServerNodeConfigRequest {
		var req dto.UpdateServerNodeConfigRequest
		body := `{"server_id":1,"routing_revision":2,"routing":{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"` + def + `"}}`
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatal(err)
		}
		return &req
	}
	requests := []*dto.UpdateServerNodeConfigRequest{bind("direct"), bind("block")}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, req := range requests {
		wg.Add(1)
		go func(req *dto.UpdateServerNodeConfigRequest) {
			defer wg.Done()
			<-start
			errs <- newUpdateServerNodeConfigLogic(context.Background(), deps).UpdateServerNodeConfig(req)
		}(req)
	}
	close(start)
	wg.Wait()
	close(errs)
	success, conflicts := 0, 0
	for err := range errs {
		if err == nil {
			success++
			continue
		}
		var code *xerr.CodeError
		if errors.As(errors.Cause(err), &code) && code.GetErrCode() == xerr.NetworkRoutingRevisionConflict {
			conflicts++
		} else {
			t.Fatalf("unexpected error %v", err)
		}
	}
	if success != 1 || conflicts != 1 || store.override.RoutingRevision != 3 || store.override.Block == nil || *store.override.Block != legacy {
		t.Fatalf("success %d conflict %d override %#v", success, conflicts, store.override)
	}
}
