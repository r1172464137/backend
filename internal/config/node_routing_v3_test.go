package config

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func v3Outbound(n int, protocol string) NodeOutbound {
	return NodeOutbound{ID: routingID("ob_", n), Name: protocol + "-" + string(rune('a'+n)), Protocol: protocol, Address: "192.0.2.1", Port: 1080,
		Rules: []string{}, IPRules: []string{}, ProtocolRules: []string{}}
}

func TestAccountScopeAndDefaults(t *testing.T) {
	p := asV3(basicRouting())
	p.Rules[0].UserIDs = []int64{43, 42, 43}
	p.UserDefaults = []NodeUserDefault{{UserID: 9, OutboundID: RoutingBlock}, {UserID: 3, OutboundID: p.Outbounds[0].ID}}
	got, err := NormalizeNodeRouting(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Rules[0].UserIDs, []int64{42, 43}) {
		t.Fatalf("user_ids not sorted/deduplicated: %v", got.Rules[0].UserIDs)
	}
	if got.UserDefaults[0].UserID != 3 || got.UserDefaults[1].UserID != 9 {
		t.Fatalf("user_defaults not sorted: %v", got.UserDefaults)
	}
	for name, edit := range map[string]func(*NodeRouting){
		"null user_ids":      func(p *NodeRouting) { p.Rules[0].UserIDs = nil },
		"zero account":       func(p *NodeRouting) { p.Rules[0].UserIDs = []int64{0} },
		"negative account":   func(p *NodeRouting) { p.Rules[0].UserIDs = []int64{-1} },
		"null user_defaults": func(p *NodeRouting) { p.UserDefaults = nil },
		"duplicate default":  func(p *NodeRouting) { p.UserDefaults = []NodeUserDefault{{1, "direct"}, {1, "block"}} },
		"default unknown outbound": func(p *NodeRouting) {
			p.UserDefaults = []NodeUserDefault{{1, "ob_ffffffff-ffff-4fff-8fff-ffffffffffff"}}
		},
		"bad preference":   func(p *NodeRouting) { p.TargetIPPreference = "ipv4_only" },
		"empty preference": func(p *NodeRouting) { p.TargetIPPreference = "" },
		"null dns":         func(p *NodeRouting) { p.DNS = nil },
		"format 1":         func(p *NodeRouting) { p.Version = 1 },
		"format 2":         func(p *NodeRouting) { p.Version = 2 },
	} {
		t.Run(name, func(t *testing.T) {
			p := asV3(basicRouting())
			edit(p)
			if _, err := NormalizeNodeRouting(p); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	big := make([]int64, MaxRoutingUserIDsPerRule+1)
	for i := range big {
		big[i] = int64(i + 1)
	}
	p = asV3(basicRouting())
	p.Rules[0].UserIDs = big
	if _, err := NormalizeNodeRouting(p); err == nil {
		t.Fatal("too many accounts in one rule accepted")
	}
}

func TestOutboundChains(t *testing.T) {
	newPolicy := func() *NodeRouting {
		p := asV3(basicRouting())
		p.Outbounds = []NodeOutbound{v3Outbound(1, "socks"), v3Outbound(2, "shadowsocks"), v3Outbound(3, "vless")}
		p.Outbounds[1].Password, p.Outbounds[1].Cipher = "pw", "aes-128-gcm"
		p.Outbounds[2].UUID = "00000000-0000-4000-8000-000000000001"
		return p
	}
	p := newPolicy()
	p.Outbounds[1].ViaOutboundID = p.Outbounds[0].ID // ss via socks
	p.Outbounds[2].ViaOutboundID = p.Outbounds[1].ID // vless via ss via socks
	if _, err := NormalizeNodeRouting(p); err != nil {
		t.Fatalf("valid chain rejected: %v", err)
	}
	p = newPolicy()
	p.Outbounds[0].StreamSettings = `{"sockopt":{"dialerProxy":"routing_` + p.Outbounds[1].ID + `"}}`
	if _, err := NormalizeNodeRouting(p); err != nil {
		t.Fatalf("raw dialerProxy to a policy outbound rejected: %v", err)
	}
	for name, edit := range map[string]func(*NodeRouting){
		"self": func(p *NodeRouting) { p.Outbounds[0].ViaOutboundID = p.Outbounds[0].ID },
		"cycle": func(p *NodeRouting) {
			p.Outbounds[0].ViaOutboundID, p.Outbounds[1].ViaOutboundID = p.Outbounds[1].ID, p.Outbounds[0].ID
		},
		"long cycle": func(p *NodeRouting) {
			p.Outbounds[0].ViaOutboundID, p.Outbounds[1].ViaOutboundID, p.Outbounds[2].ViaOutboundID = p.Outbounds[1].ID, p.Outbounds[2].ID, p.Outbounds[0].ID
		},
		"dangling": func(p *NodeRouting) { p.Outbounds[0].ViaOutboundID = "ob_ffffffff-ffff-4fff-8fff-ffffffffffff" },
		"direct":   func(p *NodeRouting) { p.Outbounds[0].ViaOutboundID = RoutingDirect },
		"block":    func(p *NodeRouting) { p.Outbounds[0].ViaOutboundID = RoutingBlock },
		"raw and via": func(p *NodeRouting) {
			p.Outbounds[0].ViaOutboundID = p.Outbounds[1].ID
			p.Outbounds[0].StreamSettings = `{"sockopt":{"dialerProxy":"routing_` + p.Outbounds[2].ID + `"}}`
		},
		"raw foreign tag": func(p *NodeRouting) { p.Outbounds[0].StreamSettings = `{"sockopt":{"dialerProxy":"some-other-tag"}}` },
		"raw case cycle": func(p *NodeRouting) {
			p.Outbounds[0].StreamSettings = `{"SockOpt":{"DialerProxy":"routing_` + p.Outbounds[0].ID + `"}}`
		},
		"freedom with via": func(p *NodeRouting) {
			p.Outbounds[0].Protocol = "freedom"
			p.Outbounds[0].ViaOutboundID = p.Outbounds[1].ID
		},
		"blackhole via": func(p *NodeRouting) {
			p.Outbounds[0].Protocol = "blackhole"
			p.Outbounds[0].ViaOutboundID = p.Outbounds[1].ID
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := newPolicy()
			edit(p)
			if _, err := NormalizeNodeRouting(p); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func TestDNSPolicyValidation(t *testing.T) {
	custom := func(r ...NodeDNSResolver) *NodeDNSPolicy {
		return &NodeDNSPolicy{QueryStrategy: "UseIP", Resolvers: r}
	}
	sys := NodeDNSResolver{ID: routingID("dns_", 9), Protocol: "system", Domains: []string{}, OutboundID: RoutingDirect}
	udp := NodeDNSResolver{ID: routingID("dns_", 1), Protocol: "udp", Address: "192.0.2.53:53", Domains: []string{"geosite:cn"}, OutboundID: RoutingDirect}
	doh := NodeDNSResolver{ID: routingID("dns_", 2), Protocol: "https", Address: "https://dns.example/dns-query", Domains: []string{"example.org"}, OutboundID: RoutingDirect, BootstrapIP: "192.0.2.54"}
	valid := []*NodeDNSPolicy{
		DefaultDNSPolicy("t"),
		custom(udp, sys),
		custom(udp, doh, sys),
		custom(udp, NodeDNSResolver{ID: routingID("dns_", 3), Protocol: "tcp", Address: "dns.example", Domains: []string{}, OutboundID: RoutingDirect, BootstrapIP: "192.0.2.55"}),
	}
	// A direct resolver named by host would need system DNS to find itself.
	noBootstrap := asV3(basicRouting())
	noBootstrap.DNS = custom(udp, NodeDNSResolver{ID: routingID("dns_", 3), Protocol: "tcp", Address: "dns.example", Domains: []string{}, OutboundID: RoutingDirect})
	if _, err := NormalizeNodeRouting(noBootstrap); err == nil {
		t.Error("direct host-name resolver without bootstrap_ip accepted")
	}
	for i, d := range valid {
		p := asV3(basicRouting())
		p.DNS = d
		if _, err := NormalizeNodeRouting(p); err != nil {
			t.Errorf("valid DNS %d rejected: %v", i, err)
		}
	}
	withOutbound := asV3(basicRouting())
	via := udp
	via.OutboundID = withOutbound.Outbounds[0].ID
	withOutbound.DNS = custom(via, sys)
	if _, err := NormalizeNodeRouting(withOutbound); err != nil {
		t.Errorf("DNS through a custom outbound rejected: %v", err)
	}
	bad := map[string]*NodeDNSPolicy{
		"no resolvers":          {QueryStrategy: "UseIP", Resolvers: []NodeDNSResolver{}},
		"null resolvers":        {QueryStrategy: "UseIP"},
		"UseIPv4v6":             {QueryStrategy: "UseIPv4v6", Resolvers: []NodeDNSResolver{sys}},
		"default not last":      custom(sys, udp),
		"system not last":       custom(sys, sys),
		"middle without domain": custom(udp, func() NodeDNSResolver { r := udp; r.ID = routingID("dns_", 5); r.Domains = []string{}; return r }(), sys),
		"block outbound":        custom(func() NodeDNSResolver { r := udp; r.OutboundID = RoutingBlock; return r }(), sys),
		"unknown outbound":      custom(func() NodeDNSResolver { r := udp; r.OutboundID = "ob_ffffffff-ffff-4fff-8fff-ffffffffffff"; return r }(), sys),
		"http url":              custom(func() NodeDNSResolver { r := doh; r.Address = "http://dns.example/q"; return r }(), sys),
		"userinfo":              custom(func() NodeDNSResolver { r := doh; r.Address = "https://u:p@dns.example/q"; return r }(), sys),
		"fragment":              custom(func() NodeDNSResolver { r := doh; r.Address = "https://dns.example/q#x"; return r }(), sys),
		"+local":                custom(func() NodeDNSResolver { r := udp; r.Address = "localhost+local"; return r }(), sys),
		"bootstrap on IP":       custom(func() NodeDNSResolver { r := udp; r.BootstrapIP = "192.0.2.1"; return r }(), sys),
		"bootstrap not IP":      custom(func() NodeDNSResolver { r := doh; r.BootstrapIP = "dns.example"; return r }(), sys),
		"conflicting bootstrap": custom(doh, func() NodeDNSResolver {
			r := doh
			r.ID = routingID("dns_", 6)
			r.Domains = []string{"other.example"}
			r.BootstrapIP = "192.0.2.99"
			return r
		}(), sys),
		"partial bootstrap": custom(doh, func() NodeDNSResolver {
			r := doh
			r.ID = routingID("dns_", 7)
			r.Domains = []string{"other.example"}
			r.BootstrapIP = ""
			return r
		}(), sys),
		"duplicate id":        custom(udp, func() NodeDNSResolver { r := sys; r.ID = udp.ID; return r }()),
		"system with address": custom(func() NodeDNSResolver { r := sys; r.Address = "1.1.1.1"; return r }()),
		"system via outbound": custom(func() NodeDNSResolver { r := sys; r.OutboundID = RoutingBlock; return r }()),
		"null domains":        custom(func() NodeDNSResolver { r := sys; r.Domains = nil; return r }()),
		"bad domain":          custom(func() NodeDNSResolver { r := udp; r.Domains = []string{"regexp:("}; return r }(), sys),
		"unknown protocol":    custom(func() NodeDNSResolver { r := udp; r.Protocol = "tls"; return r }(), sys),
	}
	bad["last has domains"] = custom(udp, func() NodeDNSResolver { r := udp; r.ID = routingID("dns_", 8); return r }())
	for name, d := range bad {
		t.Run(name, func(t *testing.T) {
			p := asV3(basicRouting())
			p.DNS = d
			if _, err := NormalizeNodeRouting(p); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func TestFormat3JSONPresenceAndUnknownFields(t *testing.T) {
	good, err := json.Marshal(DefaultNodeRouting("t"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseNodeRouting(good); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(string) string{
		"missing dns": func(s string) string { return strings.Replace(s, `,"dns":`, `,"x_dns":`, 1) },
		"null dns": func(s string) string {
			return strings.Replace(s, s[strings.Index(s, `"dns":`):strings.Index(s, `,"target_ip_preference"`)], `"dns":null`, 1)
		},
		"missing user_defaults": func(s string) string { return strings.Replace(s, `"user_defaults":[],`, ``, 1) },
		"missing preference":    func(s string) string { return strings.Replace(s, `,"target_ip_preference":"none"`, ``, 1) },
		"unknown dns field": func(s string) string {
			return strings.Replace(s, `"query_strategy"`, `"cache":true,"query_strategy"`, 1)
		},
		"unknown resolver field": func(s string) string {
			return strings.Replace(s, `"protocol":"system"`, `"protocol":"system","skip_fallback":true`, 1)
		},
		"unknown user default key": func(s string) string {
			return strings.Replace(s, `"user_defaults":[]`, `"user_defaults":[{"user_id":1,"outbound_id":"direct","note":""}]`, 1)
		},
	} {
		if _, err := ParseNodeRouting([]byte(mutate(string(good)))); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestNewAccountIDs(t *testing.T) {
	prev := asV3(basicRouting())
	prev.Rules[0].UserIDs = []int64{1, 2}
	next := asV3(basicRouting())
	next.Rules[0].UserIDs = []int64{2, 3}
	next.UserDefaults = []NodeUserDefault{{UserID: 1, OutboundID: "direct"}, {UserID: 5, OutboundID: "direct"}}
	if got := NewAccountIDs(next, prev); !reflect.DeepEqual(got, []int64{3, 5}) {
		t.Fatalf("new accounts = %v, want [3 5]", got)
	}
	if got := NewAccountIDs(next, nil); !reflect.DeepEqual(got, []int64{1, 2, 3, 5}) {
		t.Fatalf("new accounts without previous = %v", got)
	}
}

func TestOutboundCapabilityMatchesNode(t *testing.T) {
	for name, o := range map[string]NodeOutbound{
		"tuic":            {Protocol: "tuic"},
		"tuic transport":  {Protocol: "socks", Transport: "tuic"},
		"unknown":         {Protocol: "naive"},
		"uot":             {Protocol: "shadowsocks", UoT: true},
		"raw uot":         {Protocol: "shadowsocks", Settings: `{"servers":[{"uot":true}]}`},
		"plugin":          {Protocol: "shadowsocks", Plugin: "obfs"},
		"hysteria on tcp": {Protocol: "hysteria2", Transport: "tcp"},
		"tcp hysteria":    {Protocol: "socks", Transport: "hysteria"},
		"bad path":        {Protocol: "vless", Transport: "ws", Path: "/%"},
	} {
		o.ID, o.Name = routingID("ob_", 1), "x"
		p := asV3(basicRouting())
		p.Outbounds = []NodeOutbound{o}
		if _, err := NormalizeNodeRouting(p); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	for _, protocol := range []string{"socks", "http", "shadowsocks", "vmess", "vless", "trojan", "freedom", "direct", "blackhole", "anytls"} {
		p := asV3(basicRouting())
		p.Outbounds = []NodeOutbound{v3Outbound(1, protocol)}
		if _, err := NormalizeNodeRouting(p); err != nil {
			t.Errorf("%s rejected: %v", protocol, err)
		}
	}
}
