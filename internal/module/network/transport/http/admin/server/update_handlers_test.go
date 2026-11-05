package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/route"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/pkg/xerr"
)

const (
	updateServerPath     = "/v1/admin/server/update"
	updateNodeConfigPath = "/v1/admin/server/node_config/update"
)

// serverUpdates stands in for the network facade's server updates: it
// answers with err and records the requests it was handed.
type serverUpdates struct {
	err        error
	server     []*dto.UpdateServerRequest
	nodeConfig []*dto.UpdateServerNodeConfigRequest
}

var (
	_ ServerUpdater           = (*serverUpdates)(nil)
	_ ServerNodeConfigUpdater = (*serverUpdates)(nil)
)

func (s *serverUpdates) UpdateServer(_ context.Context, req *dto.UpdateServerRequest) error {
	s.server = append(s.server, req)
	return s.err
}

func (s *serverUpdates) UpdateServerNodeConfig(_ context.Context, req *dto.UpdateServerNodeConfigRequest) error {
	s.nodeConfig = append(s.nodeConfig, req)
	return s.err
}

func (s *serverUpdates) calls() int {
	return len(s.server) + len(s.nodeConfig)
}

// adminServerUpdates serves the two update routes with svc as the facade.
func adminServerUpdates(svc *serverUpdates) *route.Engine {
	h := hertzserver.Default()
	h.POST(updateServerPath, UpdateServerHandler(svc))
	h.POST(updateNodeConfigPath, UpdateServerNodeConfigHandler(svc))
	return h.Engine
}

