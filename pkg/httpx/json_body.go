package httpx

import (
	"bytes"
	"fmt"
	"mime"

	"github.com/cloudwego/hertz/pkg/app"
)

// RequireJSONObject rejects query/form binding and null or empty bodies for
// endpoints whose mutation semantics depend on JSON field presence.
func RequireJSONObject(ctx *app.RequestContext) error {
	mediaType, _, err := mime.ParseMediaType(string(ctx.ContentType()))
	if err != nil || mediaType != "application/json" {
		return fmt.Errorf("Content-Type must be application/json")
	}
	body := bytes.TrimSpace(ctx.Request.Body())
	if len(body) == 0 || body[0] != '{' {
		return fmt.Errorf("a non-empty JSON object is required")
	}
	return nil
}
