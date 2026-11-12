package server

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/network"
)

// GetServerUserListHandler rejects the removed user list endpoint. Users,
// limits and UUIDs reach nodes only through the identity snapshot of the
// /v2/server/{server_id} candidate, so a node can never combine a separately
// polled user list with a different routing candidate.
//
// @Summary Removed: users are delivered in /v2/server/{server_id}
// @Tags node
// @Produce json,application/protobuf
// @Security NodeSecret
// @Failure 426 {object} httpx.ResponseErrorBean "Node routing upgrade required"
// @Router /v1/server/user [get]
func GetServerUserListHandler(_ network.Service) app.HandlerFunc {
	return func(_ context.Context, ctx *app.RequestContext) {
		ctx.Header("Vary", "Accept, "+routingVersionHeader)
		writeRoutingUpgrade(ctx)
	}
}
