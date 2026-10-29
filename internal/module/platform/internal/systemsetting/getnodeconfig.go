package systemsetting

import (
	"context"
	"strconv"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"github.com/perfect-panel/server/pkg/logger"
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
	configs, err := l.deps.System.GetNodeConfigUncached(l.ctx)
	if err != nil {
		l.Errorw("[GetNodeConfigLogic] GetNodeConfig get server config error: ", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "GetNodeConfig get server config error: %v", err.Error())
	}
	c, err := nodeConfigFromSystemEntries(configs)
	if err != nil {
		l.Errorw("[GetNodeConfigLogic] parse node config error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "parse node config error: %v", err)
	}
	if c.Routing == nil || c.RoutingRevision <= 0 || !cutoverComplete(configs) {
		return nil, errors.New("ordered routing cutover is incomplete")
	}
	return c, nil
}

func cutoverComplete(entries []*system.System) bool {
	for _, entry := range entries {
		if entry.Key == "RoutingCutover" {
			return entry.Value == "complete"
		}
	}
	return false
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
		RoutingCapabilities:    []int{1},
	}

	normal := []config.NodeDNS{}
	if dbConfig.DNS != "" {
		var err error
		normal, err = config.ParseNodeDNSJSON([]byte(dbConfig.DNS))
		if err != nil {
			return nil, errors.Wrap(err, "invalid DNS config")
		}
	}
	c.DNS = make([]dto.PlatformNodeDNSSnapshot, 0, len(normal))
	for _, d := range normal {
		c.DNS = append(c.DNS, dto.PlatformNodeDNSSnapshot{Proto: d.Proto, Address: d.Address, ServerName: d.ServerName, Domains: d.Domains})
	}
	seen := map[string]bool{}
	for _, item := range configs {
		switch item.Key {
		case "NodePullInterval", "NodePushInterval":
			if seen[item.Key] {
				return nil, errors.Errorf("duplicate %s setting", item.Key)
			}
			seen[item.Key] = true
			seconds, err := strconv.ParseInt(item.Value, 10, 64)
			if err != nil || (item.Type != "int" && item.Type != "int64") {
				return nil, errors.Errorf("invalid %s setting", item.Key)
			}
			if err := config.ValidateNodeInterval(item.Key, seconds); err != nil {
				return nil, err
			}
		case "Routing", "RoutingRevision":
			if seen[item.Key] {
				return nil, errors.Errorf("duplicate %s setting", item.Key)
			}
			seen[item.Key] = true
			if item.Key == "Routing" && item.Value != "" {
				policy, err := config.ParseNodeRouting([]byte(item.Value))
				if err != nil {
					return nil, errors.Wrap(err, "invalid global routing")
				}
				c.Routing = policy
			}
			if item.Key == "RoutingRevision" {
				revision, err := strconv.ParseInt(item.Value, 10, 64)
				if err != nil || revision < 0 {
					return nil, errors.Errorf("invalid global routing revision")
				}
				c.RoutingRevision = revision
			}
		}
	}
	c.RoutingMode = "global-v1"
	return c, nil
}
