package server

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/pkg/httpx"
)

var _ httpx.ResponseErrorBean

// GetServerConfigHandler rejects the removed legacy config endpoint.
// @Summary Legacy node config removed; use /v2/server/{server_id}
// @Tags node
// @Produce json,application/protobuf
// @Security NodeSecret
// @Failure 426 {object} httpx.ResponseErrorBean "Node routing upgrade required"
// @Router /v1/server/config [get]
func GetServerConfigHandler(_ network.Service) app.HandlerFunc {
	return func(_ context.Context, ctx *app.RequestContext) {
		ctx.Header("Vary", "Accept, "+routingVersionHeader)
		writeRoutingUpgrade(ctx)
	}
}
