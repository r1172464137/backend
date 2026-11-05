package server

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"uuid"

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
	case "ordered-v1-local", "ordered-v2-local":
		if p.Source != "server" || p.SourceServerID <= 0 || (serverID > 0 && p.SourceServerID != serverID) || p.SourceRevision != p.LocalTransitionRevision {
			return fmt.Errorf("invalid local routing identity")
		}
	case "ordered-v1-inherited", "ordered-v2-inherited":
		if p.Source != "global" || p.SourceServerID != 0 {
			return fmt.Errorf("invalid inherited routing identity")
		}
	default:
		return fmt.Errorf("unknown routing mode %q", p.Mode)
	}
	if p.Mode != fmt.Sprintf("ordered-v%d-%s", p.Version, map[string]string{"global": "inherited", "server": "local"}[p.Source]) {
		return fmt.Errorf("routing version and mode mismatch")
	}
	var defaults *[]config.NodeUserDefault
	if p.Version == config.RoutingVersion1 && response.IdentitySnapshot != nil {
		return fmt.Errorf("version 1 cannot contain identity snapshot")
	}
	if p.Version == config.RoutingVersion2 {
		if response.IdentitySnapshot == nil || len(response.IdentitySnapshot.SnapshotID) != 64 {
			return fmt.Errorf("v2 identity snapshot is required")
		}
		snapshot := response.IdentitySnapshot
		if _, err := hex.DecodeString(snapshot.SnapshotID); err != nil || snapshot.SnapshotID != strings.ToLower(snapshot.SnapshotID) {
			return fmt.Errorf("invalid snapshot ID")
		}
		expected := map[string]bool{}
		for _, v := range response.Protocols {
			expected[v.Type] = true
		}
		seen := map[string]bool{}
		for _, v := range snapshot.Protocols {
			if !expected[v.Protocol] || seen[v.Protocol] || v.Users == nil {
				return fmt.Errorf("invalid snapshot protocol")
			}
			seen[v.Protocol] = true
			ids := map[int64]bool{}
			uuids := map[string]bool{}
			for _, u := range v.Users {
				parsed, err := uuid.Parse(u.UUID)
				if err != nil || u.Id <= 0 || u.AccountID <= 0 || u.SpeedLimit < 0 || u.DeviceLimit < 0 || ids[u.Id] || uuids[parsed.String()] {
					return fmt.Errorf("invalid snapshot user")
				}
				ids[u.Id] = true
				uuids[parsed.String()] = true
			}
		}
		if len(seen) != len(expected) {
			return fmt.Errorf("snapshot missing protocol")
		}
		defaults = &p.UserDefaults
	}
	content := struct {
		Version           int                       `json:"version"`
		Outbounds         []dto.NodeOutbound        `json:"outbounds"`
		Rules             []dto.NodeRoutingRule     `json:"rules"`
		DefaultOutboundID string                    `json:"default_outbound_id"`
		UserDefaults      *[]config.NodeUserDefault `json:"user_defaults,omitempty"`
		DNSPolicy         *config.NodeDNSPolicy     `json:"dns_policy,omitempty"`
	}{p.Version, p.Outbounds, p.Rules, p.DefaultOutboundID, defaults, p.DNSPolicy}
	body, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("encode routing: %w", err)
	}
	if _, err := config.ParseNodeRouting(body); err != nil {
		return fmt.Errorf("invalid routing: %w", err)
	}
	return nil
}
