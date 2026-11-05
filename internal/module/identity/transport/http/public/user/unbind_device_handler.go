package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/identity"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/transport/http/validation"
	"github.com/perfect-panel/server/pkg/httpx"
)

// UnbindDeviceHandler documents Unbind Device.
//
// @Summary Unbind Device
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.UnbindDeviceRequest true "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean
// @Router /v1/public/user/unbind_device [put]
func UnbindDeviceHandler(service identity.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		var req dto.UnbindDeviceRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			httpx.ParamErrorResult(ctx, err)
			return
		}
		validateErr := validation.Validate(&req)
		if validateErr != nil {
			httpx.ParamErrorResult(ctx, validateErr)
			return
		}

		err := service.UnbindDevice(c, &req)
		httpx.HttpResult(ctx, nil, err)
	}
}
