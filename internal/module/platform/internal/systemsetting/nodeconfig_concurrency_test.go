package systemsetting

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type serializedPlatformConfigStore struct {
	mu      sync.Mutex
	systems map[string]*system.System
}
type serializedPlatformView struct {
	repository.PlatformStore
	system repository.SystemRepo
}

func (v serializedPlatformView) System() repository.SystemRepo { return v.system }
func (s *serializedPlatformConfigStore) InPlatformTx(_ context.Context, fn func(repository.PlatformStore) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fn(serializedPlatformView{system: &serializedSystemRepo{owner: s}})
}

type serializedSystemRepo struct {
	repository.SystemRepo
	owner *serializedPlatformConfigStore
}

func (r *serializedSystemRepo) GetNodeConfigForUpdate(context.Context) ([]*system.System, error) {
	result := make([]*system.System, 0, len(r.owner.systems))
	for _, item := range r.owner.systems {
		if item.Key == "Block" || item.Key == "BlockIPs" || item.Key == "BlockProtocols" || item.Key == "Outbound" {
			continue
		}
		clone := *item
		result = append(result, &clone)
	}
	return result, nil
}
func (r *serializedSystemRepo) GetNodeConfigUncached(ctx context.Context) ([]*system.System, error) {
	return r.GetNodeConfigForUpdate(ctx)
}
func (r *serializedSystemRepo) UpdateValueByCategoryKey(_ context.Context, category, key, value string, valueType ...string) error {
	item, ok := r.owner.systems[key]
	if !ok {
		typ := "string"
		if len(valueType) > 0 {
			typ = valueType[0]
		}
		item = &system.System{Category: category, Key: key, Type: typ}
		r.owner.systems[key] = item
	}
	item.Value = value
	return nil
}
func testPlatformStore() *serializedPlatformConfigStore {
	return &serializedPlatformConfigStore{systems: map[string]*system.System{
		"NodeSecret":      {Category: "server", Key: "NodeSecret", Value: "secret", Type: "string"},
		"Block":           {Category: "server", Key: "Block", Value: `["geosite:cn"]`, Type: "string"},
		"Routing":         {Category: "server", Key: "Routing", Value: `{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"direct"}`, Type: "string"},
		"RoutingRevision": {Category: "server", Key: "RoutingRevision", Value: "2", Type: "int64"},
		"RoutingCutover":  {Category: "server", Key: "RoutingCutover", Value: "complete", Type: "string"},
	}}
}
func TestConcurrentGlobalOrderedRevision(t *testing.T) {
	store := testPlatformStore()
	deps := Deps{Store: store}
	bind := func(def string) *dto.NodeConfig {
		var req dto.NodeConfig
		body := `{"routing_revision":2,"routing":{"version":1,"outbounds":[],"rules":[],"default_outbound_id":"` + def + `"}}`
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatal(err)
		}
		return &req
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, req := range []*dto.NodeConfig{bind("direct"), bind("block")} {
		wg.Add(1)
		go func(req *dto.NodeConfig) {
			defer wg.Done()
			<-start
			errs <- newUpdateNodeConfigLogic(context.Background(), deps).UpdateNodeConfig(req)
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
		var c *xerr.CodeError
		if errors.As(errors.Cause(err), &c) && c.GetErrCode() == xerr.PlatformRoutingRevisionConflict {
			conflicts++
		} else {
			t.Fatalf("unexpected error %v", err)
		}
	}
	if success != 1 || conflicts != 1 || store.systems["RoutingRevision"].Value != "3" || store.systems["Block"].Value != `["geosite:cn"]` {
		t.Fatalf("success=%d conflict=%d data=%#v", success, conflicts, store.systems)
	}
}
