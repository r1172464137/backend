package nodeconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
)

type GlobalReader func(context.Context) (config.NodeConfig, error)
type OverrideReader interface {
	FindServerRoutingOverride(context.Context, int64) (*node.ServerConfigOverride, error)
}

// ResolveSnapshot double-reads the platform routing revision around the
// network-owned override read. No process cache or cross-domain transaction
// participates. Every route write must increment its owning revision.
func ResolveSnapshot(ctx context.Context, read GlobalReader, store OverrideReader, id int64) (config.NodeConfig, *node.ServerConfigOverride, error) {
	if read == nil || store == nil {
		return config.NodeConfig{}, nil, errors.New("authoritative routing readers unavailable")
	}
	for i := 0; i < 4; i++ {
		before, err := read(ctx)
		if err != nil {
			return config.NodeConfig{}, nil, err
		}
		override, err := store.FindServerRoutingOverride(ctx, id)
		if err != nil {
			return config.NodeConfig{}, nil, err
		}
		after, err := read(ctx)
		if err != nil {
			return config.NodeConfig{}, nil, err
		}
		if before.RoutingRevision == after.RoutingRevision {
			return after, override, nil
		}
	}
	return config.NodeConfig{}, nil, errors.New("global routing changed while resolving snapshot")
}

func DeliveryPolicy(global config.NodeConfig, override *node.ServerConfigOverride, serverID int64) (*dto.NodeRoutingPolicy, error) {
	if global.Routing == nil || global.RoutingRevision <= 0 {
		return nil, errors.New("global ordered routing is unavailable")
	}
	var policy *config.NodeRouting
	localRevision := int64(0)
	if override != nil {
		localRevision = override.RoutingRevision
		if localRevision < 0 {
			return nil, errors.New("invalid local routing revision")
		}
		if override.Routing != nil {
			var err error
			policy, err = config.ParseNodeRouting([]byte(*override.Routing))
			if err != nil {
				return nil, fmt.Errorf("invalid local routing: %w", err)
			}
		}
	}
	local := policy != nil
	if !local {
		policy = global.Routing
	}
	body, err := json.Marshal(policy)
	if err != nil {
		return nil, err
	}
	var result dto.NodeRoutingPolicy
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return nil, err
	}
	result.LocalTransitionRevision = localRevision
	if local {
		if serverID <= 0 || localRevision <= 0 {
			return nil, errors.New("invalid local routing identity")
		}
		result.Mode, result.Source, result.SourceServerID, result.SourceRevision = "ordered-v1-local", "server", serverID, localRevision
	} else {
		if global.RoutingRevision <= 0 {
			return nil, errors.New("invalid global routing revision")
		}
		result.Mode, result.Source, result.SourceRevision = "ordered-v1-inherited", "global", global.RoutingRevision
	}
	return &result, nil
}
