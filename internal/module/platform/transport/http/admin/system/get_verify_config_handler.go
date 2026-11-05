package system

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/httpx"
)

var _ dto.VerifyConfig

// GetVerifyConfigHandler documents Get verify config.
//
// @Summary Get verify config
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.VerifyConfig}
// @Router /v1/admin/system/verify_config [get]
func GetVerifyConfigHandler(service Settings) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		resp, err := service.GetVerifyConfig(ctx)
		httpx.HttpResult(c, resp, err)
	}
}