// postJSON posts body to path and decodes the result envelope, which is
// using the actual HTTP status for validation and conflict failures.
func postJSON(t *testing.T, engine *route.Engine, path, body string) (code uint32, msg string) {
	t.Helper()
	w := ut.PerformRequest(engine, http.MethodPost, path, &ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest && w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unexpected status %d (body %q)", w.Code, w.Body.String())
	}
	var result struct {
		Code uint32          `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode result %q: %v", w.Body.String(), err)
	}
	if result.Data != nil {
		t.Fatalf("result = %s, want no data", w.Body.String())
	}
	return result.Code, result.Msg
}

func fieldSet(fields ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		set[field] = struct{}{}
	}
	return set
}

// The update keeps a stored protocol's values for the fields the request
// leaves out, so the handler records, per protocol, every field the body
// names, including ones set to zero, false, an empty string or null: those
// clear the stored value instead of keeping it.
func TestUpdateServerHandler_recordsTheFieldsEachProtocolSets(t *testing.T) {
	svc := &serverUpdates{}
	body := `
	{"id":3,"name":"tokyo-1","address":"203.0.113.9","protocols":[
		{"type":"vless","port":443,"enable":false,"flow":""},
		{"type":"trojan","port":0,"enable":false,"ratio":0,"sni":null}
	]}`

	if code, msg := postJSON(t, adminServerUpdates(svc), updateServerPath, body); code != 200 || msg != "success" {
		t.Fatalf("result = %d %q, want success", code, msg)
	}

	want := &dto.UpdateServerRequest{
		Id: 3, Name: "tokyo-1", Address: "203.0.113.9",
		Protocols: []dto.Protocol{{Type: "vless", Port: 443}, {Type: "trojan"}},
		ProtocolFieldSets: []map[string]struct{}{
			fieldSet("type", "port", "enable", "flow"),
			fieldSet("type", "port", "enable", "ratio", "sni"),
		},
	}
	if len(svc.server) != 1 || !reflect.DeepEqual(svc.server[0], want) {
		t.Fatalf("facade requests = %+v, want %+v", svc.server, want)
	}
}

// An explicit empty list is a valid zero-listener update; omission and null
// must not accidentally erase the deployed server's listeners.
func TestUpdateServerHandler_recordsNoFieldSetsWithoutProtocols(t *testing.T) {
	for name, body := range map[string]string{"no protocols": `{"id":3}`, "null protocols": `{"id":3,"protocols":null}`} {
		t.Run(name, func(t *testing.T) {
			svc := &serverUpdates{}
			code, _ := postJSON(t, adminServerUpdates(svc), updateServerPath, body)
			if code != xerr.NetworkNodeProtocolInvalid || svc.calls() != 0 {
				t.Fatalf("code %d calls %d", code, svc.calls())
			}
		})
	}
	svc := &serverUpdates{}
	if code, _ := postJSON(t, adminServerUpdates(svc), updateServerPath, `{"id":3,"protocols":[]}`); code != 200 || len(svc.server) != 1 || len(svc.server[0].ProtocolFieldSets) != 0 {
		t.Fatalf("code %d requests %+v", code, svc.server)
	}
}

// A body either decoder rejects is a parameter error and no update is
// made. The field-set decoder reads the whole body, so trailing data the
// request binder ignores is rejected too.
func TestUpdateServerHandler_rejectsMalformedBodies(t *testing.T) {
	for name, body := range map[string]string{
		"truncated":               `{"id":3,"protocols":[{"type":"vless"`,
		"protocols not a list":    `{"id":3,"protocols":{"type":"vless"}}`,
		"protocol not an object":  `{"id":3,"protocols":["vless"]}`,
		"trailing data":           `{"id":3,"protocols":[{"type":"vless"}]} {"id":4}`,
		"list instead of request": `[{"id":3}]`,
	} {
		t.Run(name, func(t *testing.T) {
			svc := &serverUpdates{}

			code, msg := postJSON(t, adminServerUpdates(svc), updateServerPath, body)

			if (code != xerr.InvalidParams && code != xerr.NetworkNodeProtocolInvalid) || msg == "" {
				t.Fatalf("result = %d %q, want a parameter error", code, msg)
			}
			if svc.calls() != 0 {
				t.Fatalf("facade was called with %+v", svc.server)
			}
		})
	}
}

// The node configuration override is forwarded as sent: explicit false
// inherit flags replace the global value, and an explicit empty list
// differs from a missing one.
func TestUpdateServerNodeConfigHandler_forwardsTheOverride(t *testing.T) {
	svc := &serverUpdates{}
	body := `{"server_id":5,"inherit_ip_strategy":false,"ip_strategy":"prefer_ipv6",
		"inherit_dns":false,"dns":[{"proto":"udp","address":"1.1.1.1","domains":["example.com"]}]}`

	if code, msg := postJSON(t, adminServerUpdates(svc), updateNodeConfigPath, body); code != 200 || msg != "success" {
		t.Fatalf("result = %d %q, want success", code, msg)
	}

	want := &dto.UpdateServerNodeConfigRequest{ServerID: 5, ServerNodeConfigOverride: dto.ServerNodeConfigOverride{
		IPStrategy: "prefer_ipv6",
		DNS:        []dto.NodeDNS{{Proto: "udp", Address: "1.1.1.1", Domains: []string{"full:example.com"}}},
	}, RequestFields: map[string]struct{}{"server_id": {}, "inherit_ip_strategy": {}, "ip_strategy": {}, "inherit_dns": {}, "dns": {}}}
	if len(svc.nodeConfig) != 1 || !reflect.DeepEqual(svc.nodeConfig[0], want) {
		t.Fatalf("facade request = %#v, want %#v", svc.nodeConfig[0], want)
	}
}

// The override belongs to a server: without a server_id, or with a body
// that does not decode, it is a parameter error and nothing is stored.
func TestUpdateServerNodeConfigHandler_rejectsInvalidRequests(t *testing.T) {
	for name, tc := range map[string]struct{ body, msg string }{
		"no server":   {`{"inherit_dns":true}`, "ServerID is a required field"},
		"server zero": {`{"server_id":0,"inherit_dns":true}`, "ServerID is a required field"},
		"truncated":   {`{"server_id":5,"dns":[`, ""},
		"wrong type":  {`{"server_id":"five"}`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			svc := &serverUpdates{}

			code, msg := postJSON(t, adminServerUpdates(svc), updateNodeConfigPath, tc.body)

			if code != xerr.InvalidParams || msg == "" || (tc.msg != "" && msg != tc.msg) {
				t.Fatalf("result = %d %q, want a parameter error", code, msg)
			}
			if svc.calls() != 0 {
				t.Fatalf("facade was called with %+v", svc.nodeConfig)
			}
		})
	}
}

// A facade failure is reported with its code and message; one without a
// code is an internal error that does not leak its text.
func TestServerUpdateHandlers_reportFacadeErrors(t *testing.T) {
	requests := map[string]string{
		updateServerPath:     `{"id":3,"name":"tokyo-1","protocols":[{"type":"vless","enable":false}]}`,
		updateNodeConfigPath: `{"server_id":5,"inherit_dns":true}`,
	}
	for name, tc := range map[string]struct {
		err  error
		code uint32
		msg  string
	}{
		"coded": {fmt.Errorf("protocols type is empty: %w", xerr.NewErrCodeMsg(xerr.InvalidParams, "protocols type is empty")), xerr.InvalidParams, "protocols type is empty"},
		"plain": {errors.New("database is down"), xerr.ERROR, "Internal Server Error"},
	} {
		for path, body := range requests {
			t.Run(name+" "+path, func(t *testing.T) {
				svc := &serverUpdates{err: tc.err}

				code, msg := postJSON(t, adminServerUpdates(svc), path, body)

				if code != tc.code || msg != tc.msg {
					t.Fatalf("result = %d %q, want %d %q", code, msg, tc.code, tc.msg)
				}
				if svc.calls() != 1 {
					t.Fatalf("facade calls = %d, want 1", svc.calls())
				}
			})
		}
	}
}
