package systemsetting

import (
	"context"
	"fmt"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"github.com/perfect-panel/server/pkg/xerr"
)

// GetNodeConfig reads the authoritative active settings for authorized administrators.
func (s *Service) GetNodeConfig(ctx context.Context) (*dto.NodeConfig, error) {
	rows, err := s.deps.System.GetNodeConfigUncached(ctx)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "get node config: %v", err)
	}
	view, err := nodeConfigFromSystemEntries(rows)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.ERROR, "parse node config: %v", err)
	}
	return view, nil
}

func nodeConfigFromSystemEntries(rows []*system.System) (*dto.NodeConfig, error) {
	parsed, err := system.ParseNodeConfig(rows)
	if err != nil {
		return nil, err
	}
	return nodeConfigView(parsed), nil
}

func nodeConfigView(c config.NodeConfig) *dto.NodeConfig {
	view := &dto.NodeConfig{NodeSecret: c.NodeSecret, NodePullInterval: c.NodePullInterval, NodePushInterval: c.NodePushInterval, TrafficReportThreshold: c.TrafficReportThreshold, IPStrategy: c.IPStrategy, Routing: c.Routing, RoutingRevision: c.RoutingRevision, RoutingMode: fmt.Sprintf("global-v%d", c.Routing.Version), RoutingCapabilities: []int{1, 2}, DNS: make([]dto.PlatformNodeDNSSnapshot, 0, len(c.DNS))}
	for _, dns := range c.DNS {
		view.DNS = append(view.DNS, dto.PlatformNodeDNSSnapshot(dns))
	}
	return view
}
