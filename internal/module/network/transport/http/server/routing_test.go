package server

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route/param"
	serverv1 "github.com/perfect-panel/server/api/server/v1"
	"github.com/perfect-panel/server/internal/module/network"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/pkg/xerr"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

type routingTestService struct {
	network.Service
	response *dto.QueryServerConfigResponse
}

func (s routingTestService) QueryServerProtocolConfig(context.Context, *dto.QueryServerConfigRequest) (*dto.QueryServerConfigResponse, error) {
	return s.response, nil
}

func routeTestPolicy() *dto.NodeRoutingPolicy {
	flag := false
	return &dto.NodeRoutingPolicy{
		Version: 1, Mode: "ordered-v1-local", Source: "server", SourceServerID: 7,
		SourceRevision: 42, LocalTransitionRevision: 42, DefaultOutboundID: "direct",
		Rules: []dto.NodeRoutingRule{{ID: "rr_00000001-0000-4000-8000-000000000001", Enabled: &flag, Kind: "domain", Values: []string{"full:example.com"}, OutboundID: "ob_00000001-0000-4000-8000-000000000001"}},
		Outbounds: []dto.NodeOutbound{{
			ID: "ob_00000001-0000-4000-8000-000000000001", Name: "Proxy", Protocol: "vless", Address: "example.com", Port: 443,
			User: "user", Password: "password", UUID: "uuid", Cipher: "cipher", Plugin: "plugin", PluginOptions: map[string]any{"key": "value"},
			Security: "reality", SNI: "sni", ALPN: []string{"h2", "http/1.1"}, AllowInsecure: true, Fingerprint: "chrome",
			Transport: "xhttp", Host: "host", Path: "/path", ServiceName: "service", XHTTPMode: "auto", XHTTPExtra: `{}`, Flow: "vision",
			Encryption: "mlkem", EncryptionMode: "native", EncryptionRTT: "1rtt", EncryptionTicket: "ticket", EncryptionPadding: "padding", EncryptionPassword: "secret",
			Multiplex: "medium", UoT: true, UoTVersion: 2, CongestionController: "bbr", UDPStream: true, ReduceRtt: true, Heartbeat: 30,
			RealityPublicKey: "public", RealityShortId: "short", SpiderX: "/spider", Settings: ` {"raw":true} `, StreamSettings: `{"network":"tcp"}`,
			Rules: []string{}, IPRules: []string{}, ProtocolRules: []string{},
		}},
	}
}

