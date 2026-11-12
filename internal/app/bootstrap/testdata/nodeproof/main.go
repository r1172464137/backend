// A read-only integration helper compiled in a private temporary module against
// the final ppnode checkout. It is never shipped as a backend binary.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/perfect-panel/ppanel-node/api/panel"
	"github.com/perfect-panel/ppanel-node/core"
	"github.com/perfect-panel/ppanel-node/core/inbound"
	"github.com/perfect-panel/ppanel-node/core/outbound"
	"github.com/perfect-panel/ppanel-node/core/streamguard"
)

type request struct {
	Kind     string          `json:"kind"`
	Outbound panel.Outbound  `json:"outbound"`
	Protocol panel.Protocol  `json:"protocol"`
	Policy   json.RawMessage `json:"policy"`
	URI      string          `json:"uri"`
}
type route struct {
	Kind string `json:"kind"`
	Tag  string `json:"tag"`
}
type result struct {
	OK    bool    `json:"ok"`
	Error string  `json:"error,omitempty"`
	Rules []route `json:"rules,omitempty"`
}

func main() {
	var q request
	if err := json.NewDecoder(os.Stdin).Decode(&q); err != nil {
		_ = json.NewEncoder(os.Stdout).Encode(result{Error: "invalid request"})
		return
	}
	response := run(q)
	_ = json.NewEncoder(os.Stdout).Encode(response)
}
func run(q request) (response result) {
	defer func() {
		if v := recover(); v != nil {
			response = result{Error: fmt.Sprintf("builder panic: %v", v)}
		}
	}()
	if q.Kind == "spider_uri" {
		if err := streamguard.ValidateURI("spider_x", q.URI, true); err != nil {
			return result{Error: err.Error()}
		}
		return result{OK: true}
	}
	identity := panel.RoutingIdentity{Mode: "inherited", Source: "global", SourceRevision: 1}
	p := &panel.RoutingPolicy{Version: panel.RoutingVersion, Outbounds: []panel.Outbound{}, Rules: []panel.RouteRule{}, DefaultOutboundID: "direct",
		UserDefaults: []panel.UserDefault{}, TargetIPPreference: "none", RoutingIdentity: identity,
		DNS: &panel.DNSPolicy{QueryStrategy: "UseIP", Resolvers: []panel.DNSResolverPolicy{{ID: "dns_00000000-0000-4000-8000-000000000001", Protocol: "system", Domains: []string{}, OutboundID: "direct"}}}}
	if q.Kind == "policy" {
		if err := json.Unmarshal(q.Policy, p); err != nil {
			return result{Error: err.Error()}
		}
		p.RoutingIdentity = identity
	}
	if q.Kind == "outbound" {
		if q.Outbound.ID == "" {
			q.Outbound.ID = "ob_11111111-1111-4111-8111-111111111111"
		}
		p.Outbounds = []panel.Outbound{q.Outbound}
	}
	protocols := []panel.Protocol{}
	if q.Kind == "inbound" {
		protocols = append(protocols, q.Protocol)
	}
	snapshot := &panel.IdentitySnapshot{SnapshotID: strings.Repeat("a", 64), Protocols: []panel.ProtocolIdentity{}}
	seen := map[string]bool{}
	for _, proto := range protocols {
		if !seen[proto.Type] {
			seen[proto.Type] = true
			snapshot.Protocols = append(snapshot.Protocols, panel.ProtocolIdentity{Protocol: proto.Type, Users: []panel.IdentityUser{}})
		}
	}
	server := &panel.ServerConfigResponse{Data: &panel.Data{Routing: p, Protocols: &protocols, Total: len(protocols), IdentitySnapshot: snapshot}}
	if err := core.ValidateServerConfig(server); err != nil {
		return result{Error: err.Error()}
	}
	if q.Kind == "inbound" {
		if !q.Protocol.Enable {
			return result{OK: true}
		}
		info := &panel.NodeInfo{Type: q.Protocol.Type, Protocol: &q.Protocol}
		if _, err := inbound.Build(info, "proof"); err != nil {
			return result{Error: err.Error()}
		}
		return result{OK: true}
	}
	built, err := outbound.Build(server, false, func(protocol string) string { return "proof-" + protocol })
	if err != nil {
		return result{Error: err.Error()}
	}
	resultRules := make([]route, 0, len(built.Router.Rule))
	for _, r := range built.Router.Rule {
		kind := "system"
		switch {
		case len(r.GetDomain()) > 0:
			kind = "domain"
		case len(r.GetIp()) > 0:
			kind = "ip"
		case len(r.GetProtocol()) > 0:
			kind = "protocol"
		}
		resultRules = append(resultRules, route{Kind: kind, Tag: r.GetTag()})
	}
	return result{OK: true, Rules: resultRules}
}
