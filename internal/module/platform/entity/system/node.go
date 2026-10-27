package system

import (
	"encoding/json"
	"fmt"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/slicesx"
)

// ParseNodeConfig decodes the node configuration stored in the "server"
// settings rows. Startup, the admin reload and the admin settings read all
// parse it here, so they agree on every rule.
//
// DNS, all block categories and Outbound hold JSON documents; malformed
// persisted JSON is reported rather than silently weakening routing. A scalar
// setting that does not decode keeps its zero value and is logged with its key.
func ParseNodeConfig(rows []*System) (config.NodeConfig, error) {
	var stored config.NodeDBConfig
	if err := config.DecodeSystemConfig(rows, &stored); err != nil {
		logger.Errorw("[NodeConfig] stored node settings could not be applied, the affected fields keep their zero value",
			logger.Field("error", err.Error()))
	}
	parsed := config.NodeConfig{
		NodeSecret:             stored.NodeSecret,
		NodePullInterval:       stored.NodePullInterval,
		NodePushInterval:       stored.NodePushInterval,
		TrafficReportThreshold: stored.TrafficReportThreshold,
		IPStrategy:             stored.IPStrategy,
	}
	if stored.DNS != "" {
		if err := json.Unmarshal([]byte(stored.DNS), &parsed.DNS); err != nil {
			return config.NodeConfig{}, fmt.Errorf("node setting DNS: %w", err)
		}
	}
	if stored.Block != "" {
		var block []string
		if err := json.Unmarshal([]byte(stored.Block), &block); err != nil {
			return config.NodeConfig{}, fmt.Errorf("node setting Block: %w", err)
		}
		parsed.Block = slicesx.RemoveDuplicateElements(block...)
	}
	if stored.BlockIPs != "" {
		var blockIPs []string
		if err := json.Unmarshal([]byte(stored.BlockIPs), &blockIPs); err != nil {
			return config.NodeConfig{}, fmt.Errorf("node setting BlockIPs: %w", err)
		}
		parsed.BlockIPs = slicesx.RemoveDuplicateElements(blockIPs...)
	}
	if stored.BlockProtocols != "" {
		var blockProtocols []string
		if err := json.Unmarshal([]byte(stored.BlockProtocols), &blockProtocols); err != nil {
			return config.NodeConfig{}, fmt.Errorf("node setting BlockProtocols: %w", err)
		}
		parsed.BlockProtocols = slicesx.RemoveDuplicateElements(blockProtocols...)
	}
	if stored.Outbound != "" {
		if err := json.Unmarshal([]byte(stored.Outbound), &parsed.Outbound); err != nil {
			return config.NodeConfig{}, fmt.Errorf("node setting Outbound: %w", err)
		}
	}
	return parsed, nil
}
