package authmethodadmin

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/xerr"
)

// GetAuthMethodConfig returns the stored configuration of one method.
func (s *Service) GetAuthMethodConfig(ctx context.Context, req *dto.GetAuthMethodConfigRequest) (*dto.AuthMethodConfig, error) {
	method, err := s.deps.Auths.FindOneByMethod(ctx, req.Method)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "find auth method %q", req.Method)
	}
	return methodConfig(method)
}
