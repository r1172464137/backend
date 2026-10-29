package systemsetting

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
)

// ReadNodeConfigSnapshot reads the authoritative platform-owned server
// category without Redis or a process snapshot. NodeSecret is intentionally
// excluded from this cross-module read port.
func (s *Service) ReadNodeConfigSnapshot(ctx context.Context) (config.NodeConfig, error) {
	value, err := newGetNodeConfigLogic(ctx, s.deps).GetNodeConfig()
	if err != nil {
		return config.NodeConfig{}, err
	}
	out := config.NodeConfig{
		NodePullInterval: value.NodePullInterval, NodePushInterval: value.NodePushInterval,
		TrafficReportThreshold: value.TrafficReportThreshold, IPStrategy: value.IPStrategy,
		Routing: value.Routing, RoutingRevision: value.RoutingRevision,
	}
	out.DNS = make([]config.NodeDNS, 0, len(value.DNS))
	for _, d := range value.DNS {
		out.DNS = append(out.DNS, config.NodeDNS{Proto: d.Proto, Address: d.Address, ServerName: d.ServerName, Domains: d.Domains})
	}
	return out, nil
}
