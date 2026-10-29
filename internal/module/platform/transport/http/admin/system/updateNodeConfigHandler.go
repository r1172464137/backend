package system

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/perfect-panel/server/internal/module/platform"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/transport/http/validation"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/xerr"
)

// UpdateNodeConfigHandler documents Update node config.
//
// @Summary Update node config
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.NodeConfig true "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean
// @Failure 409 {object} httpx.ResponseErrorBean "Stale routing revision"
// @Failure 422 {object} httpx.ResponseErrorBean "Legacy routing fields removed"
// @Router /v1/admin/system/node_config [put]
func UpdateNodeConfigHandler(service platform.Service) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if len(c.Request.Body()) > 4*1024*1024 {
			c.JSON(consts.StatusRequestEntityTooLarge, httpx.Error(xerr.InvalidParams, "request too large"))
			return
		}
		if err := httpx.RequireJSONObject(c); err != nil {
			c.JSON(consts.StatusBadRequest, httpx.Error(xerr.InvalidParams, err.Error()))
			return
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(c.Request.Body(), &fields) == nil {
			for field := range fields {
				switch strings.ToLower(field) {
				case "block", "block_ips", "block_protocols", "outbound", "routing_action":
					c.JSON(consts.StatusUnprocessableEntity, httpx.Error(xerr.PlatformLegacyRoutingRemoved, xerr.MapErrMsg(xerr.PlatformLegacyRoutingRemoved)))
					return
				}
			}
		}
		var req dto.NodeConfig
		if err := httpx.ShouldBind(c, &req); err != nil {
			httpx.ParamErrorResult(c, err)
			return
		}
		validateErr := validation.Validate(&req)
		if validateErr != nil {
			httpx.ParamErrorResult(c, validateErr)
			return
		}

		err := service.UpdateNodeConfig(ctx, &req)
		result := httpx.BuildHTTPResult(nil, err)
		if body, ok := result.Body.(*httpx.ResponseErrorBean); ok {
			switch body.Code {
			case xerr.PlatformRoutingRevisionConflict:
				result.StatusCode = consts.StatusConflict
			case xerr.PlatformLegacyRoutingRemoved:
				result.StatusCode = consts.StatusUnprocessableEntity
			}
		}
		c.JSON(result.StatusCode, result.Body)
	}
}
