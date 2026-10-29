package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/cloudwego/hertz/pkg/app"
	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	serverv1 "github.com/perfect-panel/server/api/server/v1"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/network/internal/routingstatus"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
)

type badStatusRepo struct{ repository.NodeRepo }

func (badStatusRepo) FindOneServer(context.Context, int64) (*node.Server, error) {
	return &node.Server{Id: 7, Protocols: `[]`}, nil
}
func (badStatusRepo) UpdateStatusCache(context.Context, int64, *node.Status) error { return nil }

type badStatusStore struct {
	network.Store
	nodes repository.NodeRepo
}

func (s badStatusStore) Node() repository.NodeRepo { return s.nodes }
func TestNodeFailureStagesJSONAndProtobuf(t *testing.T) {
	for _, stage := range []string{"config", "monitor"} {
		for _, wire := range []string{"json", "protobuf"} {
			t.Run(stage+"/"+wire, func(t *testing.T) {
				mini := miniredis.RunT(t)
				client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
				t.Cleanup(func() { _ = client.Close() })
				svc := network.New(network.Deps{Store: badStatusStore{nodes: badStatusRepo{}}, Redis: client, Config: func() network.Snapshot { return network.Snapshot{Node: config.NodeConfig{}} }})
				engine := hertzserver.Default(hertzserver.WithDisablePrintRoute(true))
				engine.Use(ServerMiddleware(func() string { return "test-secret" }))
				engine.POST("/v1/server/status", ServerPushStatusHandler(svc))
				msg := &serverv1.PushServerStatusRequest{RoutingVersion: 1, RoutingSequence: 1, InstanceId: "11111111-1111-4111-8111-111111111111", RoutingError: &serverv1.RoutingError{Stage: stage, Message: "password=do-not-store"}}
				var body []byte
				var err error
				if wire == "protobuf" {
					body, err = proto.Marshal(msg)
				} else {
					body, err = json.Marshal(map[string]any{"routing_version": 1, "routing_sequence": 1, "instance_id": msg.InstanceId, "routing_error": map[string]any{"stage": stage, "message": "password=do-not-store"}})
				}
				if err != nil {
					t.Fatal(err)
				}
				request := engine.NewContext()
				request.Request.SetRequestURI("/v1/server/status?server_id=7&secret_key=test-secret")
				request.Request.Header.SetMethod("POST")
				if wire == "protobuf" {
					request.Request.Header.SetContentTypeBytes([]byte(protobufContentType))
					request.Request.Header.Set("Accept", protobufContentType)
				} else {
					request.Request.Header.SetContentTypeBytes([]byte("application/json"))
				}
				request.Request.SetBody(body)
				engine.ServeHTTP(context.Background(), request)
				if request.Response.StatusCode() != 200 {
					t.Fatalf("status %d", request.Response.StatusCode())
				}
				if wire == "protobuf" {
					var result serverv1.Result
					if err := proto.Unmarshal(request.Response.Body(), &result); err != nil || result.Code != 200 {
						t.Fatalf("protobuf result: %v %#v", err, result)
					}
				} else {
					var result struct {
						Code int `json:"code"`
					}
					if err := json.Unmarshal(request.Response.Body(), &result); err != nil || result.Code != 200 {
						t.Fatalf("JSON result: %v %s", err, request.Response.Body())
					}
				}
				view, err := routingstatus.Read(context.Background(), client, 7)
				if err != nil || !view.Known || view.Report == nil || view.Report.Applied != nil || view.Report.Error == nil || view.Report.Error.Stage != stage || view.Report.Error.Message != "candidate failed during "+stage {
					t.Fatalf("stage report: %#v %v", view, err)
				}
				// An unknown stage must still fail closed on both wire formats.
				msg.RoutingSequence = 2
				msg.RoutingError.Stage = "untrusted-stage"
				if wire == "protobuf" {
					body, err = proto.Marshal(msg)
				} else {
					body, err = json.Marshal(map[string]any{"routing_version": 1, "routing_sequence": 2, "instance_id": msg.InstanceId, "routing_error": map[string]any{"stage": "untrusted-stage", "message": "password=do-not-store"}})
				}
				if err != nil {
					t.Fatal(err)
				}
				bad := engine.NewContext()
				bad.Request.SetRequestURI("/v1/server/status?server_id=7&secret_key=test-secret")
				bad.Request.Header.SetMethod("POST")
				if wire == "protobuf" {
					bad.Request.Header.SetContentTypeBytes([]byte(protobufContentType))
					bad.Request.Header.Set("Accept", protobufContentType)
				} else {
					bad.Request.Header.SetContentTypeBytes([]byte("application/json"))
				}
				bad.Request.SetBody(body)
				engine.ServeHTTP(context.Background(), bad)
				view, err = routingstatus.Read(context.Background(), client, 7)
				if err != nil || view.Known || strings.Contains(string(bad.Response.Body()), "password=do-not-store") {
					t.Fatalf("unknown stage not sanitized/unknown: %#v %v", view, err)
				}
			})
		}
	}
}

