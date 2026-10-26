package bootstrap

import (
	"context"
	"encoding/json"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/random"
	"github.com/perfect-panel/server/pkg/slicesx"
)

func Node(ctx *Dependencies) {
	logger.Debug("Node config initialization")
	configs, err := ctx.Store.System().GetNodeConfig(context.Background())
	if err != nil {
		panic(err)
	}
	var nodeConfig config.NodeDBConfig
	config.SystemConfigSliceReflectToStruct(configs, &nodeConfig)
	c := config.NodeConfig{
		NodeSecret:             nodeConfig.NodeSecret,
		NodePullInterval:       nodeConfig.NodePullInterval,
		NodePushInterval:       nodeConfig.NodePushInterval,
		IPStrategy:             nodeConfig.IPStrategy,
		TrafficReportThreshold: nodeConfig.TrafficReportThreshold,
	}
	if nodeConfig.DNS != "" {
		var dns []config.NodeDNS
		mustUnmarshalNodeConfigJSON("DNS", nodeConfig.DNS, &dns)
		c.DNS = dns
	}
	if nodeConfig.Block != "" {
		var block []string
		mustUnmarshalNodeConfigJSON("domain block", nodeConfig.Block, &block)
		c.Block = slicesx.RemoveDuplicateElements(block...)
	}
	if nodeConfig.BlockIPs != "" {
		var blockIPs []string
		mustUnmarshalNodeConfigJSON("IP block", nodeConfig.BlockIPs, &blockIPs)
		c.BlockIPs = slicesx.RemoveDuplicateElements(blockIPs...)
	}
	if nodeConfig.BlockProtocols != "" {
		var blockProtocols []string
		mustUnmarshalNodeConfigJSON("protocol block", nodeConfig.BlockProtocols, &blockProtocols)
		c.BlockProtocols = slicesx.RemoveDuplicateElements(blockProtocols...)
	}
	if nodeConfig.Outbound != "" {
		var outbound []config.NodeOutbound
		mustUnmarshalNodeConfigJSON("outbound", nodeConfig.Outbound, &outbound)
		c.Outbound = outbound
	}

	ctx.updateConfig(func(current *config.Config) { current.Node = c })

	nodeMultiplierData, err := ctx.Store.System().FindNodeMultiplierConfig(context.Background())
	if err != nil {
		logger.Error("Get Node Multiplier Config Error: ", logger.Field("error", err.Error()))
		return
	}

	// Manager initialization
	if nodeMultiplierData.Id == 0 {
		if err := ctx.Store.System().Insert(context.Background(), &system.System{
			Key:      "NodeMultiplierConfig",
			Value:    "[]",
			Type:     "string",
			Desc:     "Node Multiplier Config",
			Category: "server",
		}); err != nil {
			logger.Errorf("Create Node Multiplier Config Error: %s", err.Error())
		}
		return
	}

	var periods []network.MultiplierPeriod
	if err := json.Unmarshal([]byte(nodeMultiplierData.Value), &periods); err != nil {
		logger.Error("Unmarshal Node Multiplier Config Error: ", logger.Field("error", err.Error()), logger.Field("value", nodeMultiplierData.Value))
	}
	if ctx.SetNodeMultiplierManager != nil {
		ctx.SetNodeMultiplierManager(network.NewMultiplierManager(periods))
	}
}

func mustUnmarshalNodeConfigJSON(field, value string, target any) {
	if err := json.Unmarshal([]byte(value), target); err != nil {
		logger.Errorf("[Node] Unmarshal %s config error: %s", field, err.Error())
		panic(err)
	}
}

// LegacyDefaultNodeSecret is the node secret the original seed data shipped. The
// value is public knowledge, so an installation still carrying it serves the
// node API — which hands out every user's subscription uuid — to anyone.
const LegacyDefaultNodeSecret = "12345678"

// nodeSecretLength yields about 190 bits over the 62-character alphabet.
const nodeSecretLength = 32

// NodeSecret provisions a random node secret when the database does not carry
// one yet, so a fresh installation never serves the node API with a guessable
// credential. It has to run after Migrate, which seeds the empty row, and
// before Node, which copies the value into the runtime config.
//
// An installation still holding LegacyDefaultNodeSecret is only reported, never
// rotated: every node was configured with that secret, so rotating it here would
// silently cut them off. The operator rotates it from the admin panel and
// reconfigures the nodes in the same window.
func NodeSecret(svcCtx *Dependencies) {
	logger.Debug("Node secret initialization")
	// The read and the write share a transaction so the read goes to the
	// database instead of Redis; GetNodeConfig is a cached query, and the write
	// below does not invalidate that cache.
	err := svcCtx.Store.InPlatformTx(context.Background(), func(store repository.PlatformStore) error {
		configs, err := store.System().GetNodeConfig(context.Background())
		if err != nil {
			return err
		}
		var nodeConfig config.NodeDBConfig
		config.SystemConfigSliceReflectToStruct(configs, &nodeConfig)

		switch nodeConfig.NodeSecret {
		case "":
			secret := random.KeyNew(nodeSecretLength, 1)
			if err := store.System().UpdateValueByCategoryKey(context.Background(), "server", "NodeSecret", secret); err != nil {
				return err
			}
			logger.Info("[NodeSecret] generated a random node secret, read it from the admin panel to configure nodes")
		case LegacyDefaultNodeSecret:
			logger.Error("[NodeSecret] the node secret is still the well-known default, rotate it from the admin panel and reconfigure every node")
		}
		return nil
	})
	if err != nil {
		logger.Errorf("[NodeSecret] provision error: %v", err.Error())
		panic(err)
	}
}
