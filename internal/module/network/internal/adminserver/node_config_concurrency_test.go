package adminserver

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/repository"
)

type serializedAdminNodeStore struct {
	mu       sync.Mutex
	override *node.ServerConfigOverride
	nodeRepo *serializedNodeRepo
}

type serializedNetworkView struct {
	repository.NetworkStore
	node repository.NodeRepo
}

func (v serializedNetworkView) Node() repository.NodeRepo { return v.node }

func newSerializedAdminNodeStore() *serializedAdminNodeStore {
	store := &serializedAdminNodeStore{}
	store.nodeRepo = &serializedNodeRepo{owner: store}
	return store
}

func (s *serializedAdminNodeStore) InNetworkTx(_ context.Context, fn func(repository.NetworkStore) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fn(serializedNetworkView{node: s.nodeRepo})
}
func (s *serializedAdminNodeStore) Node() repository.NodeRepo { return s.nodeRepo }
func (s *serializedAdminNodeStore) UserSubscription() repository.UserSubscriptionRepo {
	return nil
}

type serializedNodeRepo struct {
	repository.NodeRepo
	owner *serializedAdminNodeStore
}

func (r *serializedNodeRepo) FindOneServerForUpdate(context.Context, int64) (*node.Server, error) {
	return &node.Server{Id: 1}, nil
}
func (r *serializedNodeRepo) FindServerConfigOverride(context.Context, int64) (*node.ServerConfigOverride, error) {
	if r.owner.override == nil {
		return nil, nil
	}
	clone := *r.owner.override
	return &clone, nil
}
func (r *serializedNodeRepo) SaveServerConfigOverride(_ context.Context, value *node.ServerConfigOverride) error {
	clone := *value
	clone.Id = 1
	r.owner.override = &clone
	return nil
}
func (r *serializedNodeRepo) DeleteServerConfigOverride(context.Context, int64) error {
	r.owner.override = nil
	return nil
}
func (r *serializedNodeRepo) ClearServerCache(context.Context, int64) error { return nil }

func TestConcurrentOverrideOldClientCannotLoseNewStructuredRules(t *testing.T) {
	store := newSerializedAdminNodeStore()
	deps := Deps{Store: store, Config: func() Snapshot { return Snapshot{Node: config.NodeConfig{}} }}
	bind := func(body string) *dto.UpdateServerNodeConfigRequest {
		var req dto.UpdateServerNodeConfigRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatal(err)
		}
		return &req
	}
	requests := []*dto.UpdateServerNodeConfigRequest{
		bind(`{"server_id":1,"inherit_ip_strategy":true,"inherit_dns":true,"inherit_block":false,"block":[],"block_ips":["geoip:cn"],"block_protocols":[],"inherit_outbound":true}`),
		bind(`{"server_id":1,"inherit_ip_strategy":true,"inherit_dns":true,"inherit_block":false,"block":["legacy.example"],"inherit_outbound":true}`),
	}
	start := make(chan struct{})
	errs := make(chan error, len(requests))
	var wg sync.WaitGroup
	for _, req := range requests {
		wg.Add(1)
		go func(req *dto.UpdateServerNodeConfigRequest) {
			defer wg.Done()
			<-start
			errs <- NewService(deps).UpdateServerNodeConfig(context.Background(), req)
		}(req)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent update error = %v", err)
		}
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.override == nil || store.override.BlockIPs == nil || *store.override.BlockIPs != `["geoip:cn"]` {
		t.Fatalf("override = %#v, want preserved concurrent block_ips", store.override)
	}
}
