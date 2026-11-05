package system

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/httpx"
)

var _ dto.SiteConfig

// GetSiteConfigHandler documents Get site config.
//
// @Summary Get site config
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.SiteConfig}
// @Router /v1/admin/system/site_config [get]
func GetSiteConfigHandler(service Settings) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		resp, err := service.GetSiteConfig(ctx)
		httpx.HttpResult(c, resp, err)
	}
}
