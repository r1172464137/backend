package server

import (
	"bytes"
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/network"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/transport/http/validation"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/xerr"
)

// CreateServerHandler documents Create Server.
//
// @Summary Create Server
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.CreateServerRequest true "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.CreateServerResponse}
// @Failure 422 {object} httpx.ResponseErrorBean "Enabled node protocol is invalid or unsupported"
// @Router /v1/admin/server/create [post]
func CreateServerHandler(service network.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		if err := httpx.RequireJSONObject(ctx); err != nil {
			ctx.JSON(consts.StatusBadRequest, httpx.Error(xerr.InvalidParams, err.Error()))
			return
		}
		body := bytes.TrimSpace(ctx.Request.Body())
		if len(body) > 0 && body[0] == '{' {
			if err := config.ValidateUniqueJSON(body); err != nil {
				httpx.ParamErrorResult(ctx, err)
				return
			}
		}
		if err := requireProtocolEnablePresence(body, false); err != nil {
			ctx.JSON(consts.StatusUnprocessableEntity, httpx.Error(xerr.NetworkNodeProtocolInvalid, err.Error()))
			return
		}
		var req dto.CreateServerRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			httpx.ParamErrorResult(ctx, err)
			return
		}
		validateErr := validation.Validate(&req)
		if validateErr != nil {
			httpx.ParamErrorResult(ctx, validateErr)
			return
		}

		response, err := service.CreateServer(c, &req)
		result := httpx.BuildHTTPResult(response, err)
		if body, ok := result.Body.(*httpx.ResponseErrorBean); ok && body.Code == xerr.NetworkNodeProtocolInvalid {
			result.StatusCode = consts.StatusUnprocessableEntity
		}
		ctx.JSON(result.StatusCode, result.Body)
	}
}
