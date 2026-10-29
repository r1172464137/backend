package system

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/perfect-panel/server/internal/module/platform"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/xerr"
)

var _ dto.NodeConfig

// GetNodeConfigHandler documents Get node config.
//
// @Summary Get node config
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.NodeConfig}
// @Router /v1/admin/system/node_config [get]
func GetNodeConfigHandler(service platform.Service) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		resp, err := service.GetNodeConfig(ctx)
		if err == nil {
			body, marshalErr := json.Marshal(httpx.Success(resp))
			if marshalErr != nil || len(body) > 4*1024*1024 {
				c.JSON(consts.StatusInternalServerError, httpx.Error(xerr.ERROR, "node config response exceeds limit"))
				return
			}
		}
		httpx.HttpResult(c, resp, err)
	}
}