func TestOrderedRoutingJSONProtoProjection(t *testing.T) {
	response := &dto.QueryServerConfigResponse{Routing: routeTestPolicy(), DNS: []dto.NodeDNS{}, Protocols: []dto.Protocol{}}
	if err := validateRoutingDelivery(response, 7); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"block", "block_ips", "block_protocols", "outbound"} {
		if _, ok := fields[field]; ok {
			t.Fatalf("legacy field %q in ordered JSON: %s", field, body)
		}
	}
	var decoded dto.QueryServerConfigResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.Routing, response.Routing) {
		t.Fatalf("JSON changed routing\n%#v\n%#v", decoded.Routing, response.Routing)
	}
	msg, err := queryServerProtocolConfigResponseToProtobuf(response)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var round serverv1.QueryServerProtocolConfigResponse
	if err := proto.Unmarshal(wire, &round); err != nil {
		t.Fatal(err)
	}
	p := round.Data.Routing
	if p.Version != 1 || p.Mode != "ordered-v1-local" || p.Source != "server" || p.SourceServerId != 7 || p.SourceRevision != 42 || p.LocalTransitionRevision != 42 || p.DefaultOutboundId != "direct" {
		t.Fatalf("identity = %v", p)
	}
	if len(p.Rules) != 1 || p.Rules[0].Enabled == nil || *p.Rules[0].Enabled || p.Rules[0].Kind != serverv1.RouteKind_ROUTE_KIND_DOMAIN || !reflect.DeepEqual(p.Rules[0].Values, []string{"full:example.com"}) {
		t.Fatalf("rule = %v", p.Rules)
	}
	o := p.Outbounds[0]
	// Project every backend Outbound field back from the protobuf message.
	projected := dto.NodeOutbound{
		ID: o.Id, Name: o.Name, Protocol: o.Protocol, Address: o.Address, Port: o.Port, User: o.User, Password: o.Password,
		UUID: o.Uuid, Cipher: o.Cipher, Plugin: o.Plugin, PluginOptions: o.PluginOptions.AsInterface(), Security: o.Security, SNI: o.Sni,
		ALPN: o.Alpn, AllowInsecure: o.AllowInsecure, Fingerprint: o.Fingerprint, Transport: o.Transport, Host: o.Host, Path: o.Path,
		ServiceName: o.ServiceName, XHTTPMode: o.XhttpMode, XHTTPExtra: o.XhttpExtra, Flow: o.Flow, Encryption: o.Encryption,
		EncryptionMode: o.EncryptionMode, EncryptionRTT: o.EncryptionRtt, EncryptionTicket: o.EncryptionTicket, EncryptionPadding: o.EncryptionClientPadding,
		EncryptionPassword: o.EncryptionPassword, Multiplex: o.Multiplex, UoT: o.Uot, UoTVersion: int(o.UotVersion), CongestionController: o.CongestionController,
		UDPStream: o.UdpStream, ReduceRtt: o.ReduceRtt, Heartbeat: int(o.Heartbeat), RealityPublicKey: o.RealityPublicKey, RealityShortId: o.RealityShortId,
		SpiderX: o.SpiderX, Settings: o.Settings, StreamSettings: o.StreamSettings, Rules: []string{}, IPRules: []string{}, ProtocolRules: []string{},
	}
	if !reflect.DeepEqual(projected, decoded.Routing.Outbounds[0]) {
		t.Fatalf("JSON vs proto outbound\n%#v\n%#v", decoded.Routing.Outbounds[0], projected)
	}
	if (&serverv1.Outbound{}).ProtoReflect().Descriptor().Fields().ByName("id").Number() != 44 ||
		(&serverv1.QueryServerProtocolConfigData{}).ProtoReflect().Descriptor().Fields().ByName("routing").Number() != 12 ||
		!(&serverv1.RouteRule{}).ProtoReflect().Descriptor().Fields().ByName("enabled").HasPresence() {
		t.Fatal("protobuf routing tags or enabled presence changed")
	}
}

func queryContext(accept, capability, etag string) *app.RequestContext {
	ctx := app.NewContext(0)
	ctx.Params = param.Params{{Key: "server_id", Value: "7"}}
	ctx.Request.SetRequestURI("/v2/server/7?secret_key=secret")
	if accept != "" {
		ctx.Request.Header.Set("Accept", accept)
	}
	if capability != "" {
		ctx.Request.Header.Set(routingVersionHeader, capability)
	}
	if etag != "" {
		ctx.Request.Header.Set("If-None-Match", etag)
	}
	return ctx
}

func TestRoutingCapabilityCheckedBefore304(t *testing.T) {
	for _, accept := range []string{"application/json", "application/protobuf"} {
		t.Run(accept, func(t *testing.T) {
			handler := QueryServerProtocolConfigHandler(routingTestService{response: &dto.QueryServerConfigResponse{Routing: routeTestPolicy()}}, func() string { return "secret" })
			first := queryContext(accept, "1", "")
			handler(context.Background(), first)
			if first.Response.StatusCode() != 200 {
				t.Fatalf("first status %d, body %s", first.Response.StatusCode(), first.Response.Body())
			}
			etag := string(first.Response.Header.Peek("ETag"))
			if etag == "" {
				t.Fatal("no ETag")
			}
			if vary := string(first.Response.Header.Peek("Vary")); !strings.Contains(vary, "Accept") || !strings.Contains(vary, routingVersionHeader) {
				t.Fatalf("Vary = %q", vary)
			}
			cached := queryContext(accept, "1", etag)
			handler(context.Background(), cached)
			if cached.Response.StatusCode() != 304 {
				t.Fatalf("supported status %d", cached.Response.StatusCode())
			}
			for _, capability := range []string{"", "2", "0"} {
				old := queryContext(accept, capability, etag)
				handler(context.Background(), old)
				if old.Response.StatusCode() != 426 {
					t.Fatalf("capability %q status %d", capability, old.Response.StatusCode())
				}
				if accept == protobufContentType {
					var result serverv1.Result
					if err := proto.Unmarshal(old.Response.Body(), &result); err != nil || result.Code != xerr.NodeRoutingUpgradeRequired {
						t.Fatalf("protobuf error = %v, %v", &result, err)
					}
				} else {
					var result struct {
						Code uint32 `json:"code"`
					}
					if err := json.Unmarshal(old.Response.Body(), &result); err != nil || result.Code != xerr.NodeRoutingUpgradeRequired {
						t.Fatalf("JSON error = %#v, %v", result, err)
					}
				}
			}
		})
	}
}

