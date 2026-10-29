package serverapi

import (
	"context"
	"fmt"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/network/internal/nodeconfig"
	"github.com/perfect-panel/server/pkg/logger"
)

type QueryServerProtocolConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewQueryServerProtocolConfigLogic Get Server Protocol Config
func newQueryServerProtocolConfigLogic(ctx context.Context, deps Deps) *QueryServerProtocolConfigLogic {
	return &QueryServerProtocolConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *QueryServerProtocolConfigLogic) QueryServerProtocolConfig(req *dto.QueryServerConfigRequest) (resp *dto.QueryServerConfigResponse, err error) {
	// find server
	data, err := l.deps.Store.Node().FindOneServer(l.ctx, req.ServerID)
	if err != nil {
		l.Errorf("[GetServerProtocols] FindOneServer Error: %s", err.Error())
		return nil, err
	}

	// Never sanitize an enabled invalid listener into an apparently valid
	// zero-protocol response; only explicitly disabled entries may disappear.
	if err := node.ValidateNodeProtocols(data); err != nil {
		return nil, err
	}
	// handler protocols
	var protocols []dto.Protocol
	dst, err := data.UnmarshalProtocols()
	if err != nil {
		l.Errorf("[FilterServerList] UnmarshalProtocols Error: %s", err.Error())
		return nil, err
	}
	dst = node.SanitizeProtocolsForNodeDistribution(dst)
	mapping.DeepCopy(&protocols, dst)

	// only return enabled protocols for node distribution
	enabledProtocols := make([]dto.Protocol, 0, len(protocols))
	for _, p := range protocols {
		if p.Enable {
			enabledProtocols = append(enabledProtocols, p)
		}
	}
	protocols = enabledProtocols

	// filter by req.Protocols

	if len(req.Protocols) > 0 {
		filtered := make([]dto.Protocol, 0, len(protocols))
		protocolSet := make(map[string]struct{})
		for _, p := range req.Protocols {
			protocol, err := node.NormalizeProtocolForStorage(node.Protocol{Type: p})
			if err != nil {
				protocolSet[p] = struct{}{}
				continue
			}
			protocolSet[protocol.Type] = struct{}{}
		}
		for _, p := range protocols {
			if _, exists := protocolSet[p.Type]; exists {
				filtered = append(filtered, p)
			}
		}
		if len(filtered) == 0 && len(protocols) > 0 {
			return nil, fmt.Errorf("protocol filter excludes all enabled listeners")
		}
		protocols = filtered
	}

	nodeSnapshot, override, err := nodeconfig.ResolveSnapshot(l.ctx, l.deps.ReadGlobalNodeConfig, l.deps.Store.Node(), req.ServerID)
	if err != nil {
		l.Errorf("[GetServerProtocols] authoritative snapshot error: %s", err.Error())
		return nil, err
	}
	nodeValues := nodeconfig.GlobalValues(nodeSnapshot)
	if override != nil {
		if err = nodeconfig.ApplyOverride(&nodeValues, override); err != nil {
			l.Errorf("[GetServerProtocols] ApplyOverride Error: %s", err.Error())
			return nil, err
		}
	}

	routing, err := nodeconfig.DeliveryPolicy(nodeSnapshot, override, req.ServerID)
	if err != nil {
		return nil, err
	}
	return &dto.QueryServerConfigResponse{
		TrafficReportThreshold: nodeSnapshot.TrafficReportThreshold,
		PushInterval:           nodeSnapshot.NodePushInterval,
		PullInterval:           nodeSnapshot.NodePullInterval,
		IPStrategy:             nodeValues.IPStrategy,
		DNS:                    nodeValues.DNS,
		Protocols:              protocols,
		Total:                  int64(len(protocols)),
		Routing:                routing,
	}, nil
}
