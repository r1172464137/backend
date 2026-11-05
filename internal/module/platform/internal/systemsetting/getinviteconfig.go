package systemsetting

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

// GetInviteConfig returns the stored invitation settings.
func (s *Service) GetInviteConfig(ctx context.Context) (*dto.InviteConfig, error) {
	configs, err := s.deps.System.GetInviteConfig(ctx)
	if err != nil {
		logger.WithContext(ctx).Errorw("[GetInviteConfig] query the invite config failed", logger.Field("error", err.Error()))
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "get invite config error: %v", err.Error())
	}
	resp := &dto.InviteConfig{}
	config.SystemConfigSliceReflectToStruct(configs, resp)
	return resp, nil
}
