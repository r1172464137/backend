package system

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/platform"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/httpx"
)

// GetInterfaceConfigHandler documents the shared admin interface configuration.
// @Summary Get Interface Config
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.InterfaceConfig}
// @Router /v1/admin/system/interface_config [get]
func GetInterfaceConfigHandler(service platform.Service) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		result, err := service.GetInterfaceConfig(ctx)
		httpx.HttpResult(c, result, err)
	}
}

// UpdateInterfaceConfigHandler documents the shared admin interface configuration.
// @Summary Update Interface Config
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.InterfaceConfig true "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean
// @Router /v1/admin/system/interface_config [put]
func UpdateInterfaceConfigHandler(service platform.Service) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if err := httpx.RequireJSONObject(c); err != nil {
			httpx.ParamErrorResult(c, err)
			return
		}
		var req dto.InterfaceConfig
		decoder := json.NewDecoder(bytes.NewReader(c.Request.Body()))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			httpx.ParamErrorResult(c, err)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			httpx.ParamErrorResult(c, fmt.Errorf("invalid trailing JSON"))
			return
		}
		httpx.HttpResult(c, nil, service.UpdateInterfaceConfig(ctx, &req))
	}
}
