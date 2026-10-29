package serverv1

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// This optional cross-repository read test never writes to the node checkout.
// Set PPANEL_NODE_PROTO and PPANEL_PROTOC_INCLUDE to verify the final owner
// schema in an isolated local test run.
func TestNodeWireDescriptorParity(t *testing.T) {
	nodeFile := os.Getenv("PPANEL_NODE_PROTO")
	include := os.Getenv("PPANEL_PROTOC_INCLUDE")
	if nodeFile == "" || include == "" {
		t.Skip("set PPANEL_NODE_PROTO and PPANEL_PROTOC_INCLUDE for final node wire parity")
	}
	protoc, err := exec.LookPath("protoc")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "schema.pb")
	cmd := exec.Command(protoc, "-I", root, "-I", filepath.Dir(nodeFile), "-I", include, "--include_imports", "--descriptor_set_out="+out, "api/server/v1/server.proto", filepath.Base(nodeFile))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("protoc: %v: %s", err, output)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(body, &set); err != nil {
		t.Fatal(err)
	}
	var panel, node *descriptorpb.FileDescriptorProto
	for _, f := range set.File {
		switch f.GetName() {
		case "api/server/v1/server.proto":
			panel = f
		case filepath.Base(nodeFile):
			node = f
		}
	}
	if panel == nil || node == nil {
		t.Fatal("expected two protobuf schemas")
	}
	messages := func(f *descriptorpb.FileDescriptorProto) map[string]*descriptorpb.DescriptorProto {
		m := map[string]*descriptorpb.DescriptorProto{}
		for _, v := range f.MessageType {
			m[v.GetName()] = v
		}
		return m
	}
	a, b := messages(panel), messages(node)
	for _, name := range []string{"PushServerStatusRequest", "RoutingIdentity", "RoutingError", "Outbound", "RouteRule", "RoutingPolicy", "ServerProtocol", "QueryServerProtocolConfigData", "QueryServerProtocolConfigResponse"} {
		left, right := a[name], b[name]
		if left == nil || right == nil {
			t.Fatalf("%s missing", name)
		}
		if len(left.Field) != len(right.Field) {
			t.Fatalf("%s field count %d != %d", name, len(left.Field), len(right.Field))
		}
		for i, f := range left.Field {
			g := right.Field[i]
			typeName := func(v string) string { parts := strings.Split(v, "."); return parts[len(parts)-1] }
			if f.GetName() != g.GetName() || f.GetNumber() != g.GetNumber() || f.GetType() != g.GetType() || f.GetLabel() != g.GetLabel() || f.GetProto3Optional() != g.GetProto3Optional() || typeName(f.GetTypeName()) != typeName(g.GetTypeName()) {
				t.Errorf("%s field mismatch: %v vs %v", name, f, g)
			}
		}
	}
	enums := func(f *descriptorpb.FileDescriptorProto) map[string]*descriptorpb.EnumDescriptorProto {
		m := map[string]*descriptorpb.EnumDescriptorProto{}
		for _, v := range f.EnumType {
			m[v.GetName()] = v
		}
		return m
	}
	left, right := enums(panel)["RouteKind"], enums(node)["RouteKind"]
	if left == nil || right == nil || !reflect.DeepEqual(left.GetValue(), right.GetValue()) {
		t.Fatal("RouteKind enum differs")
	}
}
