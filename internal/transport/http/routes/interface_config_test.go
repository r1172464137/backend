package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/perfect-panel/server/pkg/xerr"
)

func TestInterfaceConfigRequiresAuthentication(t *testing.T) {
	router := server.New()
	registerAdminSystemRoutes(router, Dependencies{})
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		ctx := router.NewContext()
		ctx.Request.SetRequestURI("/v1/admin/system/interface_config")
		ctx.Request.Header.SetMethod(method)
		router.ServeHTTP(context.Background(), ctx)
		var result struct {
			Code uint32 `json:"code"`
		}
		if err := json.Unmarshal(ctx.Response.Body(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Code != xerr.ErrorTokenEmpty {
			t.Fatalf("%s: %s", method, ctx.Response.Body())
		}
	}
}
