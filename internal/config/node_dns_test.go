package config

import (
	"reflect"
	"testing"
)

func TestNodeDNSNormalizeAndReject(t *testing.T) {
	got, err := NormalizeNodeDNS([]NodeDNS{{Proto: " UDP ", Address: "DNS.Example.COM.:8053", Domains: []string{"Example.COM.", "suffix:example.net", "Example.COM."}}, {Address: "[2001:db8::53]"}, {Address: "LOCALHOST:5300"}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Proto != "udp" || got[0].Address != "dns.example.com.:8053" || !reflect.DeepEqual(got[0].Domains, []string{"full:Example.COM", "domain:example.net"}) || got[1].Address != "2001:db8::53" || got[2].Address != "localhost.:5300" {
		t.Fatalf("normal = %#v", got)
	}
	for _, item := range []NodeDNS{{Proto: "tcp", Address: "1.1.1.1"}, {Proto: "udp", Address: "1.1.1.1", ServerName: "x"}, {Address: "https://dns.example/query"}, {Address: "user@dns.example"}, {Address: "dns.example:0"}, {Address: "dns.example:65536"}, {Address: "dns.example:"}, {Address: "[2001:db8::1"}, {Address: "1.2.3.999"}, {Address: " 1.1.1.1"}, {Address: "1.1.1.1", Domains: []string{"regexp:("}}} {
		if _, err := NormalizeNodeDNS([]NodeDNS{item}); err == nil {
			t.Errorf("accepted %#v", item)
		}
	}
}
