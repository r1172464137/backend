package system

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/httpx"
)

func TestNodeConfigHTTPBindDistinguishesOmittedAndEmptyRouteRules(t *testing.T) {
	for _, test := range []struct {
		name          string
		body          string
		blockIPsSet   bool
		outboundIPSet bool
	}{
		{name: "legacy omission", body: `{"block":[],"outbound":[{"name":"direct","protocol":"direct","rules":[]}]}`},
		{name: "explicit empty", body: `{"block":[],"block_ips":[],"outbound":[{"name":"direct","protocol":"direct","rules":[],"ip_rules":[]}]}`, blockIPsSet: true, outboundIPSet: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := app.NewContext(0)
			ctx.Request.Header.SetContentTypeBytes([]byte("application/json"))
			ctx.Request.SetBodyString(test.body)
			var request dto.NodeConfig
			if err := httpx.ShouldBind(ctx, &request); err != nil {
				t.Fatalf("ShouldBind() error = %v", err)
			}
			_, blockIPsSet := request.RequestFields["block_ips"]
			_, outboundIPSet := request.OutboundFieldSets[0]["ip_rules"]
			if blockIPsSet != test.blockIPsSet || outboundIPSet != test.outboundIPSet {
				t.Fatalf("presence block_ips=%v outbound.ip_rules=%v, want %v/%v", blockIPsSet, outboundIPSet, test.blockIPsSet, test.outboundIPSet)
			}
		})
	}
}
