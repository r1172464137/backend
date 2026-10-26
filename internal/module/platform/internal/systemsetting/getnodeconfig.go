package systemsetting

import (
	"context"
	"encoding/json"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/slicesx"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetNodeConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newGetNodeConfigLogic(ctx context.Context, deps Deps) *GetNodeConfigLogic {
	return &GetNodeConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetNodeConfigLogic) GetNodeConfig() (*dto.NodeConfig, error) {
	// get server config from db
	configs, err := l.deps.System.GetNodeConfig(l.ctx)
	if err != nil {
		l.Errorw("[GetNodeConfigLogic] GetNodeConfig get server config error: ", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "GetNodeConfig get server config error: %v", err.Error())
	}
	c, err := nodeConfigFromSystemEntries(configs)
	if err != nil {
		l.Errorw("[GetNodeConfigLogic] parse node config error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "parse node config error: %v", err)
	}
	return c, nil
}

func nodeConfigFromSystemEntries(configs []*system.System) (*dto.NodeConfig, error) {
	var dbConfig config.NodeDBConfig
	config.SystemConfigSliceReflectToStruct(configs, &dbConfig)
	c := &dto.NodeConfig{
		NodeSecret:             dbConfig.NodeSecret,
		NodePullInterval:       dbConfig.NodePullInterval,
		NodePushInterval:       dbConfig.NodePushInterval,
		IPStrategy:             dbConfig.IPStrategy,
		TrafficReportThreshold: dbConfig.TrafficReportThreshold,
		DNS:                    []dto.PlatformNodeDNSSnapshot{},
		Block:                  []string{},
		BlockIPs:               []string{},
		BlockProtocols:         []string{},
		Outbound:               []dto.PlatformNodeOutboundSnapshot{},
	}

	if dbConfig.DNS != "" {
		if err := json.Unmarshal([]byte(dbConfig.DNS), &c.DNS); err != nil {
			return nil, errors.Wrap(err, "unmarshal DNS config")
		}
	}
	if dbConfig.Block != "" {
		var block []string
		if err := json.Unmarshal([]byte(dbConfig.Block), &block); err != nil {
			return nil, errors.Wrap(err, "unmarshal domain block config")
		}
		c.Block = slicesx.RemoveDuplicateElements(block...)
	}
	if dbConfig.BlockIPs != "" {
		var blockIPs []string
		if err := json.Unmarshal([]byte(dbConfig.BlockIPs), &blockIPs); err != nil {
			return nil, errors.Wrap(err, "unmarshal IP block config")
		}
		c.BlockIPs = slicesx.RemoveDuplicateElements(blockIPs...)
	}
	if dbConfig.BlockProtocols != "" {
		var blockProtocols []string
		if err := json.Unmarshal([]byte(dbConfig.BlockProtocols), &blockProtocols); err != nil {
			return nil, errors.Wrap(err, "unmarshal protocol block config")
		}
		c.BlockProtocols = slicesx.RemoveDuplicateElements(blockProtocols...)
	}
	if dbConfig.Outbound != "" {
		if err := json.Unmarshal([]byte(dbConfig.Outbound), &c.Outbound); err != nil {
			return nil, errors.Wrap(err, "unmarshal outbound config")
		}
		for index := range c.Outbound {
			if c.Outbound[index].Rules == nil {
				c.Outbound[index].Rules = []string{}
			}
			if c.Outbound[index].IPRules == nil {
				c.Outbound[index].IPRules = []string{}
			}
			if c.Outbound[index].ProtocolRules == nil {
				c.Outbound[index].ProtocolRules = []string{}
			}
		}
	}

	return c, nil
}
