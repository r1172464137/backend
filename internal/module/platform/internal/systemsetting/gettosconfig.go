package systemsetting

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

// GetTosConfig returns the stored terms of service.
func (s *Service) GetTosConfig(ctx context.Context) (*dto.TosConfig, error) {
	configs, err := s.deps.System.GetTosConfig(ctx)
	if err != nil {
		logger.WithContext(ctx).Errorw("[GetTosConfig] query the tos config failed", logger.Field("error", err.Error()))
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "GetTosConfig error: %v", err.Error())
	}
	resp := &dto.TosConfig{}
	config.SystemConfigSliceReflectToStruct(configs, resp)
	return resp, nil
}