func TestRoutingPluginOptionsMustSurviveBothWires(t *testing.T) {
	for _, test := range []struct {
		value any
		ok    bool
	}{
		{map[string]any{"n": json.Number("0.1"), "nested": []any{true, "x"}}, true},
		{map[string]any{"n": json.Number("9007199254740993")}, false},
	} {
		response := &dto.QueryServerConfigResponse{Routing: routeTestPolicy()}
		response.Routing.Outbounds[0].PluginOptions = test.value
		err := validateRoutingDelivery(response, 7)
		if (err == nil) != test.ok {
			t.Errorf("options %v: error = %v, want accepted=%v", test.value, err, test.ok)
		}
	}
}

func TestProtobufETagDeterministicForMaps(t *testing.T) {
	first, err := structpb.NewStruct(map[string]any{"first": "one", "second": "two", "third": "three"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := structpb.NewStruct(map[string]any{"third": "three", "second": "two", "first": "one"})
	if err != nil {
		t.Fatal(err)
	}
	var etag string
	for i := 0; i < 30; i++ {
		value := first
		if i%2 == 1 {
			value = second
		}
		ctx := app.NewContext(0)
		if err := writeServerProtobufWithETag(ctx, &serverv1.ServerConfigData{Config: value}, ""); err != nil {
			t.Fatal(err)
		}
		got := string(ctx.Response.Header.Peek("ETag"))
		if i > 0 && got != etag {
			t.Fatalf("map order changed ETag: %q vs %q", got, etag)
		}
		etag = got
	}
}

func TestRoutingHTTPProtocolOneToZeroIsArray(t *testing.T) {
	response := &dto.QueryServerConfigResponse{Routing: routeTestPolicy(), Protocols: []dto.Protocol{{Type: "vless", Port: 443, Enable: true}}, Total: 1}
	handler := QueryServerProtocolConfigHandler(routingTestService{response: response}, func() string { return "secret" })
	first := queryContext("application/json", "1", "")
	handler(context.Background(), first)
	if first.Response.StatusCode() != 200 {
		t.Fatalf("first status %d", first.Response.StatusCode())
	}
	response.Protocols = nil
	response.Total = 0
	second := queryContext("application/json", "1", "")
	handler(context.Background(), second)
	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(second.Response.Body(), &envelope); err != nil {
		t.Fatal(err)
	}
	if string(envelope.Data["protocols"]) != "[]" {
		t.Fatalf("zero protocol response %s", second.Response.Body())
	}
}

func TestRoutingDeliveryFailsClosed(t *testing.T) {
	for name, edit := range map[string]func(*dto.QueryServerConfigResponse){
		"missing policy":  func(r *dto.QueryServerConfigResponse) { r.Routing = nil },
		"missing enabled": func(r *dto.QueryServerConfigResponse) { r.Routing.Rules[0].Enabled = nil },
		"unknown version": func(r *dto.QueryServerConfigResponse) { r.Routing.Version = 2 },
		"unknown kind":    func(r *dto.QueryServerConfigResponse) { r.Routing.Rules[0].Kind = "future" },
		"wrong source":    func(r *dto.QueryServerConfigResponse) { r.Routing.SourceServerID = 8 },
		"bad reference":   func(r *dto.QueryServerConfigResponse) { r.Routing.DefaultOutboundID = "missing" },
	} {
		t.Run(name, func(t *testing.T) {
			response := &dto.QueryServerConfigResponse{Routing: routeTestPolicy()}
			edit(response)
			if err := validateRoutingDelivery(response, 7); err == nil {
				t.Fatal("accepted invalid response")
			}
			for _, accept := range []string{"application/json", protobufContentType} {
				ctx := queryContext(accept, "1", `"stale"`)
				QueryServerProtocolConfigHandler(routingTestService{response: response}, func() string { return "secret" })(context.Background(), ctx)
				if ctx.Response.StatusCode() == 304 {
					t.Fatal("invalid response produced 304")
				}
			}
		})
	}
}
