package systemsetting

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

// UpdateVerifyCodeConfig stores the verification code settings and reloads
// the verify subsystem, which enforces them.
func (s *Service) UpdateVerifyCodeConfig(ctx context.Context, req *dto.VerifyCodeConfig) error {
	change := settingsChange{
		category: "verify_code",
		next:     convertedConfigFields(*req),
		previous: previousFields(ctx, "verify_code", s.GetVerifyCodeConfig, convertedConfigFields),
	}
	if err := updateConfigFields(ctx, s.deps, change); err != nil {
		logger.WithContext(ctx).Errorw("[UpdateVerifyCodeConfig] update verify code config error", logger.Field("error", err.Error()))
		return xerr.Wrapf(err, xerr.DatabaseUpdateError, "update verify code config error: %v", err.Error())
	}
	return s.deps.reinit("verify")
}
