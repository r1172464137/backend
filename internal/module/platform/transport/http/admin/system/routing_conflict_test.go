package system

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/platform"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/xerr"
)

type conflictService struct{ platform.Service }

func (conflictService) UpdateNodeConfig(context.Context, *dto.NodeConfig) error {
	return xerr.NewErrCode(xerr.PlatformRoutingRevisionConflict)
}
func TestRoutingConflictHTTP409(t *testing.T) {
	ctx := app.NewContext(0)
	ctx.Request.Header.SetContentTypeBytes([]byte("application/json"))
	ctx.Request.SetBodyString(`{"routing_revision":0}`)
	UpdateNodeConfigHandler(conflictService{})(context.Background(), ctx)
	var result struct {
		Code uint32 `json:"code"`
	}
	if err := json.Unmarshal(ctx.Response.Body(), &result); err != nil {
		t.Fatal(err)
	}
	if ctx.Response.StatusCode() != 409 || result.Code != xerr.PlatformRoutingRevisionConflict {
		t.Fatalf("status %d, body %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}
}
