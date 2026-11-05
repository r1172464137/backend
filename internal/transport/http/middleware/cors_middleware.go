package middleware

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// CorsMiddleware is the permissive CORS policy: it reflects any Origin and
// allows credentials, as the API always did. Deployments that configure
// AllowedOrigins get NewCorsMiddleware instead.
func CorsMiddleware(c context.Context, ctx *app.RequestContext) {
	permissiveCors(c, ctx)
}

var permissiveCors = NewCorsMiddleware(nil)

// NewCorsMiddleware answers CORS for the browser origins in allowedOrigins,
// each as scheme://host[:port] and compared exactly (case-insensitively): a
// matching Origin is allowed with credentials, any other gets no CORS
// headers, so the browser refuses it the response. An empty list keeps the
// permissive default of reflecting whatever Origin the request carries,
// which is safe only while the API carries no cookie or ambient credential.
// Preflights are answered with 204 either way.
func NewCorsMiddleware(allowedOrigins []string) app.HandlerFunc {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		if origin = normalizeOrigin(origin); origin != "" {
			allowed[origin] = true
		}
	}
	return func(c context.Context, ctx *app.RequestContext) {
		origin := string(ctx.GetHeader("Origin"))
		switch {
		case len(allowed) > 0:
			if allowed[normalizeOrigin(origin)] {
				allowOrigin(ctx, origin)
			}
			// The answer depends on the Origin, so a shared cache must not
			// serve one origin's answer to another.
			ctx.Header("Vary", "Origin")
		case origin != "":
			allowOrigin(ctx, origin)
		default:
			allowOrigin(ctx, "*")
		}
		if string(ctx.Method()) == consts.MethodOptions {
			ctx.AbortWithStatus(consts.StatusNoContent)
			return
		}

		ctx.Next(c)
	}
}

// allowOrigin writes the headers admitting origin with credentials.
func allowOrigin(ctx *app.RequestContext, origin string) {
	ctx.Header("Access-Control-Allow-Origin", origin)
	ctx.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE, UPDATE")
	ctx.Header("Access-Control-Allow-Headers", "Content-Type, Origin, X-CSRF-Token, Authorization, AccessToken, Token, Range")
	ctx.Header("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers")
	ctx.Header("Access-Control-Allow-Credentials", "true")
	ctx.Header("Access-Control-Max-Age", "172800")
}

// normalizeOrigin lowercases an origin and drops surrounding space and a
// trailing slash, so a configured "https://Panel.Example/" matches the
// browser's "https://panel.example".
func normalizeOrigin(origin string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(origin)), "/")
}
