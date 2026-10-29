package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	serverv1 "github.com/perfect-panel/server/api/server/v1"
	"github.com/perfect-panel/server/pkg/xerr"
	"google.golang.org/protobuf/proto"
)

func TestRemovedV1ConfigAlwaysReturns426(t *testing.T) {
	for _, accept := range []string{"application/json", protobufContentType} {
		for _, version := range []string{"", "1"} {
			ctx := app.NewContext(0)
			ctx.Request.Header.Set("Accept", accept)
			ctx.Request.Header.Set(routingVersionHeader, version)
			ctx.Request.Header.Set("If-None-Match", `"old-etag"`)
			GetServerConfigHandler(nil)(context.Background(), ctx)
			if ctx.Response.StatusCode() != 426 {
				t.Fatalf("accept %s version %s status %d", accept, version, ctx.Response.StatusCode())
			}
			if accept == protobufContentType {
				var result serverv1.Result
				if err := proto.Unmarshal(ctx.Response.Body(), &result); err != nil || result.Code != xerr.NodeRoutingUpgradeRequired {
					t.Fatalf("proto %#v %v", &result, err)
				}
			} else {
				var result struct {
					Code uint32 `json:"code"`
				}
				if err := json.Unmarshal(ctx.Response.Body(), &result); err != nil || result.Code != xerr.NodeRoutingUpgradeRequired {
					t.Fatalf("JSON %#v %v", result, err)
				}
			}
		}
	}
}
