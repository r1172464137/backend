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

// ServerProtocolConfigReader is the part of the network facade
// QueryServerProtocolConfigHandler calls.
type ServerProtocolConfigReader interface {
	QueryServerProtocolConfig(ctx context.Context, req *dto.QueryServerConfigRequest) (*dto.QueryServerConfigResponse, error)
}

var _ ServerProtocolConfigReader = network.Service(nil)

// QueryServerProtocolConfigHandler documents Get Server Protocol Config.
//
// @Summary Get Server Protocol Config
// @Tags node
// @Produce json,application/protobuf
// @Security NodeSecret
// @Param server_id path int true "Server ID"
// @Param protocols query []string false "Protocols to include" collectionFormat(multi)
// @Param X-PPanel-Routing-Version header string true "Maximum supported routing version: 1 or 2"
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.QueryServerConfigResponse}
// @Failure 426 {object} httpx.ResponseErrorBean "Node routing upgrade required"
// @Router /v2/server/{server_id} [get]
func QueryServerProtocolConfigHandler(service ServerProtocolConfigReader, nodeSecret func() string) app.HandlerFunc {
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

		// Capability is checked before any conditional response.
		switch string(ctx.GetHeader(routingVersionHeader)) {
		case "1":
			req.RoutingVersion = 1
		case "2":
			req.RoutingVersion = 2
		default:
			writeRoutingUpgrade(ctx)
			return
		}
		resp, err := service.QueryServerProtocolConfig(c, &req)
		if err != nil {
			writeServerReportResult(ctx, err)
			return
		}
		// Never project version 2 to a version 1 node.
		if resp.Routing != nil && resp.Routing.Version > req.RoutingVersion {
			writeRoutingUpgradeMessage(ctx, "Node must support routing version 2")
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
