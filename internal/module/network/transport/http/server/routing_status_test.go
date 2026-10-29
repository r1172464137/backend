package server

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	serverv1 "github.com/perfect-panel/server/api/server/v1"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestNodeDeliveryProtocolTotalContract(t *testing.T) {
	for _, protocols := range [][]dto.Protocol{nil, {}, {{Type: "vless", Enable: true}}} {
		response := &dto.QueryServerConfigResponse{Protocols: protocols, Total: int64(len(protocols)), Routing: routeTestPolicy()}
		body, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded["protocols"] == nil || int(decoded["total"].(float64)) != len(protocols) {
			t.Fatalf("bad JSON delivery %s", body)
		}
		message, err := queryServerProtocolConfigResponseToProtobuf(response)
		if err != nil || message.Data.Total != int64(len(message.Data.Protocols)) {
			t.Fatalf("bad protobuf delivery %#v %v", message, err)
		}
		response.Total++
		if _, err := json.Marshal(response); err == nil {
			t.Fatal("JSON accepted wrong total")
		}
		if _, err := queryServerProtocolConfigResponseToProtobuf(response); err == nil {
			t.Fatal("protobuf accepted wrong total")
		}
	}
}

func TestRoutingStatusWireContract(t *testing.T) {
	fields := (&serverv1.PushServerStatusRequest{}).ProtoReflect().Descriptor().Fields()
	for name, tag := range map[string]int{"routing_version": 5, "routing_applied": 6, "routing_pending": 7, "routing_error": 8, "instance_id": 9, "routing_sequence": 10} {
		f := fields.ByName(protoreflect.Name(name))
		if f == nil || int(f.Number()) != tag {
			t.Fatalf("%s tag = %v", name, f)
		}
	}
	id := &serverv1.RoutingIdentity{Mode: "ordered-v1-inherited", Source: "global", SourceRevision: 4, LocalTransitionRevision: 2}
	msg := &serverv1.PushServerStatusRequest{UpdatedAt: 123, RoutingVersion: 1, RoutingSequence: 9007199254740993, InstanceId: "11111111-1111-4111-8111-111111111111", RoutingApplied: id, RoutingPending: id, RoutingError: &serverv1.RoutingError{Identity: id, Stage: "bind", Message: "candidate failed during bind"}}
	protoCtx := newProtobufContext(t, msg)
	var p dto.ServerPushStatusRequest
	if err := bindServerStatusRequest(protoCtx, &p); err != nil {
		t.Fatal(err)
	}
	body := `{"updated_at":123,"routing_version":1,"routing_sequence":9007199254740993,"instance_id":"11111111-1111-4111-8111-111111111111","routing_applied":{"mode":"ordered-v1-inherited","source":"global","source_server_id":0,"source_revision":4,"local_transition_revision":2},"routing_pending":{"mode":"ordered-v1-inherited","source":"global","source_server_id":0,"source_revision":4,"local_transition_revision":2},"routing_error":{"identity":{"mode":"ordered-v1-inherited","source":"global","source_server_id":0,"source_revision":4,"local_transition_revision":2},"stage":"bind","message":"candidate failed during bind"}}`
	jsonCtx := app.NewContext(0)
	jsonCtx.Request.SetBodyString(body)
	jsonCtx.Request.Header.SetContentTypeBytes([]byte("application/json"))
	var j dto.ServerPushStatusRequest
	if err := bindServerStatusRequest(jsonCtx, &j); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(j, p) {
		data, _ := json.Marshal(j)
		t.Fatalf("JSON %s vs protobuf %#v", data, p)
	}
}
