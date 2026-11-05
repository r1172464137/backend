package payment

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/billing"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/internal/transport/http/validation"
	"github.com/perfect-panel/server/pkg/httpx"
)

// UpdatePaymentMethodHandler documents Update Payment Method.
//
// @Summary Update Payment Method
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.UpdatePaymentMethodRequest true "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.PaymentConfig}
// @Router /v1/admin/payment/ [put]
func UpdatePaymentMethodHandler(service billing.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		var req dto.UpdatePaymentMethodRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			httpx.ParamErrorResult(ctx, err)
			return
		}
		validateErr := validation.Validate(&req)
		if validateErr != nil {
			httpx.ParamErrorResult(ctx, validateErr)
			return
		}

		resp, err := service.UpdatePaymentMethod(c, &req)
		httpx.HttpResult(ctx, resp, err)
	}
}
