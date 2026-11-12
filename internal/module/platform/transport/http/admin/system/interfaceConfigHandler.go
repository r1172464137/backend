package system

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/platform"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/httpx"
)

// @Summary Get admin interface config
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.InterfaceConfig}
// @Router /v1/admin/system/interface_config [get]
func GetInterfaceConfigHandler(service platform.Service) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		resp, err := service.GetInterfaceConfig(ctx)
		httpx.HttpResult(c, resp, err)
	}
}

// @Summary Update admin interface config
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.InterfaceConfig true "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean
// @Router /v1/admin/system/interface_config [put]
func UpdateInterfaceConfigHandler(service platform.Service) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.InterfaceConfig
		if err := httpx.ShouldBind(c, &req); err != nil {
			httpx.ParamErrorResult(c, err)
			return
		}
		httpx.HttpResult(c, nil, service.UpdateInterfaceConfig(ctx, &req))
	}
}
