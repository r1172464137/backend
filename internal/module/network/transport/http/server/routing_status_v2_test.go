package server

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	serverv1 "github.com/perfect-panel/server/api/server/v1"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
)

func TestV2StatusJSONProtobufSnapshotIdentity(t *testing.T) {
	id := &serverv1.RoutingIdentity{Mode: "ordered-v2-inherited", Source: "global", SourceRevision: 4, SnapshotId: strings.Repeat("a", 64)}
	msg := &serverv1.PushServerStatusRequest{RoutingVersion: 2, RoutingSequence: 3, InstanceId: "11111111-1111-4111-8111-111111111111", RoutingApplied: id, RoutingPending: id, RoutingError: &serverv1.RoutingError{Identity: id, Stage: "bind"}}
	var pb dto.ServerPushStatusRequest
	if err := bindServerStatusRequest(newProtobufContext(t, msg), &pb); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(map[string]any{"routing_version": pb.RoutingVersion, "routing_sequence": pb.RoutingSequence, "instance_id": pb.InstanceID, "routing_applied": pb.RoutingApplied, "routing_pending": pb.RoutingPending, "routing_error": pb.RoutingError})
	if err != nil {
		t.Fatal(err)
	}
	c := app.NewContext(0)
	c.Request.SetBody(encoded)
	c.Request.Header.SetContentTypeBytes([]byte("application/json"))
	var j dto.ServerPushStatusRequest
	if err := bindServerStatusRequest(c, &j); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(j, pb) || j.RoutingApplied.SnapshotID != id.SnapshotId || j.RoutingError.Identity.SnapshotID != id.SnapshotId {
		t.Fatalf("JSON %+v vs proto %+v", j, pb)
	}
}
