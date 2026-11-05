package system

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/httpx"
)

var _ dto.TosConfig

// GetTosConfigHandler documents Get Team of Service Config.
//
// @Summary Get Team of Service Config
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.TosConfig}
// @Router /v1/admin/system/tos_config [get]
func GetTosConfigHandler(service Settings) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		resp, err := service.GetTosConfig(ctx)
		httpx.HttpResult(c, resp, err)
	}
}
