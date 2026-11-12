package handler

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/protocolkey"
	"github.com/perfect-panel/server/internal/module/subscription"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type SubscribeDeps struct {
	Service subscription.Service
	Config  func() config.SubscribeConfig
}

// SubscribeHandler returns a client subscription configuration.
//
// @Summary Get subscription configuration
// @Tags user
// @Produce plain
// @Param token query string false "Subscription token; alternatively send the token header"
// @Param token header string false "Subscription token"
// @Param flag query string false "Subscription format flag"
// @Param type query string false "Subscription format type"
// @Param User-Agent header string false "Client user agent"
// @Success 200 {string} string
// @Router /v1/subscribe/config [get]
func SubscribeHandler(deps SubscribeDeps) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		req := dto.SubscribeRequest{
			Token:  string(ctx.GetHeader("token")),
			UA:     string(ctx.UserAgent()),
			Flag:   ctx.Query("flag"),
			Type:   ctx.Query("type"),
			Params: getQueryMap(ctx),
		}
		if req.Token == "" {
			req.Token = ctx.Query("token")
		}

		config := deps.Config()
		if config.PanDomain {
			domainArr := strings.Split(string(ctx.Host()), ".")
			if len(domainArr) == 0 {
				ctx.String(consts.StatusForbidden, "Access denied")
				return
			}
			short, err := protocolkey.FixedUniqueString(req.Token, 8, "")
			if err != nil {
				logger.WithContext(c).Errorf("[SubscribeHandler] Generate short token failed: %v", err)
				ctx.String(consts.StatusInternalServerError, "Internal Server")
				return
			}
			if strings.ToLower(short) != strings.ToLower(domainArr[0]) {
				logger.WithContext(c).Debug("[SubscribeHandler] short token mismatch")
				ctx.String(consts.StatusForbidden, "Access denied")
				return
			}
		}

		if config.UserAgentLimit && !deps.Service.IsUserAgentAllowed(c, req.UA) {
			ctx.String(consts.StatusForbidden, "Access denied")
			return
		}
		writeSubscribeResponse(c, ctx, deps.Service, req)
	}
}

// PanDomainSubscribeHandler returns a subscription selected by the request host.
//
// @Summary Get pan-domain subscription configuration
// @Tags user
// @Produce plain
// @Param User-Agent header string false "Client user agent"
// @Success 200 {string} string
// @Router / [get]
func PanDomainSubscribeHandler(deps SubscribeDeps) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		config := deps.Config()
		ua := string(ctx.UserAgent())
		if config.UserAgentLimit && !deps.Service.IsUserAgentAllowed(c, ua) {
			ctx.String(consts.StatusForbidden, "Access denied")
			return
		}

		domainArr := strings.Split(string(ctx.Host()), ".")
		if len(domainArr) < 2 {
			ctx.String(consts.StatusForbidden, "Access denied")
			return
		}

		writeSubscribeResponse(c, ctx, deps.Service, dto.SubscribeRequest{
			Token:  domainArr[0],
			Flag:   domainArr[1],
			UA:     ua,
			Params: getQueryMap(ctx),
		})
	}
}

func writeSubscribeResponse(c context.Context, ctx *app.RequestContext, service subscription.Service, req dto.SubscribeRequest) {
	resp, err := service.Deliver(c, subscription.RequestMeta{
		Host:       string(ctx.Host()),
		RequestURI: string(ctx.URI().RequestURI()),
		UserAgent:  string(ctx.UserAgent()),
		ClientIP:   ctx.ClientIP(),
	}, &req)
	if err != nil {
		status, msg := subscribeErrorStatus(err)
		ctx.Header("Cache-Control", "private, no-store")
		ctx.String(status, msg)
		return
	}
	contentType := resp.Headers["Content-Type"]
	if contentType == "" {
		contentType = "text/plain; charset=utf-8"
	}
	for key, value := range resp.Headers {
		if key != "Content-Type" {
			ctx.Header(key, value)
		}
	}
	ctx.Header("subscription-userinfo", resp.Header)
	ctx.Data(consts.StatusOK, contentType, resp.Config)
}

// subscribeErrorStatus maps delivery errors to plain-text statuses (CONTRACT 5.3);
// a non-200 body is never shaped like a configuration.
func subscribeErrorStatus(err error) (int, string) {
	var code *xerr.CodeError
	if !errors.As(errors.Cause(err), &code) {
		return consts.StatusInternalServerError, "Internal Server Error"
	}
	switch code.GetErrCode() {
	case xerr.ErrorTokenInvalid:
		return consts.StatusNotFound, "Subscription not found"
	case xerr.UserDisabled, xerr.UserNotExist:
		return consts.StatusForbidden, "Access denied"
	case xerr.TooManyRequests:
		return consts.StatusTooManyRequests, "Too Many Requests"
	case xerr.InvalidParams:
		return consts.StatusBadRequest, code.GetErrMsg()
	default:
		return consts.StatusInternalServerError, "Internal Server Error"
	}
}

func getQueryMap(ctx *app.RequestContext) map[string]string {
	result := make(map[string]string)
	ctx.QueryArgs().VisitAll(func(key, value []byte) {
		k := string(key)
		if _, ok := result[k]; !ok {
			result[k] = string(value)
		}
	})
	return result
}
