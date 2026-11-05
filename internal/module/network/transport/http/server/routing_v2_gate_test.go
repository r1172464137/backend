package server

import (
	"context"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
)

func TestV2DesiredRejectsV1BeforeConditionalResponse(t *testing.T) {
	p := routeTestPolicy()
	p.Version = 2
	p.Mode = "ordered-v2-local"
	handler := QueryServerProtocolConfigHandler(routingTestService{response: &dto.QueryServerConfigResponse{Routing: p}}, func() string { return "secret" })
	for _, accept := range []string{"application/json", "application/protobuf"} {
		ctx := queryContext(accept, "1", `"old-etag"`)
		handler(context.Background(), ctx)
		if ctx.Response.StatusCode() != 426 {
			t.Fatalf("%s: status %d", accept, ctx.Response.StatusCode())
		}
	}
}
