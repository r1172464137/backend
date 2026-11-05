package server

import (
	"encoding/json"
	"math"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/protocol"
	"github.com/cloudwego/hertz/pkg/route"

	serverv1 "github.com/perfect-panel/server/api/server/v1"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/xerr"
	"google.golang.org/protobuf/types/known/structpb"
)

func sendV2(engine *route.Engine, target string, headers ...ut.Header) *protocol.Response {
	headers = append(headers, ut.Header{Key: routingVersionHeader, Value: "1"})
	return send(engine, http.MethodGet, target, nil, headers...)
}

func protocolRouting() *dto.NodeRoutingPolicy { p := routeTestPolicy(); p.SourceServerID = 9; return p }

const protocolConfigTarget = "/v2/server/9?secret_key=" + testNodeSecret

// protocolConfig is a node configuration as the facade builds it; its
// plugin options are a map, which protobuf carries as a Struct.
func protocolConfig() *dto.QueryServerConfigResponse {
	return &dto.QueryServerConfigResponse{
		TrafficReportThreshold: 1024,
		PushInterval:           60,
		PullInterval:           30,
		IPStrategy:             "prefer_ipv4",
		DNS:                    []dto.NodeDNS{{Proto: "https", Address: "https://dns.example/dns-query", Domains: []string{"example.com"}}},
		Routing:                protocolRouting(),
		Protocols: []dto.Protocol{{
			Type: "shadowsocks", Port: 8388, Enable: true, Cipher: "aes-256-gcm", Plugin: "obfs",
			PluginOptions: map[string]any{"mode": "tls", "host": "cdn.example", "fast-open": true},
		}},
		Total: 1,
	}
}

// The server comes from the path: a server_id that is not a number is a
// 400, answered before the node secret is checked.
func TestQueryServerProtocolConfig_rejectsANonNumericServerID(t *testing.T) {
	svc := &nodeService{protocols: protocolConfig()}
	engine := nodeAPI(svc, testNodeSecret)

	resp := sendV2(engine, "/v2/server/abc?secret_key="+testNodeSecret)
	assertText(t, resp, http.StatusBadRequest, "Invalid Params")
	assertHeader(t, resp, "Vary", "Accept, "+routingVersionHeader)

	resp = sendV2(engine, "/v2/server/abc", acceptProtobuf)
	assertProtobufResult(t, resp, http.StatusBadRequest, http.StatusBadRequest, "Invalid Params")
	assertFacadeNotCalled(t, svc)
}

// The v2 pull is outside the /v1 group and checks the node secret itself:
// its refusal is a 401, not the group's 403.
func TestQueryServerProtocolConfig_requiresTheNodeSecret(t *testing.T) {
	for name, tc := range map[string]struct{ provisioned, target string }{
		"no secret":    {testNodeSecret, "/v2/server/9"},
		"wrong secret": {testNodeSecret, "/v2/server/9?secret_key=guess"},
		// An unprovisioned secret must not let an empty secret_key through.
		"unprovisioned secret": {"", "/v2/server/9?secret_key="},
	} {
		t.Run(name, func(t *testing.T) {
			svc := &nodeService{protocols: protocolConfig()}
			engine := nodeAPI(svc, tc.provisioned)

			resp := sendV2(engine, tc.target)
			assertText(t, resp, http.StatusUnauthorized, "Unauthorized")
			assertHeader(t, resp, "Vary", "Accept, "+routingVersionHeader)
			resp = sendV2(engine, tc.target, acceptProtobuf)
			assertProtobufResult(t, resp, http.StatusUnauthorized, http.StatusUnauthorized, "Unauthorized")
			assertFacadeNotCalled(t, svc)
		})
	}
}

// A node names the protocols it runs with repeated protocols or protocols[]
// parameters; the facade receives all of them (the plain ones first), with
// the server from the path.
func TestQueryServerProtocolConfig_forwardsTheServerAndItsProtocols(t *testing.T) {
	svc := &nodeService{protocols: protocolConfig()}

	sendV2(nodeAPI(svc, testNodeSecret), protocolConfigTarget+"&protocols=vless&protocols[]=trojan&protocols=vmess")

	assertFacadeRequest(t, svc, &dto.QueryServerConfigRequest{
		ServerID: 9, SecretKey: testNodeSecret, RoutingVersion: 1, Protocols: []string{"vless", "vmess", "trojan"},
	})
}

