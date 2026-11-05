package log

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/httpx"
)

var _ dto.LogSetting

// GetLogSettingHandler documents Get log setting.
//
// @Summary Get log setting
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.LogSetting}
// @Router /v1/admin/log/setting [get]
func GetLogSettingHandler(service Logs) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		resp, err := service.GetLogSetting(ctx)
		httpx.HttpResult(c, resp, err)
	}
}
