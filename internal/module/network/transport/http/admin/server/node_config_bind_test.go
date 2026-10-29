package server

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/network"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/pkg/xerr"
)

type noLegacyService struct {
	network.Service
	called bool
}

func (s *noLegacyService) UpdateServerNodeConfig(context.Context, *dto.UpdateServerNodeConfigRequest) error {
	s.called = true
	return nil
}
func TestServerRoutingRejectsUnsafeSpiderBeforeService(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
		valid        bool
	}{
		{"projected_max", `"spider_x":"/?p=4096"`, true},
		{"projected_over", `"spider_x":"/?p=4097"`, false},
		{"stream_nested", `"stream_settings":"{\"realitySettings\":{\"spiderX\":\"/?c=9\"}}"`, false},
		{"extra_nested", `"xhttp_extra":"{\"downloadSettings\":{\"realitySettings\":{\"spiderX\":\"/?r=5001\"}}}"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &noLegacyService{}
			ctx := app.NewContext(0)
			ctx.Request.Header.SetContentTypeBytes([]byte("application/json"))
			ctx.Request.SetBodyString(fmt.Sprintf(`{"server_id":1,"routing_revision":0,"routing":{"version":1,"outbounds":[{"id":"ob_11111111-1111-4111-8111-111111111111","name":"test","protocol":"direct",%s}],"rules":[],"default_outbound_id":"direct"}}`, tc.fields))
			UpdateServerNodeConfigHandler(svc)(context.Background(), ctx)
			if svc.called != tc.valid {
				t.Fatalf("service called=%v valid=%v response=%s", svc.called, tc.valid, ctx.Response.Body())
			}
		})
	}
}

func TestLegacyFieldsAreRejected422BeforeBinding(t *testing.T) {
	for _, body := range []string{`{"server_id":1,"block":[]}`, `{"server_id":1,"Block":[]}`, `{"server_id":1,"block_ips":null}`, `{"server_id":1,"inherit_block":true}`, `{"server_id":1,"outbound":42}`, `{"server_id":1,"inherit_outbound":false}`} {
		service := &noLegacyService{}
		ctx := app.NewContext(0)
		ctx.Request.Header.SetContentTypeBytes([]byte("application/json"))
		ctx.Request.SetBodyString(body)
		UpdateServerNodeConfigHandler(service)(context.Background(), ctx)
		var result struct {
			Code uint32 `json:"code"`
		}
		if err := json.Unmarshal(ctx.Response.Body(), &result); err != nil {
			t.Fatal(err)
		}
		if ctx.Response.StatusCode() != 422 || result.Code != xerr.NetworkLegacyRoutingRemoved || service.called {
			t.Fatalf("body %s status %d response %s", body, ctx.Response.StatusCode(), ctx.Response.Body())
		}
	}
}
