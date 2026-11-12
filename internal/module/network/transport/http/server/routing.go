package server

import (
	"encoding/json"
	"fmt"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
)

const (
	routingVersionHeader = "X-PPanel-Routing-Version"
	// The node response includes the policy as well as DNS, protocols and an
	// envelope. This is a wire limit, not the policy's 1 MiB storage budget.
	maxRoutingNodeResponseBytes = 2 * 1024 * 1024
)

// validateRoutingDelivery rejects corrupt ordered candidates. serverID=0 is
// used by protobuf conversion when no request identity is available.
func validateRoutingDelivery(response *dto.QueryServerConfigResponse, serverID int64) error {
	if response == nil {
		return fmt.Errorf("nil node configuration")
	}
	if response.Total != int64(len(response.Protocols)) {
		return fmt.Errorf("protocol total does not match protocols")
	}
	if err := config.ValidateNodeInterval("pull_interval", response.PullInterval); err != nil {
		return err
	}
	if err := config.ValidateNodeInterval("push_interval", response.PushInterval); err != nil {
		return err
	}
	p := response.Routing
	if p == nil {
		return fmt.Errorf("ordered routing is required")
	}
	if p.SourceRevision <= 0 || p.LocalTransitionRevision < 0 {
		return fmt.Errorf("invalid routing revision")
	}
	switch p.Mode {
	case "local":
		if p.Source != "server" || p.SourceServerID <= 0 || (serverID > 0 && p.SourceServerID != serverID) || p.SourceRevision != p.LocalTransitionRevision {
			return fmt.Errorf("invalid local routing identity")
		}
	case "inherited":
		if p.Source != "global" || p.SourceServerID != 0 {
			return fmt.Errorf("invalid inherited routing identity")
		}
	default:
		return fmt.Errorf("unknown routing mode %q", p.Mode)
	}
	content := struct {
		Version            int                      `json:"version"`
		Outbounds          []dto.NodeOutbound       `json:"outbounds"`
		Rules              []dto.NodeRoutingRule    `json:"rules"`
		DefaultOutboundID  string                   `json:"default_outbound_id"`
		UserDefaults       []config.NodeUserDefault `json:"user_defaults"`
		DNS                *config.NodeDNSPolicy    `json:"dns"`
		TargetIPPreference string                   `json:"target_ip_preference"`
	}{p.Version, p.Outbounds, p.Rules, p.DefaultOutboundID, p.UserDefaults, p.DNS, p.TargetIPPreference}
	body, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("encode routing: %w", err)
	}
	if _, err := config.ParseNodeRouting(body); err != nil {
		return fmt.Errorf("invalid routing: %w", err)
	}
	return validateIdentitySnapshot(response)
}

// validateIdentitySnapshot checks the snapshot shape against the delivered
// protocols before anything is sent.
func validateIdentitySnapshot(response *dto.QueryServerConfigResponse) error {
	s := response.IdentitySnapshot
	if s == nil {
		return fmt.Errorf("identity snapshot is required")
	}
	if len(s.SnapshotID) != 64 {
		return fmt.Errorf("invalid snapshot ID")
	}
	delivered := map[string]bool{}
	for _, p := range response.Protocols {
		delivered[p.Type] = true
	}
	seen := map[string]bool{}
	for i, p := range s.Protocols {
		if !delivered[p.Protocol] || seen[p.Protocol] || (i > 0 && s.Protocols[i-1].Protocol >= p.Protocol) {
			return fmt.Errorf("identity snapshot protocols do not match the delivered protocols")
		}
		seen[p.Protocol] = true
		if p.Users == nil {
			return fmt.Errorf("identity snapshot users must be an array")
		}
	}
	if len(seen) != len(delivered) {
		return fmt.Errorf("identity snapshot is missing a delivered protocol")
	}
	return nil
}
