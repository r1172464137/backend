package server

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/perfect-panel/server/internal/module/network"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/logger"
	"google.golang.org/protobuf/proto"
)

// QueryServerProtocolConfigHandler documents Get Server Protocol Config.
//
// @Summary Get Server Protocol Config
// @Tags node
// @Produce json,application/protobuf
// @Security NodeSecret
// @Param server_id path int true "Server ID"
// @Param protocols query []string false "Protocols to include" collectionFormat(multi)
// @Param X-PPanel-Routing-Version header string true "Must be 1"
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.QueryServerConfigResponse}
// @Failure 426 {object} httpx.ResponseErrorBean "Node routing upgrade required"
// @Router /v2/server/{server_id} [get]
func QueryServerProtocolConfigHandler(service network.Service, nodeSecret func() string) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		ctx.Header("Vary", "Accept, "+routingVersionHeader)
		acceptsProtobuf := acceptsProtobuf(ctx)
		serverID, err := strconv.ParseInt(ctx.Param("server_id"), 10, 64)
		if err != nil {
			logger.WithContext(c).Debugf("[QueryServerProtocolConfigHandler] Parse server_id error: %v, Param: %s", err, ctx.Param("server_id"))
			writeServerText(ctx, consts.StatusBadRequest, "Invalid Params")
			ctx.Abort()
			return
		}
		req := dto.QueryServerConfigRequest{
			ServerID:  serverID,
			SecretKey: ctx.Query("secret_key"),
			Protocols: queryValues(ctx, "protocols", "protocols[]"),
		}
		if !nodeSecretMatches(nodeSecret(), req.SecretKey) {
			writeServerText(ctx, consts.StatusUnauthorized, "Unauthorized")
			ctx.Abort()
			return
		}

		// Ordered-v1 is the only runtime mode. Reject old nodes before any
		// conditional-response or configuration read can return a candidate.
		if string(ctx.GetHeader(routingVersionHeader)) != "1" {
			writeRoutingUpgrade(ctx)
			return
		}
		resp, err := service.QueryServerProtocolConfig(c, &req)
		if err != nil {
			writeServerReportResult(ctx, err)
			return
		}
		if err := validateRoutingDelivery(resp, serverID); err != nil {
			logger.WithContext(c).Errorf("[QueryServerProtocolConfigHandler] invalid routing candidate: %v", err)
			writeServerReportResult(ctx, err)
			return
		}
		if acceptsProtobuf {
			message, err := queryServerProtocolConfigResponseToProtobuf(resp)
			if err != nil {
				writeServerReportResult(ctx, err)
				return
			}
			if proto.Size(message) > maxRoutingNodeResponseBytes {
				writeServerReportResult(ctx, errors.New("ordered routing response exceeds wire limit"))
				return
			}
			if err := writeServerProtobufWithETag(ctx, message, string(ctx.GetHeader("If-None-Match"))); err != nil {
				writeServerReportResult(ctx, err)
			}
			return
		}
		body, err := json.Marshal(resp)
		if err != nil {
			writeHTTPResult(ctx, nil, err)
			return
		}
		envelope, err := json.Marshal(httpx.Success(resp))
		if err != nil || len(envelope) > maxRoutingNodeResponseBytes {
			writeServerReportResult(ctx, errors.New("ordered routing response exceeds wire limit"))
			return
		}
		etag := httpx.GenerateETag(body)
		ctx.Header("ETag", etag)
		if string(ctx.GetHeader("If-None-Match")) == etag {
			ctx.SetStatusCode(consts.StatusNotModified)
			return
		}
		writeHTTPResult(ctx, resp, nil)
	}
}
