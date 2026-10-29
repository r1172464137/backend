package server

import (
	"context"
	"crypto/subtle"
	"mime"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	serverv1 "github.com/perfect-panel/server/api/server/v1"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

const protobufContentType = "application/protobuf"

// certificateSHA256Header carries the SHA256 fingerprint of the node's
// self-signed TLS certificate; ppanel-node attaches it to every request once
// the certificate is loaded.
const certificateSHA256Header = "X-Node-Certificate-SHA256"

// nodeSecretMatches reports whether key authenticates as the node secret. An
// unprovisioned (empty) secret never authenticates: comparing it directly would
// let a bare `?secret_key=` through.
func nodeSecretMatches(secret, key string) bool {
	if secret == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(key), []byte(secret)) == 1
}

func ServerMiddleware(nodeSecret func() string) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		ctx.Header("Vary", "Accept")
		if nodeSecretMatches(nodeSecret(), ctx.Query("secret_key")) {
			ctx.Next(c)
			return
		}
		writeServerText(ctx, consts.StatusForbidden, "Forbidden")
		ctx.Abort()
	}
}

func serverCommonRequest(ctx *app.RequestContext) (dto.ServerCommon, error) {
	var serverID int64
	if rawServerID := ctx.Query("server_id"); rawServerID != "" {
		id, err := strconv.ParseInt(rawServerID, 10, 64)
		if err != nil {
			return dto.ServerCommon{}, err
		}
		serverID = id
	}
	return dto.ServerCommon{
		Protocol:  ctx.Query("protocol"),
		ServerId:  serverID,
		SecretKey: ctx.Query("secret_key"),
	}, nil
}

func queryValues(ctx *app.RequestContext, keys ...string) []string {
	var values []string
	for _, key := range keys {
		for _, value := range ctx.QueryArgs().PeekAll(key) {
			values = append(values, string(value))
		}
	}
	return values
}

func writeHeaders(ctx *app.RequestContext, headers map[string]string) {
	for key, value := range headers {
		ctx.Header(key, value)
	}
}

func writeHTTPResult(ctx *app.RequestContext, resp interface{}, err error) {
	res := httpx.BuildHTTPResult(resp, err)
	ctx.JSON(res.StatusCode, res.Body)
}

func writeServerReportResult(ctx *app.RequestContext, err error) {
	if acceptsProtobuf(ctx) {
		writeServerProtobuf(ctx, consts.StatusOK, serverResult(err))
		return
	}
	writeHTTPResult(ctx, nil, err)
}

func writeParamError(ctx *app.RequestContext, err error) {
	if acceptsProtobuf(ctx) {
		writeServerProtobuf(ctx, consts.StatusOK, &serverv1.Result{Code: xerr.InvalidParams, Message: err.Error()})
		return
	}
	resp := httpx.BuildParamErrorResult(err)
	ctx.JSON(resp.StatusCode, resp.Body)
}

func writeRoutingUpgrade(ctx *app.RequestContext) {
	message := xerr.MapErrMsg(xerr.NodeRoutingUpgradeRequired)
	if acceptsProtobuf(ctx) {
		writeServerProtobuf(ctx, consts.StatusUpgradeRequired, &serverv1.Result{Code: xerr.NodeRoutingUpgradeRequired, Message: message})
	} else {
		ctx.JSON(consts.StatusUpgradeRequired, httpx.Error(xerr.NodeRoutingUpgradeRequired, message))
	}
}

func writeServerText(ctx *app.RequestContext, statusCode int, message string) {
	if acceptsProtobuf(ctx) {
		writeServerProtobuf(ctx, statusCode, &serverv1.Result{Code: uint32(statusCode), Message: message})
		return
	}
	ctx.String(statusCode, message)
}

func writeServerProtobuf(ctx *app.RequestContext, statusCode int, message proto.Message) {
	body, err := proto.Marshal(message)
	if err != nil {
		ctx.String(consts.StatusInternalServerError, "Internal Server Error")
		return
	}
	ctx.Data(statusCode, protobufContentType, body)
}

func writeServerProtobufWithETag(ctx *app.RequestContext, message proto.Message, ifNoneMatch string) error {
	body, err := (proto.MarshalOptions{Deterministic: true}).Marshal(message)
	if err != nil {
		return err
	}
	etag := httpx.GenerateETag(body)
	ctx.Header("ETag", etag)
	if ifNoneMatch == etag {
		ctx.SetStatusCode(consts.StatusNotModified)
		return nil
	}
	ctx.Data(consts.StatusOK, protobufContentType, body)
	return nil
}

func serverResult(err error) *serverv1.Result {
	if err == nil {
		return &serverv1.Result{Code: 200, Message: "success"}
	}
	code := xerr.ERROR
	message := "Internal Server Error"
	var codeErr *xerr.CodeError
	if errors.As(errors.Cause(err), &codeErr) {
		code = codeErr.GetErrCode()
		message = codeErr.GetErrMsg()
	}
	return &serverv1.Result{Code: code, Message: message}
}

func isProtobufRequest(ctx *app.RequestContext) bool {
	mediaType, _, err := mime.ParseMediaType(string(ctx.ContentType()))
	return err == nil && mediaType == protobufContentType
}

func acceptsProtobuf(ctx *app.RequestContext) bool {
	for _, value := range strings.Split(string(ctx.GetHeader("Accept")), ",") {
		mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(value))
		if err != nil || mediaType != protobufContentType || params["q"] == "0" {
			continue
		}
		return true
	}
	return false
}

func ifNoneMatchForRepresentation(ifNoneMatch string, protobuf bool) string {
	if protobuf {
		return ""
	}
	return ifNoneMatch
}
