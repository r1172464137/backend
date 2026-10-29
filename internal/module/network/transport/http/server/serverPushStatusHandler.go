package server

import (
	"context"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/network"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/transport/http/validation"
)

// ServerPushStatusHandler documents Push server status.
//
// @Summary Push server status
// @Tags node
// @Accept json,application/protobuf
// @Produce json,application/protobuf
// @Security NodeSecret
// @Param request body dto.ServerPushStatusRequest true "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean
// @Router /v1/server/status [post]
func ServerPushStatusHandler(service network.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		// Authentication is performed by ServerMiddleware before this handler.
		// Resolve the server before parsing so malformed reports invalidate only
		// that server's routing status without retaining body/error details.
		commonReq, err := serverCommonRequest(ctx)
		if err != nil || commonReq.ServerId <= 0 {
			writeParamError(ctx, errors.New("invalid server identity"))
			return
		}
		req := dto.ServerPushStatusRequest{}
		if err := bindServerStatusRequest(ctx, &req); err != nil {
			_ = service.InvalidateRoutingStatus(c, commonReq.ServerId)
			writeParamError(ctx, errors.New("invalid status payload"))
			return
		}
		req.ServerCommon = commonReq
		req.CertPinSHA256 = string(ctx.GetHeader(certificateSHA256Header))
		if validateErr := validation.Validate(&req); validateErr != nil {
			_ = service.InvalidateRoutingStatus(c, commonReq.ServerId)
			writeParamError(ctx, errors.New("invalid status payload"))
			return
		}

		writeServerReportResult(ctx, service.ServerPushStatus(c, &req))
	}
}
