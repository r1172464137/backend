package serverapi

import (
	"context"
	"fmt"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/network/internal/nodeconfig"
	"github.com/perfect-panel/server/internal/module/network/internal/protocolmap"
	"github.com/perfect-panel/server/pkg/logger"
)

// QueryServerProtocolConfig returns the configuration a node pulls for its
// server: the enabled protocols, only the requested ones when the node names
// any, with the node settings in effect after the server's override.
func (s *Service) QueryServerProtocolConfig(ctx context.Context, req *dto.QueryServerConfigRequest) (*dto.QueryServerConfigResponse, error) {
	log := logger.WithContext(ctx)
	data, err := s.deps.Servers.FindOneServer(ctx, req.ServerID)
	if err != nil {
		log.Errorf("[QueryServerProtocolConfig] FindOneServer Error: %s", err.Error())
		return nil, err
	}

	// Never turn an invalid enabled listener into an empty response.
	if err := node.ValidateNodeProtocols(data); err != nil {
		return nil, err
	}
	dst, err := data.UnmarshalProtocols()
	if err != nil {
		log.Errorf("[QueryServerProtocolConfig] UnmarshalProtocols Error: %s", err.Error())
		return nil, err
	}
	protocols, err := protocolmap.ToDTO(node.SanitizeProtocolsForNodeDistribution(dst))
	if err != nil {
		return nil, err
	}

	enabledProtocols := make([]dto.Protocol, 0, len(protocols))
	for _, p := range protocols {
		if p.Enable {
			enabledProtocols = append(enabledProtocols, p)
		}
	}
	protocols = enabledProtocols

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

	nodeSnapshot, override, err := nodeconfig.ResolveSnapshot(ctx, s.deps.ReadGlobalNodeConfig, s.deps.Store.Node(), req.ServerID)
	if err != nil {
		log.Errorf("[QueryServerProtocolConfig] authoritative snapshot error: %s", err.Error())
		return nil, err
	}
	nodeValues := nodeconfig.GlobalValues(nodeSnapshot)
	if override != nil {
		if err = nodeconfig.ApplyOverride(&nodeValues, override); err != nil {
			log.Errorf("[QueryServerProtocolConfig] ApplyOverride Error: %s", err.Error())
			return nil, err
		}
	}

	routing, err := nodeconfig.DeliveryPolicy(nodeSnapshot, override, req.ServerID)
	if err != nil {
		return nil, err
	}
	response := &dto.QueryServerConfigResponse{
		TrafficReportThreshold: nodeSnapshot.TrafficReportThreshold,
		PushInterval:           nodeSnapshot.NodePushInterval,
		PullInterval:           nodeSnapshot.NodePullInterval,
		IPStrategy:             nodeValues.IPStrategy,
		DNS:                    nodeValues.DNS,
		Protocols:              protocols,
		Total:                  int64(len(protocols)),
		Routing:                routing,
	}
	if routing.Version == config.RoutingVersion2 && req.RoutingVersion >= 2 {
		return newQueryServerProtocolConfigLogic(ctx, s.deps).v2Delivery(req, response)
	}
	return response, nil
}
