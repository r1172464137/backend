package server

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	serverv1 "github.com/perfect-panel/server/api/server/v1"
	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"google.golang.org/protobuf/proto"
)

func TestV2JSONAndProtobufProjection(t *testing.T) {
	enabled := true
	a := "ob_11111111-1111-4111-8111-111111111111"
	b := "ob_22222222-2222-4222-8222-222222222222"
	response := &dto.QueryServerConfigResponse{Protocols: []dto.Protocol{{Type: "vless", Enable: true, Port: 443}}, Total: 1, DNS: []dto.NodeDNS{},
		Routing: &dto.NodeRoutingPolicy{Version: 2, Mode: "ordered-v2-inherited", Source: "global", SourceRevision: 3, DefaultOutboundID: "direct", Outbounds: []dto.NodeOutbound{
			{ID: a, Name: "A", Protocol: "socks", Address: "192.0.2.1", Port: 1080, Rules: []string{}, IPRules: []string{}, ProtocolRules: []string{}},
			{ID: b, Name: "B", Protocol: "socks", Address: "192.0.2.2", Port: 1080, ViaOutboundID: a, Rules: []string{}, IPRules: []string{}, ProtocolRules: []string{}},
		}, Rules: []dto.NodeRoutingRule{{ID: "rr_33333333-3333-4333-8333-333333333333", Enabled: &enabled, Kind: "domain", Values: []string{"full:example.com"}, UserIDs: []int64{42}, OutboundID: b}}, UserDefaults: []config.NodeUserDefault{{UserID: 42, OutboundID: b}}, DNSPolicy: &config.NodeDNSPolicy{QueryStrategy: "UseIPv4", Resolvers: []config.NodeDNSResolver{{ID: "dns_44444444-4444-4444-8444-444444444444", Protocol: "https", Address: "https://resolver.example/dns-query", BootstrapIP: "192.0.2.53", Domains: []string{}, OutboundID: b}}}},
		IdentitySnapshot: &dto.IdentitySnapshot{SnapshotID: strings.Repeat("a", 64), Protocols: []dto.ProtocolUserSnapshot{{Protocol: "vless", Users: []dto.ServerUser{{Id: 8, UUID: "00000000-0000-4000-8000-000000000001", AccountID: 42, SpeedLimit: 10, DeviceLimit: 2}}}}},
	}
	if err := validateRoutingDelivery(response, 7); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var j dto.QueryServerConfigResponse
	if err = json.Unmarshal(body, &j); err != nil {
		t.Fatal(err)
	}
	p, err := queryServerProtocolConfigResponseToProtobuf(response)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := proto.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var round serverv1.QueryServerProtocolConfigResponse
	if err = proto.Unmarshal(wire, &round); err != nil {
		t.Fatal(err)
	}
	r := round.Data.Routing
	if r.Version != 2 || r.Outbounds[1].ViaOutboundId != j.Routing.Outbounds[1].ViaOutboundID || !reflect.DeepEqual(r.Rules[0].UserIds, j.Routing.Rules[0].UserIDs) || r.UserDefaults[0].UserId != 42 || r.DnsPolicy.Resolvers[0].BootstrapIp != "192.0.2.53" {
		t.Fatalf("routing mismatch: %+v", r)
	}
	if s := round.Data.IdentitySnapshot; s.SnapshotId != j.IdentitySnapshot.SnapshotID || s.Protocols[0].Users[0].AccountId != 42 || s.Protocols[0].Users[0].DeviceLimit != 2 {
		t.Fatalf("identity mismatch: %+v", s)
	}
}
