package serverapi

import (
	"context"

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
	var enabledProtocols []dto.Protocol
	for _, p := range protocols {
		if p.Enable {
			enabledProtocols = append(enabledProtocols, p)
		}
	}
	protocols = enabledProtocols

	// filter by req.Protocols

	if len(req.Protocols) > 0 {
		var filtered []dto.Protocol
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
		protocols = filtered
	}

	nodeSnapshot := l.deps.Config().Node
	nodeValues := nodeconfig.GlobalValues(nodeSnapshot)
	override, err := l.deps.Store.Node().FindServerConfigOverride(l.ctx, req.ServerID)
	if err != nil {
		l.Errorf("[GetServerProtocols] FindServerConfigOverride Error: %s", err.Error())
		return nil, err
	}
	if override != nil {
		if err = nodeconfig.ApplyOverride(&nodeValues, override); err != nil {
			l.Errorf("[GetServerProtocols] ApplyOverride Error: %s", err.Error())
			return nil, err
		}
	}

	return &dto.QueryServerConfigResponse{
		TrafficReportThreshold: nodeSnapshot.TrafficReportThreshold,
		PushInterval:           nodeSnapshot.NodePushInterval,
		PullInterval:           nodeSnapshot.NodePullInterval,
		IPStrategy:             nodeValues.IPStrategy,
		DNS:                    nodeValues.DNS,
		Block:                  nodeValues.Block,
		BlockIPs:               nodeValues.BlockIPs,
		BlockProtocols:         nodeValues.BlockProtocols,
		Outbound:               nodeValues.Outbound,
		Protocols:              protocols,
		Total:                  int64(len(protocols)),
	}, nil
}
