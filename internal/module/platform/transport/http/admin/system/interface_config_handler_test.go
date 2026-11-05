package system

import (
	"context"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/platform"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
)

type interfaceConfigService struct {
	platform.Service
	called bool
}

func (s *interfaceConfigService) UpdateInterfaceConfig(_ context.Context, _ *dto.InterfaceConfig) error {
	s.called = true
	return nil
}

func TestInterfaceConfigRejectsInvalidJSONBinding(t *testing.T) {
	for _, tc := range []struct {
		body, contentType string
		want              bool
	}{
		{`{"hiddenMenuIds":[]}`, "application/json", true},
		{`{"hiddenMenuIds":[],"other":1}`, "application/json", false},
		{`{"hiddenMenuIds":[]} {}`, "application/json", false},
		{`null`, "application/json", false},
		{`{"hiddenMenuIds":[]}`, "text/plain", false},
		{`{"hiddenMenuIds":"bad"}`, "application/json", false},
	} {
		svc := &interfaceConfigService{}
		c := app.NewContext(0)
		c.Request.Header.SetContentTypeBytes([]byte(tc.contentType))
		c.Request.SetBodyString(tc.body)
		UpdateInterfaceConfigHandler(svc)(context.Background(), c)
		if svc.called != tc.want {
			t.Errorf("%s %s called=%v response=%s", tc.contentType, tc.body, svc.called, c.Response.Body())
		}
	}
}
