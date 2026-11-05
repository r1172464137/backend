package document

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/support"
	dto "github.com/perfect-panel/server/internal/module/support/contract"
	"github.com/perfect-panel/server/pkg/httpx"
)

var _ dto.QueryDocumentListResponse

// QueryDocumentListHandler documents Get document list.
//
// @Summary Get document list
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.QueryDocumentListResponse}
// @Router /v1/public/document/list [get]
func QueryDocumentListHandler(service support.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {

		resp, err := service.QueryDocumentList(c)
		httpx.HttpResult(ctx, resp, err)
	}
}
