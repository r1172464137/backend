package server

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/perfect-panel/server/internal/module/network"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/transport/http/validation"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/xerr"
)

// UpdateServerNodeConfigHandler documents Update Server Node Config.
//
// @Summary Update Server Node Config
// @Description Saves desired routing for new or offline servers without requiring a routing status report. Success does not mean applied; inspect routing_status and compare its applied identity with the desired identity. A routing change requires the current routing_revision.
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.UpdateServerNodeConfigRequest true "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean
// @Failure 409 {object} httpx.ResponseErrorBean "Stale routing revision"
// @Failure 422 {object} httpx.ResponseErrorBean "Legacy routing fields removed"
// @Router /v1/admin/server/node_config/update [post]
func UpdateServerNodeConfigHandler(service network.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		if len(ctx.Request.Body()) > 4*1024*1024 {
			ctx.JSON(consts.StatusRequestEntityTooLarge, httpx.Error(xerr.InvalidParams, "request too large"))
			return
		}
		if err := httpx.RequireJSONObject(ctx); err != nil {
			ctx.JSON(consts.StatusBadRequest, httpx.Error(xerr.InvalidParams, err.Error()))
			return
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(ctx.Request.Body(), &fields) == nil {
			for field := range fields {
				switch strings.ToLower(field) {
				case "block", "block_ips", "block_protocols", "outbound", "inherit_block", "inherit_outbound":
					ctx.JSON(consts.StatusUnprocessableEntity, httpx.Error(xerr.NetworkLegacyRoutingRemoved, xerr.MapErrMsg(xerr.NetworkLegacyRoutingRemoved)))
					return
				}
			}
		}
		var req dto.UpdateServerNodeConfigRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			httpx.ParamErrorResult(ctx, err)
			return
		}
		validateErr := validation.Validate(&req)
		if validateErr != nil {
			httpx.ParamErrorResult(ctx, validateErr)
			return
		}

		err := service.UpdateServerNodeConfig(c, &req)
		result := httpx.BuildHTTPResult(nil, err)
		if body, ok := result.Body.(*httpx.ResponseErrorBean); ok {
			switch body.Code {
			case xerr.NetworkRoutingRevisionConflict:
				result.StatusCode = consts.StatusConflict
			case xerr.NetworkLegacyRoutingRemoved:
				result.StatusCode = consts.StatusUnprocessableEntity
			}
		}
		ctx.JSON(result.StatusCode, result.Body)
	}
}
