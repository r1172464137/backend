package systemsetting

import (
	"context"

	"fmt"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/repository"
)

// ReadNodeConfigSnapshot reads the authoritative platform-owned server
// category without Redis or a process snapshot. NodeSecret is intentionally
// excluded from this cross-module read port.
func (s *Service) ReadNodeConfigSnapshot(ctx context.Context) (config.NodeConfig, error) {
	return ReadNodeConfigSnapshotFromRepo(ctx, s.deps.System)
}

// ReadNodeConfigSnapshotFromRepo uses the caller's transaction-bound repository.
func ReadNodeConfigSnapshotFromRepo(ctx context.Context, repo repository.SystemRepo) (config.NodeConfig, error) {
	entries, err := repo.GetNodeConfigUncached(ctx)
	if err != nil {
		return config.NodeConfig{}, err
	}
	value, err := nodeConfigFromSystemEntries(entries)
	if err != nil {
		return config.NodeConfig{}, err
	}
	if value.Routing == nil || value.RoutingRevision <= 0 || !cutoverComplete(entries) {
		return config.NodeConfig{}, fmt.Errorf("ordered routing cutover is incomplete")
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