func TestMalformedStatusInvalidatesAuthenticatedServerState(t *testing.T) {
	for _, tc := range []struct {
		name, contentType string
		bad               []byte
	}{
		{"json", "application/json", []byte(`{"cpu": "secret-token",`)},
		{"json_top_typo", "application/json", []byte(`{"routing_pendng":{}}`)},
		{"json_case_variant", "application/json", []byte(`{"Routing_Version":1}`)},
		{"json_duplicate_casefold", "application/json", []byte(`{"cpu":1,"CPU":2}`)},
		{"json_identity_typo", "application/json", []byte(`{"routing_applied":{"source_revison":1}}`)},
		{"json_error_typo", "application/json", []byte(`{"routing_error":{"stage":"bind","mesage":"secret-token"}}`)},
		{"json_error_identity_typo", "application/json", []byte(`{"routing_error":{"identity":{"source_revison":1},"stage":"bind"}}`)},
		{"protobuf", protobufContentType, []byte{0x32, 0x05, 0x01}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: server.Addr()})
			t.Cleanup(func() { client.Close() })
			svc := network.New(network.Deps{Store: badStatusStore{nodes: badStatusRepo{}}, Redis: client, Config: func() network.Snapshot { return network.Snapshot{Node: config.NodeConfig{}} }})
			engine := hertzserver.Default(hertzserver.WithDisablePrintRoute(true))
			engine.Use(ServerMiddleware(func() string { return "test-secret" }))
			engine.POST("/v1/server/status", ServerPushStatusHandler(svc))
			makeCtx := func(contentType string, body []byte, secret string) *app.RequestContext {
				ctx := engine.NewContext()
				ctx.Request.SetRequestURI("/v1/server/status?server_id=7&secret_key=" + secret)
				ctx.Request.Header.SetMethod("POST")
				ctx.Request.Header.SetContentTypeBytes([]byte(contentType))
				ctx.Request.SetBody(body)
				return ctx
			}
			good := makeCtx("application/json", []byte(`{"cpu":0.1,"updated_at":1,"routing_version":1,"routing_sequence":1,"instance_id":"11111111-1111-4111-8111-111111111111","routing_applied":{"mode":"ordered-v1-inherited","source":"global","source_server_id":0,"source_revision":2,"local_transition_revision":0}}`), "test-secret")
			engine.ServeHTTP(context.Background(), good)
			state, err := routingstatus.Read(context.Background(), client, 7)
			if err != nil || !state.Known {
				t.Fatalf("valid report %#v %v", state, err)
			}
			unauthorized := makeCtx(tc.contentType, tc.bad, "wrong")
			engine.ServeHTTP(context.Background(), unauthorized)
			state, err = routingstatus.Read(context.Background(), client, 7)
			if err != nil || !state.Known {
				t.Fatal("unauthenticated request invalidated status")
			}
			bad := makeCtx(tc.contentType, tc.bad, "test-secret")
			engine.ServeHTTP(context.Background(), bad)
			state, err = routingstatus.Read(context.Background(), client, 7)
			if err != nil || state.Known {
				t.Fatalf("bad report left applied known %#v %v", state, err)
			}
			if strings.Contains(string(bad.Response.Body()), "secret-token") {
				t.Fatal("invalid payload leaked in response")
			}
		})
	}
}
