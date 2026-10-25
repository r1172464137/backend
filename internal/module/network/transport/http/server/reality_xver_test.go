package server

import (
	"encoding/json"
	"testing"

	serverv1 "github.com/perfect-panel/server/api/server/v1"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestRealityXverJSONAndProtobufDelivery(t *testing.T) {
	field := (&serverv1.ServerProtocol{}).ProtoReflect().Descriptor().Fields().ByName("reality_xver")
	if field == nil || field.Number() != 66 || field.Kind() != protoreflect.Uint32Kind {
		t.Fatalf("wire contract = %v", field)
	}
	for _, xver := range []int{0, 1, 2} {
		response := &dto.QueryServerConfigResponse{Protocols: []dto.Protocol{{Type: "vless", Security: "reality", RealityXver: xver, AcceptProxyProtocol: true}}}
		body, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		var decoded dto.QueryServerConfigResponse
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.Protocols[0].RealityXver != xver || !decoded.Protocols[0].AcceptProxyProtocol {
			t.Fatalf("JSON = %s", body)
		}
		message, err := queryServerProtocolConfigResponseToProtobuf(response)
		if err != nil {
			t.Fatal(err)
		}
		wire, err := proto.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		var received serverv1.QueryServerProtocolConfigResponse
		if err := proto.Unmarshal(wire, &received); err != nil {
			t.Fatal(err)
		}
		if received.Data.Protocols[0].RealityXver != uint32(xver) || !received.Data.Protocols[0].AcceptProxyProtocol {
			t.Fatalf("protobuf = %v", &received)
		}
	}
	for _, xver := range []int{-1, 3} {
		if _, err := serverProtocolToProtobuf(dto.Protocol{RealityXver: xver}); err == nil {
			t.Fatalf("accepted invalid xver %d", xver)
		}
	}
	for _, raw := range []string{`1.5`, `"1"`, `true`, `18446744073709551616`} {
		var p dto.Protocol
		if err := json.Unmarshal([]byte(`{"reality_xver":`+raw+`}`), &p); err == nil {
			t.Fatalf("DTO accepted %s", raw)
		}
	}
}
