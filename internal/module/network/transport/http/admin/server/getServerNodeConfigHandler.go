package server

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/perfect-panel/server/internal/module/network"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/transport/http/validation"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/xerr"
)

// GetServerNodeConfigHandler documents Get Server Node Config.
//
// @Summary Get Server Node Config
// @Description Returns desired routing and independently reported routing_status. Unknown or pending does not prevent configuration saves; only a node-reported matching applied identity confirms application.
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request query dto.GetServerNodeConfigRequest false "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.GetServerNodeConfigResponse}
// @Router /v1/admin/server/node_config [get]
func GetServerNodeConfigHandler(service network.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		var req dto.GetServerNodeConfigRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			httpx.ParamErrorResult(ctx, err)
			return
		}
		validateErr := validation.Validate(&req)
		if validateErr != nil {
			httpx.ParamErrorResult(ctx, validateErr)
			return
		}

		resp, err := service.GetServerNodeConfig(c, &req)
		if err == nil {
			body, marshalErr := json.Marshal(httpx.Success(resp))
			if marshalErr != nil || len(body) > 4*1024*1024 {
				ctx.JSON(consts.StatusInternalServerError, httpx.Error(xerr.ERROR, "node config response exceeds limit"))
				return
			}
		}
		httpx.HttpResult(ctx, resp, err)
	}
}
