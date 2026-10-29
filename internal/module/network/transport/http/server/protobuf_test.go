package server

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	serverv1 "github.com/perfect-panel/server/api/server/v1"
	appconfig "github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func newProtobufContext(t *testing.T, message proto.Message) *app.RequestContext {
	t.Helper()
	body, err := proto.Marshal(message)
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	ctx := app.NewContext(0)
	ctx.Request.Header.SetContentTypeBytes([]byte("application/protobuf; charset=binary"))
	ctx.Request.SetBody(body)
	return ctx
}

func TestBindOnlineUsersRequest_Protobuf(t *testing.T) {
	ctx := newProtobufContext(t, &serverv1.PushOnlineUsersRequest{
		Users: []*serverv1.OnlineUser{{UserId: 42, Ip: "203.0.113.1"}},
	})
	var request dto.OnlineUsersRequest
	if err := bindOnlineUsersRequest(ctx, &request); err != nil {
		t.Fatalf("bindOnlineUsersRequest() error = %v", err)
	}
	if len(request.Users) != 1 || request.Users[0].SID != 42 || request.Users[0].IP != "203.0.113.1" {
		t.Fatalf("users = %+v, want converted protobuf user", request.Users)
	}
}

func TestBindUserTrafficRequest_Protobuf(t *testing.T) {
	ctx := newProtobufContext(t, &serverv1.PushUserTrafficRequest{
		Traffic: []*serverv1.UserTraffic{{UserId: 42, Upload: 100, Download: 200}},
	})
	var request dto.ServerPushUserTrafficRequest
	if err := bindUserTrafficRequest(ctx, &request); err != nil {
		t.Fatalf("bindUserTrafficRequest() error = %v", err)
	}
	if len(request.Traffic) != 1 || request.Traffic[0] != (dto.UserTraffic{SID: 42, Upload: 100, Download: 200}) {
		t.Fatalf("traffic = %+v, want converted protobuf traffic", request.Traffic)
	}
}

func TestBindServerStatusRequest_Protobuf(t *testing.T) {
	ctx := newProtobufContext(t, &serverv1.PushServerStatusRequest{
		Cpu: 0.5, Mem: 0.6, Disk: 0.7, UpdatedAt: 123,
	})
	var request dto.ServerPushStatusRequest
	if err := bindServerStatusRequest(ctx, &request); err != nil {
		t.Fatalf("bindServerStatusRequest() error = %v", err)
	}
	if request.Cpu != 0.5 || request.Mem != 0.6 || request.Disk != 0.7 || request.UpdatedAt != 123 {
		t.Fatalf("request = %+v, want converted protobuf status", request)
	}
}

func TestBindOnlineUsersRequest_JSON(t *testing.T) {
	ctx := app.NewContext(0)
	ctx.Request.Header.SetContentTypeBytes([]byte("application/json"))
	ctx.Request.SetBodyString(`{"users":[{"uid":42,"ip":"203.0.113.1"}]}`)
	var request dto.OnlineUsersRequest
	if err := bindOnlineUsersRequest(ctx, &request); err != nil {
		t.Fatalf("bindOnlineUsersRequest() error = %v", err)
	}
	if len(request.Users) != 1 || request.Users[0].SID != 42 || request.Users[0].IP != "203.0.113.1" {
		t.Fatalf("users = %+v, want decoded JSON user", request.Users)
	}
}

func TestWriteServerReportResult_Protobuf(t *testing.T) {
	ctx := app.NewContext(0)
	ctx.Request.Header.Set("Accept", "application/protobuf, application/json;q=0.9")
	writeServerReportResult(ctx, nil)

	if got := string(ctx.Response.Header.ContentType()); got != protobufContentType {
		t.Fatalf("Content-Type = %q, want %q", got, protobufContentType)
	}
	var result serverv1.Result
	if err := proto.Unmarshal(ctx.Response.Body(), &result); err != nil {
		t.Fatalf("proto.Unmarshal() error = %v", err)
	}
	if result.Code != 200 || result.Message != "success" {
		t.Fatalf("result = %+v, want successful protobuf result", &result)
	}
}

