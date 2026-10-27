package systemsetting

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	platformlog "github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
)

type serializedPlatformConfigStore struct {
	mu      sync.Mutex
	systems map[string]*system.System
}

type serializedSettingsStore struct {
	owner *serializedPlatformConfigStore
}

func (s *serializedPlatformConfigStore) InSettingsTx(_ context.Context, fn func(SettingsStore) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fn(&serializedSettingsStore{owner: s})
}

func (r *serializedSettingsStore) GetNodeConfigForUpdate(context.Context) ([]*system.System, error) {
	result := make([]*system.System, 0, len(r.owner.systems))
	for _, item := range r.owner.systems {
		clone := *item
		result = append(result, &clone)
	}
	return result, nil
}

func (r *serializedSettingsStore) UpdateValueByCategoryKey(_ context.Context, category, key, value string, valueType ...string) error {
	item, ok := r.owner.systems[key]
	if !ok {
		typeName := "string"
		if len(valueType) > 0 {
			typeName = valueType[0]
		}
		item = &system.System{Category: category, Key: key, Type: typeName}
		r.owner.systems[key] = item
	}
	item.Value = value
	return nil
}

func (*serializedSettingsStore) Insert(context.Context, *platformlog.SystemLog) error { return nil }

func TestConcurrentGlobalOldClientCannotLoseNewStructuredRules(t *testing.T) {
	store := &serializedPlatformConfigStore{systems: map[string]*system.System{
		"NodeSecret": {Category: "server", Key: "NodeSecret", Value: "secret", Type: "string"},
		"Block":      {Category: "server", Key: "Block", Value: `[]`, Type: "string"},
		"Outbound":   {Category: "server", Key: "Outbound", Value: `[]`, Type: "string"},
	}}
	service := NewService(Deps{Store: store})
	bind := func(body string) *dto.NodeConfig {
		var req dto.NodeConfig
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatal(err)
		}
		return &req
	}
	requests := []*dto.NodeConfig{
		bind(`{"block":[],"block_ips":["geoip:cn"],"block_protocols":[],"outbound":[]}`),
		bind(`{"block":["legacy.example"],"outbound":[]}`),
	}
	start := make(chan struct{})
	errs := make(chan error, len(requests))
	var wg sync.WaitGroup
	for _, req := range requests {
		wg.Add(1)
		go func(req *dto.NodeConfig) {
			defer wg.Done()
			<-start
			errs <- service.UpdateNodeConfig(context.Background(), req)
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
	blockIPs := store.systems["BlockIPs"].Value
	store.mu.Unlock()
	if blockIPs != `["geoip:cn"]` {
		t.Fatalf("BlockIPs = %s, want preserved concurrent explicit update", blockIPs)
	}
}
