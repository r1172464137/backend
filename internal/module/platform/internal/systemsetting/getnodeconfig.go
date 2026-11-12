package systemsetting

import (
	"context"
	"strconv"

	appconfig "github.com/perfect-panel/server/internal/config"
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
		return nil, errors.New("routing format 3 migration is incomplete")
	}
	return c, nil
}

func cutoverComplete(entries []*system.System) bool {
	for _, entry := range entries {
		if entry.Key == "RoutingCutover" {
			return entry.Value == "complete-v3"
		}
	}
	return false
}

func nodeConfigFromSystemEntries(configs []*system.System) (*dto.NodeConfig, error) {
	var dbConfig appconfig.NodeDBConfig
	appconfig.SystemConfigSliceReflectToStruct(configs, &dbConfig)
	c := &dto.NodeConfig{
		NodeSecret:             dbConfig.NodeSecret,
		NodePullInterval:       dbConfig.NodePullInterval,
		NodePushInterval:       dbConfig.NodePushInterval,
		TrafficReportThreshold: dbConfig.TrafficReportThreshold,
		RoutingCapabilities:    []int{appconfig.RoutingVersion},
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
			if err := appconfig.ValidateNodeInterval(item.Key, seconds); err != nil {
				return nil, err
			}
		case "Routing", "RoutingRevision":
			if seen[item.Key] {
				return nil, errors.Errorf("duplicate %s setting", item.Key)
			}
			seen[item.Key] = true
			if item.Key == "Routing" && item.Value != "" {
				policy, err := appconfig.ParseNodeRouting([]byte(item.Value))
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
	c.RoutingMode = "global"
	return c, nil
}
