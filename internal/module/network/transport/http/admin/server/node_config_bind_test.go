package server

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/pkg/httpx"
)

func TestServerNodeConfigHTTPBindDistinguishesOmittedAndEmptyRouteRules(t *testing.T) {
	for _, test := range []struct {
		name              string
		body              string
		blockProtocolsSet bool
		outboundProtoSet  bool
	}{
		{name: "legacy omission", body: `{"server_id":1,"inherit_block":false,"block":[],"inherit_outbound":false,"outbound":[{"name":"direct","protocol":"direct","rules":[]}]}`},
		{name: "explicit empty", body: `{"server_id":1,"inherit_block":false,"block":[],"block_protocols":[],"inherit_outbound":false,"outbound":[{"name":"direct","protocol":"direct","rules":[],"protocol_rules":[]}]}`, blockProtocolsSet: true, outboundProtoSet: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := app.NewContext(0)
			ctx.Request.Header.SetContentTypeBytes([]byte("application/json"))
			ctx.Request.SetBodyString(test.body)
			var request dto.UpdateServerNodeConfigRequest
			if err := httpx.ShouldBind(ctx, &request); err != nil {
				t.Fatalf("ShouldBind() error = %v", err)
			}
			_, blockProtocolsSet := request.RequestFields["block_protocols"]
			_, outboundProtoSet := request.OutboundFieldSets[0]["protocol_rules"]
			if blockProtocolsSet != test.blockProtocolsSet || outboundProtoSet != test.outboundProtoSet {
				t.Fatalf("presence block_protocols=%v outbound.protocol_rules=%v, want %v/%v", blockProtocolsSet, outboundProtoSet, test.blockProtocolsSet, test.outboundProtoSet)
			}
		})
	}
}