func TestWriteServerReportResult_DefaultsToJSON(t *testing.T) {
	ctx := app.NewContext(0)
	writeServerReportResult(ctx, nil)

	if got := string(ctx.Response.Header.ContentType()); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want JSON", got)
	}
	var response struct {
		Code uint32 `json:"code"`
	}
	if err := json.Unmarshal(ctx.Response.Body(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if response.Code != 200 {
		t.Fatalf("response code = %d, want 200", response.Code)
	}
}

func TestWriteServerReportResult_ProtobufError(t *testing.T) {
	ctx := app.NewContext(0)
	ctx.Request.Header.Set("Accept", protobufContentType)
	writeServerReportResult(ctx, errors.New("report failed"))

	if got := string(ctx.Response.Header.ContentType()); got != protobufContentType {
		t.Fatalf("Content-Type = %q, want %q", got, protobufContentType)
	}
	var response serverv1.Result
	if err := proto.Unmarshal(ctx.Response.Body(), &response); err != nil {
		t.Fatalf("proto.Unmarshal() error = %v", err)
	}
	if response.Code != 500 || response.Message != "Internal Server Error" {
		t.Fatalf("response = %+v, want internal-error protobuf envelope", &response)
	}
}

func TestResponseConversions(t *testing.T) {
	users := serverUserListResponseToProtobuf(&dto.GetServerUserListResponse{Users: []dto.ServerUser{{
		Id: 1, UUID: "uuid", SpeedLimit: 10, DeviceLimit: 2,
	}}})
	if users.Code != 200 || len(users.Data.Users) != 1 || users.Data.Users[0].Uuid != "uuid" {
		t.Fatalf("users = %+v, want protobuf user-list envelope", users)
	}

	protocols, err := queryServerProtocolConfigResponseToProtobuf(&dto.QueryServerConfigResponse{
		TrafficReportThreshold: 1024,
		PushInterval:           60,
		PullInterval:           30,
		IPStrategy:             "prefer_ipv4",
		DNS: []dto.NodeDNS{{
			Proto: "https", Address: "https://dns.example/dns-query", Domains: []string{"example.com"},
		}},
		Routing: routeTestPolicy(),
		Protocols: []dto.Protocol{
			{
				Type: "vless", Port: 443, Enable: true, Transport: "xhttp", XhttpMode: "auto",
				PluginOptions: map[string]string{"host": "example.com"},
			},
			{
				Type: "nowhere", Port: 8443, Version: 1, Enable: true, Security: "tls", Network: "mix",
				SNI: "nowhere.example", ALPN: []string{"now/1"}, CertMode: "self",
			},
		},
		Total: 2,
	})
	if err != nil {
		t.Fatalf("queryServerProtocolConfigResponseToProtobuf() error = %v", err)
	}
	if protocols.Code != 200 || protocols.Data.Total != 2 || protocols.Data.IpStrategy != "prefer_ipv4" {
		t.Fatalf("protocols = %+v, want protobuf protocol-config envelope", protocols)
	}
	if len(protocols.Data.Dns) != 1 || protocols.Data.Dns[0].Address != "https://dns.example/dns-query" {
		t.Fatalf("dns = %+v, want strongly typed DNS configuration", protocols.Data.Dns)
	}
	if protocols.Data.Routing == nil || len(protocols.Data.Routing.Outbounds) != 1 || len(protocols.Data.Outbound) != 0 || len(protocols.Data.Block) != 0 {
		t.Fatalf("ordered-only protobuf projection = %+v", protocols.Data)
	}
	wire, err := proto.Marshal(protocols)
	if err != nil {
		t.Fatal(err)
	}
	var roundTripped serverv1.QueryServerProtocolConfigResponse
	if err := proto.Unmarshal(wire, &roundTripped); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(protocols, &roundTripped) {
		t.Fatalf("protobuf exact round trip changed response\nwant: %v\ngot:  %v", protocols, &roundTripped)
	}
	if len(protocols.Data.Protocols) != 2 || protocols.Data.Protocols[0].XhttpMode != "auto" || protocols.Data.Protocols[0].PluginOptions.GetStructValue().Fields["host"].GetStringValue() != "example.com" {
		t.Fatalf("protocols = %+v, want strongly typed protocol configuration", protocols.Data.Protocols)
	}
	nowhere := protocols.Data.Protocols[1]
	if nowhere.Type != "nowhere" || nowhere.Port != 8443 || nowhere.Version != 1 ||
		nowhere.Security != "tls" || nowhere.Network != "mix" || nowhere.Sni != "nowhere.example" ||
		len(nowhere.Alpn) != 1 || nowhere.Alpn[0] != "now/1" || nowhere.CertMode != "self" {
		t.Fatalf("Nowhere protobuf protocol = %+v, want complete server contract", nowhere)
	}
}

func TestStructuredRouteCanonicalCrosswireProjection(t *testing.T) {
	domains, err := appconfig.NormalizeDomainRules("outbound[1].rules", []string{
		"example.com.", "KEYWORD:tracker", "SUFFIX:example.org.", `regex:(?i)(^|\.)announce\.`, "geosite:category-ai-!cn@ads",
	})
	if err != nil {
		t.Fatal(err)
	}
	ips, err := appconfig.NormalizeIPRules("outbound[1].ip_rules", []string{"geoip:cn_test-1", "10.2.3.4/8"})
	if err != nil {
		t.Fatal(err)
	}
	protocolRules, err := appconfig.NormalizeProtocolRules("outbound[1].protocol_rules", []string{"BitTorrent"})
	if err != nil {
		t.Fatal(err)
	}
	p := routeTestPolicy()
	enabled := true
	p.Rules = []dto.NodeRoutingRule{
		{ID: "rr_00000001-0000-4000-8000-000000000001", Enabled: &enabled, Kind: "domain", Values: domains, OutboundID: "direct"},
		{ID: "rr_00000002-0000-4000-8000-000000000002", Enabled: &enabled, Kind: "ip", Values: ips, OutboundID: "direct"},
		{ID: "rr_00000003-0000-4000-8000-000000000003", Enabled: &enabled, Kind: "protocol", Values: protocolRules, OutboundID: "block"},
	}
	converted, err := queryServerProtocolConfigResponseToProtobuf(&dto.QueryServerConfigResponse{Routing: p})
	if err != nil {
		t.Fatal(err)
	}
	wantDomains := []string{"full:example.com", "keyword:tracker", "domain:example.org", `regexp:(?i)(^|\.)announce\.`, "geosite:category-ai-!cn@ads"}
	wantIPs := []string{"geoip:cn_test-1", "10.0.0.0/8"}
	if !reflect.DeepEqual(converted.Data.Routing.Rules[0].Values, wantDomains) || !reflect.DeepEqual(converted.Data.Routing.Rules[1].Values, wantIPs) ||
		!reflect.DeepEqual(converted.Data.Routing.Rules[2].Values, []string{"bittorrent"}) {
		t.Fatalf("crosswire protobuf projection = %#v", converted.Data)
	}
}

func TestReservedLegacyProtoTagsRemainStable(t *testing.T) {
	queryFields := (&serverv1.QueryServerProtocolConfigData{}).ProtoReflect().Descriptor().Fields()
	outboundFields := (&serverv1.Outbound{}).ProtoReflect().Descriptor().Fields()
	for name, want := range map[string]int{"traffic_report_threshold": 1, "push_interval": 2, "pull_interval": 3, "ip_strategy": 4, "dns": 5, "block": 6, "outbound": 7, "protocols": 8, "total": 9} {
		if got := int(queryFields.ByName(protoreflect.Name(name)).Number()); got != want {
			t.Errorf("QueryServerProtocolConfigData.%s tag = %d, want unchanged %d", name, got, want)
		}
	}
	for name, want := range map[string]int{"name": 1, "protocol": 2, "stream_settings": 40, "rules": 41} {
		if got := int(outboundFields.ByName(protoreflect.Name(name)).Number()); got != want {
			t.Errorf("Outbound.%s tag = %d, want unchanged %d", name, got, want)
		}
	}
	if got := queryFields.ByName("block_ips").Number(); got != 10 {
		t.Fatalf("block_ips tag = %d, want 10", got)
	}
	if got := queryFields.ByName("block_protocols").Number(); got != 11 {
		t.Fatalf("block_protocols tag = %d, want 11", got)
	}
	if got := outboundFields.ByName("ip_rules").Number(); got != 42 {
		t.Fatalf("ip_rules tag = %d, want 42", got)
	}
	if got := outboundFields.ByName("protocol_rules").Number(); got != 43 {
		t.Fatalf("protocol_rules tag = %d, want 43", got)
	}

}

func TestAcceptsProtobuf_RejectsZeroQuality(t *testing.T) {
	ctx := app.NewContext(0)
	ctx.Request.Header.Set("Accept", "application/protobuf;q=0, application/json")
	if acceptsProtobuf(ctx) {
		t.Fatal("acceptsProtobuf() = true, want false for q=0")
	}
}

func TestWriteServerProtobufWithETag(t *testing.T) {
	message := &serverv1.Result{Code: 200, Message: "success"}

	ctx := app.NewContext(0)
	if err := writeServerProtobufWithETag(ctx, message, ""); err != nil {
		t.Fatalf("writeServerProtobufWithETag() error = %v", err)
	}
	etag := string(ctx.Response.Header.Peek("ETag"))
	if etag == "" || string(ctx.Response.Header.ContentType()) != protobufContentType {
		t.Fatalf("response headers = %+v, want Protobuf content type and ETag", &ctx.Response.Header)
	}

	ctx = app.NewContext(0)
	if err := writeServerProtobufWithETag(ctx, message, etag); err != nil {
		t.Fatalf("writeServerProtobufWithETag() error = %v", err)
	}
	if got := ctx.Response.StatusCode(); got != 304 {
		t.Fatalf("status = %d, want 304", got)
	}
}
