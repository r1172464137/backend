package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/network"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/pkg/xerr"
)

type conflictService struct{ network.Service }

func (conflictService) UpdateServerNodeConfig(context.Context, *dto.UpdateServerNodeConfigRequest) error {
	return xerr.NewErrCode(xerr.NetworkRoutingRevisionConflict)
}
func TestRoutingConflictHTTP409(t *testing.T) {
	ctx := app.NewContext(0)
	ctx.Request.Header.SetContentTypeBytes([]byte("application/json"))
	ctx.Request.SetBodyString(`{"server_id":1,"routing_revision":0}`)
	UpdateServerNodeConfigHandler(conflictService{})(context.Background(), ctx)
	var result struct {
		Code uint32 `json:"code"`
	}
	if err := json.Unmarshal(ctx.Response.Body(), &result); err != nil {
		t.Fatal(err)
	}
	if ctx.Response.StatusCode() != 409 || result.Code != xerr.NetworkRoutingRevisionConflict {
		t.Fatalf("status %d, body %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}
}
