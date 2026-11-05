package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/identity"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/transport/http/validation"
	"github.com/perfect-panel/server/pkg/httpx"
)

// CreateUserAuthMethodHandler documents Create user auth method.
//
// @Summary Create user auth method
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.CreateUserAuthMethodRequest true "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean
// @Router /v1/admin/user/auth_method [post]
func CreateUserAuthMethodHandler(service identity.Service) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.CreateUserAuthMethodRequest
		if err := httpx.ShouldBind(c, &req); err != nil {
			httpx.ParamErrorResult(c, err)
			return
		}
		validateErr := validation.Validate(&req)
		if validateErr != nil {
			httpx.ParamErrorResult(c, validateErr)
			return
		}

		err := service.CreateUserAuthMethod(ctx, &req)
		httpx.HttpResult(c, nil, err)
	}
}