// A JSON pull is answered in the result envelope, with an ETag of the
// configuration (not of the envelope) and a 304 once the node holds it.
func TestQueryServerProtocolConfig_answersJSONWithAnETagOfTheConfiguration(t *testing.T) {
	svc := &nodeService{protocols: protocolConfig()}
	engine := nodeAPI(svc, testNodeSecret)
	data, err := json.Marshal(protocolConfig())
	if err != nil {
		t.Fatal(err)
	}
	etag := httpx.GenerateETag(data)

	resp := sendV2(engine, protocolConfigTarget, ifNoneMatch("stale"))
	result := decodeJSONResult(t, resp)
	if result.Code != 200 || result.Msg != "success" {
		t.Fatalf("result = %s, want success", resp.Body())
	}
	assertJSON(t, result.Data, string(data))
	assertHeader(t, resp, "ETag", etag)
	assertHeader(t, resp, "Vary", "Accept, "+routingVersionHeader)

	resp = sendV2(engine, protocolConfigTarget, ifNoneMatch(etag))
	assertStatus(t, resp, http.StatusNotModified)
	assertHeader(t, resp, "ETag", etag)
	if len(resp.Body()) != 0 {
		t.Fatalf("304 body = %q, want none", resp.Body())
	}
}

// A protobuf pull carries the typed configuration with an ETag of the body,
// the same on every pull of one configuration, and a 304 once the node
// holds it.
func TestQueryServerProtocolConfig_answersProtobufWithAnETagOfTheBody(t *testing.T) {
	svc := &nodeService{protocols: protocolConfig()}
	engine := nodeAPI(svc, testNodeSecret)
	pluginOptions, err := structpb.NewValue(map[string]any{"mode": "tls", "host": "cdn.example", "fast-open": true})
	if err != nil {
		t.Fatal(err)
	}
	routing, err := routingToProtobuf(protocolRouting())
	if err != nil {
		t.Fatal(err)
	}
	want := &serverv1.QueryServerProtocolConfigResponse{Code: 200, Message: "success", Data: &serverv1.QueryServerProtocolConfigData{
		TrafficReportThreshold: 1024,
		PushInterval:           60,
		PullInterval:           30,
		IpStrategy:             "prefer_ipv4",
		Dns:                    []*serverv1.DNSResolver{{Proto: "https", Address: "https://dns.example/dns-query", Domains: []string{"example.com"}}},
		Routing:                routing,
		Protocols: []*serverv1.ServerProtocol{{
			Type: "shadowsocks", Port: 8388, Enable: true, Cipher: "aes-256-gcm", Plugin: "obfs", PluginOptions: pluginOptions,
		}},
		Total: 1,
	}}

	resp := sendV2(engine, protocolConfigTarget, acceptProtobuf, ifNoneMatch("stale"))
	assertProtobuf(t, resp, http.StatusOK, want)
	assertHeader(t, resp, "Vary", "Accept, "+routingVersionHeader)
	etag := string(resp.Header.Peek("ETag"))
	if etag != httpx.GenerateETag(resp.Body()) {
		t.Fatalf("ETag = %q, want the ETag of the protobuf body", etag)
	}
	for range 20 {
		assertHeader(t, sendV2(engine, protocolConfigTarget, acceptProtobuf), "ETag", etag)
	}

	resp = sendV2(engine, protocolConfigTarget, acceptProtobuf, ifNoneMatch(etag))
	assertStatus(t, resp, http.StatusNotModified)
	assertHeader(t, resp, "ETag", etag)
	if len(resp.Body()) != 0 {
		t.Fatalf("304 body = %q, want none", resp.Body())
	}
}

// A configuration that cannot be encoded is an internal-error result in
// the negotiated encoding, rather than a partial body.
func TestQueryServerProtocolConfig_answersAnInternalErrorForAnUnencodableConfiguration(t *testing.T) {
	// JSON has no NaN, and the protobuf plugin options are built from JSON.
	for name, nan := range map[string]*dto.QueryServerConfigResponse{
		"protocol": {Routing: protocolRouting(), Protocols: []dto.Protocol{{Type: "shadowsocks", PluginOptions: math.NaN()}}, Total: 1},
		"outbound": {Routing: protocolRouting()},
	} {
		t.Run(name+" plugin options JSON cannot encode", func(t *testing.T) {
			if name == "outbound" {
				nan.Routing.Outbounds[0].PluginOptions = map[string]any{"ratio": math.NaN()}
			}
			engine := nodeAPI(&nodeService{protocols: nan}, testNodeSecret)
			assertJSONResult(t, sendV2(engine, protocolConfigTarget), xerr.ERROR, "Internal Server Error")
			resp := sendV2(engine, protocolConfigTarget, acceptProtobuf)
			assertProtobufResult(t, resp, http.StatusOK, xerr.ERROR, "Internal Server Error")
		})
	}

	// A protobuf string must be UTF-8.
	notUTF8 := &dto.QueryServerConfigResponse{IPStrategy: "\xff", Routing: protocolRouting()}
	resp := sendV2(nodeAPI(&nodeService{protocols: notUTF8}, testNodeSecret), protocolConfigTarget, acceptProtobuf)
	assertProtobufResult(t, resp, http.StatusOK, xerr.ERROR, "Internal Server Error")
}
