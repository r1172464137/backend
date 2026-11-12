// Package identity builds the per-candidate user identity snapshot and the
// candidate's snapshot_id (CONTRACT.md 1.4/1.5). The node delivery path and the
// administrator's read-only "desired" computation use the same functions, so
// they always agree on the snapshot ID.
package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/slicesx"
)

// Reader is the set of repositories read inside one transaction.
type Reader interface {
	Node() repository.NodeRepo
	Subscribe() repository.SubscribeRepo
	UserSubscription() repository.UserSubscriptionRepo
	User() repository.UserRepo
}

// projection is the part of each authorised subscription that changes the
// candidate without being sent to the node: renewals, plan changes and quota
// changes must produce a new snapshot ID.
type projection struct {
	Protocol     string `json:"protocol"`
	ID           int64  `json:"id"`
	PlanID       int64  `json:"plan_id"`
	ExpireTimeMs int64  `json:"expire_time_ms"`
	Traffic      int64  `json:"traffic"`
}

// Result is a built snapshot plus the hidden projection used for its ID.
type Result struct {
	Snapshot *dto.IdentitySnapshot
	// PendingActivation reports subscriptions that a real node pull would
	// activate. The read-only desired computation cannot predict them.
	PendingActivation bool
	projection        []projection
}

// Build reads the authorised identities for every delivered protocol.
// activate=true performs the existing pending-subscription activation (real
// node pulls only); activate=false never writes.
func Build(ctx context.Context, r Reader, serverID int64, protocols []dto.Protocol, now time.Time, activate bool) (*Result, error) {
	types := make([]string, 0, len(protocols))
	seenType := map[string]bool{}
	for _, p := range protocols {
		if seenType[p.Type] {
			continue
		}
		seenType[p.Type] = true
		types = append(types, p.Type)
	}
	sort.Strings(types)
	res := &Result{Snapshot: &dto.IdentitySnapshot{Protocols: make([]dto.ProtocolIdentity, 0, len(types))}}
	for _, protocol := range types {
		users, proj, pending, err := protocolUsers(ctx, r, serverID, protocol, now, activate)
		if err != nil {
			return nil, err
		}
		res.PendingActivation = res.PendingActivation || pending
		res.Snapshot.Protocols = append(res.Snapshot.Protocols, dto.ProtocolIdentity{Protocol: protocol, Users: users})
		res.projection = append(res.projection, proj...)
	}
	return res, nil
}

