package system

import (
	"fmt"
	"strconv"

	"github.com/perfect-panel/server/internal/config"
)

// ParseNodeConfig parses the active server settings. Legacy Block and Outbound
// rows are cutover backups and never participate in the runtime configuration.
func ParseNodeConfig(rows []*System) (config.NodeConfig, error) {
	var stored config.NodeDBConfig
	if err := config.DecodeSystemConfig(rows, &stored); err != nil {
		return config.NodeConfig{}, fmt.Errorf("decode node settings: %w", err)
	}
	parsed := config.NodeConfig{NodeSecret: stored.NodeSecret, NodePullInterval: stored.NodePullInterval, NodePushInterval: stored.NodePushInterval, TrafficReportThreshold: stored.TrafficReportThreshold, IPStrategy: stored.IPStrategy}
	if stored.DNS != "" {
		var err error
		parsed.DNS, err = config.ParseNodeDNSJSON([]byte(stored.DNS))
		if err != nil {
			return config.NodeConfig{}, fmt.Errorf("node setting DNS: %w", err)
		}
	}
	seen := map[string]bool{}
	cutover := false
	for _, row := range rows {
		if row == nil {
			continue
		}
		switch row.Key {
		case "NodePullInterval", "NodePushInterval", "Routing", "RoutingRevision", "RoutingCutover":
			if seen[row.Key] {
				return config.NodeConfig{}, fmt.Errorf("duplicate %s setting", row.Key)
			}
			seen[row.Key] = true
		}
		switch row.Key {
		case "NodePullInterval", "NodePushInterval":
			seconds, err := strconv.ParseInt(row.Value, 10, 64)
			if err != nil || (row.Type != "int" && row.Type != "int64") {
				return config.NodeConfig{}, fmt.Errorf("invalid %s setting", row.Key)
			}
			if err := config.ValidateNodeInterval(row.Key, seconds); err != nil {
				return config.NodeConfig{}, err
			}
		case "Routing":
			var err error
			parsed.Routing, err = config.ParseNodeRouting([]byte(row.Value))
			if err != nil {
				return config.NodeConfig{}, fmt.Errorf("node setting Routing: %w", err)
			}
		case "RoutingRevision":
			var err error
			parsed.RoutingRevision, err = strconv.ParseInt(row.Value, 10, 64)
			if err != nil || parsed.RoutingRevision <= 0 {
				return config.NodeConfig{}, fmt.Errorf("invalid global routing revision")
			}
		case "RoutingCutover":
			cutover = row.Value == "complete"
		}
	}
	if !cutover || parsed.Routing == nil || parsed.RoutingRevision <= 0 {
		return config.NodeConfig{}, fmt.Errorf("ordered routing cutover is incomplete")
	}
	return parsed, nil
}