func protocolUsers(ctx context.Context, r Reader, serverID int64, protocol string, now time.Time, activate bool) ([]dto.IdentityUser, []projection, bool, error) {
	nodes, err := r.Node().ListNodes(ctx, &node.FilterNodeParams{ServerId: []int64{serverID}, Protocol: protocol})
	if err != nil {
		return nil, nil, false, err
	}
	var nodeIDs []int64
	var tags []string
	for _, n := range nodes {
		nodeIDs = append(nodeIDs, n.Id)
		if n.Tags != "" {
			tags = append(tags, strings.Split(n.Tags, ",")...)
		}
	}
	users := []dto.IdentityUser{}
	if len(nodeIDs) == 0 && len(tags) == 0 {
		return users, nil, false, nil
	}
	plans, err := r.Subscribe().FindByNodeScope(ctx, nodeIDs, tags)
	if err != nil {
		return nil, nil, false, err
	}
	if len(plans) == 0 {
		return users, nil, false, nil
	}
	planIDs := make([]int64, 0, len(plans))
	byID := make(map[int64]*subscribe.Subscribe, len(plans))
	for _, p := range plans {
		planIDs = append(planIDs, p.Id)
		byID[p.Id] = p
	}
	if activate {
		if err := r.UserSubscription().ActivatePendingSubscribesBySubscribeIds(ctx, planIDs); err != nil {
			return nil, nil, false, err
		}
	}
	subs, err := r.UserSubscription().FindUsersSubscribeBySubscribeIds(ctx, planIDs)
	if err != nil {
		return nil, nil, false, err
	}
	accountIDs := make([]int64, 0, len(subs))
	for _, s := range subs {
		accountIDs = append(accountIDs, s.UserId)
	}
	enabledIDs, err := r.User().FindEnabledUserIDs(ctx, slicesx.RemoveDuplicateElements(accountIDs...))
	if err != nil {
		return nil, nil, false, err
	}
	enabled := make(map[int64]bool, len(enabledIDs))
	for _, id := range enabledIDs {
		enabled[id] = true
	}
	pending := false
	var proj []projection
	byUUID := map[string]int64{}
	for _, s := range subs {
		plan := byID[s.SubscribeId]
		if plan == nil || !enabled[s.UserId] {
			continue
		}
		// Validity is checked here, not only by the status the expiry sweep
		// maintains, so an expired or exhausted subscription disappears at
		// once rather than at the next sweep.
		if !s.ExpireTime.IsZero() && s.ExpireTime.UnixMilli() != 0 && !s.ExpireTime.After(now) {
			continue
		}
		if s.Traffic > 0 && s.Upload+s.Download >= s.Traffic {
			continue
		}
		if s.Status == 0 {
			pending = true
		}
		uuid := strings.TrimSpace(s.UUID)
		if uuid == "" {
			return nil, nil, false, fmt.Errorf("subscription %d has no UUID", s.Id)
		}
		// UUIDs authenticate case-insensitively, so duplicates are detected
		// case-insensitively; the stored value is delivered unchanged.
		key := strings.ToLower(uuid)
		if other, dup := byUUID[key]; dup && other != s.UserId {
			return nil, nil, false, fmt.Errorf("protocol %s: one UUID belongs to two accounts", protocol)
		} else if dup {
			return nil, nil, false, fmt.Errorf("protocol %s: duplicate UUID", protocol)
		}
		byUUID[key] = s.UserId
		users = append(users, dto.IdentityUser{ID: s.Id, UUID: uuid, SpeedLimit: plan.SpeedLimit, DeviceLimit: plan.DeviceLimit, AccountID: s.UserId})
		expire := int64(0)
		if !s.ExpireTime.IsZero() {
			expire = s.ExpireTime.UTC().UnixMilli()
		}
		proj = append(proj, projection{Protocol: protocol, ID: s.Id, PlanID: plan.Id, ExpireTimeMs: expire, Traffic: s.Traffic})
	}
	sort.Slice(users, func(i, j int) bool { return users[i].ID < users[j].ID })
	for i := 1; i < len(users); i++ {
		if users[i].ID == users[i-1].ID {
			return nil, nil, false, fmt.Errorf("protocol %s: duplicate subscription %d", protocol, users[i].ID)
		}
	}
	return users, proj, pending, nil
}

// SnapshotID hashes the canonical candidate: routing content and delivery
// identity, the sorted delivered protocols, the sorted identities and the
// hidden projection. Interval/threshold settings and ETags are excluded.
func SnapshotID(routing *dto.NodeRoutingPolicy, protocols []dto.Protocol, res *Result) (string, error) {
	if routing == nil || res == nil || res.Snapshot == nil {
		return "", fmt.Errorf("incomplete candidate")
	}
	sortedProtocols := append([]dto.Protocol(nil), protocols...)
	sort.SliceStable(sortedProtocols, func(i, j int) bool {
		if sortedProtocols[i].Type != sortedProtocols[j].Type {
			return sortedProtocols[i].Type < sortedProtocols[j].Type
		}
		return sortedProtocols[i].Port < sortedProtocols[j].Port
	})
	proj := append([]projection(nil), res.projection...)
	sort.Slice(proj, func(i, j int) bool {
		if proj[i].Protocol != proj[j].Protocol {
			return proj[i].Protocol < proj[j].Protocol
		}
		return proj[i].ID < proj[j].ID
	})
	body, err := json.Marshal(struct {
		Routing    *dto.NodeRoutingPolicy `json:"routing"`
		Protocols  []dto.Protocol         `json:"protocols"`
		Identities []dto.ProtocolIdentity `json:"identities"`
		Projection []projection           `json:"projection"`
	}{routing, sortedProtocols, res.Snapshot.Protocols, proj})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}
