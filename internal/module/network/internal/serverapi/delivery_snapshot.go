package serverapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"uuid"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/network/internal/nodeconfig"
	"github.com/perfect-panel/server/internal/module/network/internal/protocolmap"
	"github.com/perfect-panel/server/internal/module/network/internal/routingstatus"
	"github.com/perfect-panel/server/internal/module/platform"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
)

type QueryServerProtocolConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newQueryServerProtocolConfigLogic(ctx context.Context, deps Deps) *QueryServerProtocolConfigLogic {
	return &QueryServerProtocolConfigLogic{Logger: logger.WithContext(ctx), ctx: ctx, deps: deps}
}

var errPendingSnapshotSubscription = errors.New("pending subscription appeared during delivery snapshot")

// nodesForProtocol uses the same scoped node selector as the v1 user list.
func nodesForProtocol(ctx context.Context, nodes NodeLister, serverID int64, protocol string) ([]int64, []string, error) {
	matches, err := nodes.ListNodes(ctx, &node.FilterNodeParams{ServerId: []int64{serverID}, Protocol: protocol})
	if err != nil {
		return nil, nil, err
	}
	ids := make([]int64, 0, len(matches))
	var tags []string
	for _, n := range matches {
		ids = append(ids, n.Id)
		if n.Tags != "" {
			tags = append(tags, strings.Split(n.Tags, ",")...)
		}
	}
	return ids, tags, nil
}

func (l *QueryServerProtocolConfigLogic) v2Delivery(req *dto.QueryServerConfigRequest, observed *dto.QueryServerConfigResponse) (*dto.QueryServerConfigResponse, error) {
	filter, err := routingstatus.CanonicalFilter(req.Protocols)
	if err != nil {
		return nil, err
	}
	// A real pull activates legacy pending rows before the read-only snapshot.
	activator, ok := l.deps.Subscriptions.(interface {
		ActivatePendingByNodeScope(context.Context, []int64, []string) error
	})
	if len(observed.Protocols) > 0 && !ok {
		return nil, fmt.Errorf("subscription activation unavailable")
	}
	for _, protocol := range observed.Protocols {
		ids, tags, err := nodesForProtocol(l.ctx, l.deps.Nodes, req.ServerID, protocol.Type)
		if err != nil {
			return nil, err
		}
		if err := activator.ActivatePendingByNodeScope(l.ctx, ids, tags); err != nil {
			return nil, err
		}
	}
	result, err := l.v2Snapshot(req, filter)
	if err == nil && l.deps.Redis != nil {
		if e := routingstatus.RememberFilter(l.ctx, l.deps.Redis, req.ServerID, result.IdentitySnapshot.SnapshotID, filter); e != nil {
			l.Errorw("cannot remember delivery filter", logger.Field("error", e.Error()))
		}
	}
	return result, err
}

// v2Snapshot only reads authoritative state; pending subscriptions make the
// candidate unknown until a real node pull performs the existing activation.
func (l *QueryServerProtocolConfigLogic) v2Snapshot(req *dto.QueryServerConfigRequest, filter []string) (*dto.QueryServerConfigResponse, error) {
	transactor, ok := l.deps.Store.(repository.DeliverySnapshotTransactor)
	if !ok {
		return nil, fmt.Errorf("consistent delivery snapshot unavailable")
	}
	var result *dto.QueryServerConfigResponse
	err := transactor.InDeliverySnapshot(l.ctx, func(store repository.DeliverySnapshot) error {
		server, err := store.Node().FindOneServer(l.ctx, req.ServerID)
		if err != nil {
			return err
		}
		protocols, err := snapshotProtocols(server, req.Protocols)
		if err != nil {
			return err
		}
		global, err := platform.ReadNodeConfigSnapshotFromRepo(l.ctx, store.System())
		if err != nil {
			return err
		}
		override, err := store.Node().FindServerRoutingOverride(l.ctx, req.ServerID)
		if err != nil {
			return err
		}
		values := nodeconfig.GlobalValues(global)
		if err := nodeconfig.ApplyOverride(&values, override); err != nil {
			return err
		}
		routing, err := nodeconfig.DeliveryPolicy(global, override, req.ServerID)
		if err != nil {
			return err
		}
		if routing.Version != config.RoutingVersion2 {
			return fmt.Errorf("routing changed while taking v2 snapshot")
		}
		result = &dto.QueryServerConfigResponse{TrafficReportThreshold: global.TrafficReportThreshold, PushInterval: global.NodePushInterval, PullInterval: global.NodePullInterval, IPStrategy: values.IPStrategy, DNS: values.DNS, Protocols: protocols, Total: int64(len(protocols)), Routing: routing, IdentitySnapshot: &dto.IdentitySnapshot{Protocols: []dto.ProtocolUserSnapshot{}}}
		entitlements, err := snapshotIdentity(l.ctx, store, req.ServerID, protocols, result.IdentitySnapshot)
		if err != nil {
			return err
		}
		sort.Slice(result.IdentitySnapshot.Protocols, func(i, j int) bool {
			return result.IdentitySnapshot.Protocols[i].Protocol < result.IdentitySnapshot.Protocols[j].Protocol
		})
		sort.Slice(entitlements, func(i, j int) bool {
			if entitlements[i].Protocol != entitlements[j].Protocol {
				return entitlements[i].Protocol < entitlements[j].Protocol
			}
			return entitlements[i].SubscriptionID < entitlements[j].SubscriptionID
		})
		b, err := json.Marshal(struct {
			Filter       []string                       `json:"filter"`
			Data         *dto.QueryServerConfigResponse `json:"data"`
			Entitlements []snapshotEntitlement          `json:"entitlements"`
		}{filter, result, entitlements})
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		result.IdentitySnapshot.SnapshotID = hex.EncodeToString(sum[:])
		return nil
	})
	return result, err
}

// snapshotProtocols projects the enabled listeners and checks the requested
// filter before any policy or identity is delivered.
func snapshotProtocols(server *node.Server, requested []string) ([]dto.Protocol, error) {
	if err := node.ValidateNodeProtocols(server); err != nil {
		return nil, err
	}
	raw, err := server.UnmarshalProtocols()
	if err != nil {
		return nil, err
	}
	raw = node.SanitizeProtocolsForNodeDistribution(raw)
	filters := make(map[string]bool)
	for _, value := range requested {
		p, e := node.NormalizeProtocolForStorage(node.Protocol{Type: value})
		if e == nil {
			filters[p.Type] = true
		} else {
			filters[value] = true
		}
	}
	protocols := make([]dto.Protocol, 0, len(raw))
	enabledCount := 0
	for _, item := range raw {
		if !item.Enable {
			continue
		}
		enabledCount++
		if len(filters) > 0 && !filters[item.Type] {
			continue
		}
		mapped, err := protocolmap.ToDTO([]node.Protocol{item})
		if err != nil {
			return nil, err
		}
		protocols = append(protocols, mapped[0])
	}
	if len(filters) > 0 && len(protocols) == 0 && enabledCount > 0 {
		return nil, fmt.Errorf("protocol filter excludes all enabled listeners")
	}
	return protocols, nil
}

type snapshotEntitlement struct {
	Protocol       string `json:"protocol"`
	SubscriptionID int64  `json:"subscription_id"`
	PlanID         int64  `json:"plan_id"`
	ExpiresAt      int64  `json:"expires_at"`
}

// All identity projections use store, the same repeatable-read transaction
// as the node and policy, including disabled-account filtering.
func snapshotIdentity(ctx context.Context, store repository.DeliverySnapshot, serverID int64, protocols []dto.Protocol, identity *dto.IdentitySnapshot) ([]snapshotEntitlement, error) {
	entitlements := make([]snapshotEntitlement, 0)
	seen := make(map[string]bool)
	now := time.Now().Unix()
	for _, protocol := range protocols {
		if seen[protocol.Type] {
			continue
		}
		seen[protocol.Type] = true
		users, entries, err := snapshotProtocolUsers(ctx, store, serverID, protocol.Type, now)
		if err != nil {
			return nil, err
		}
		identity.Protocols = append(identity.Protocols, dto.ProtocolUserSnapshot{Protocol: protocol.Type, Users: users})
		entitlements = append(entitlements, entries...)
	}
	return entitlements, nil
}

func snapshotProtocolUsers(ctx context.Context, store repository.DeliverySnapshot, serverID int64, protocol string, now int64) ([]dto.ServerUser, []snapshotEntitlement, error) {
	nodeIDs, tags, err := nodesForProtocol(ctx, store.Node(), serverID, protocol)
	if err != nil {
		return nil, nil, err
	}
	plans, err := store.RoutingPlans(ctx, nodeIDs, tags)
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[int64]*subscribe.Subscribe, len(plans))
	planIDs := make([]int64, 0, len(plans))
	for _, p := range plans {
		byID[p.Id] = p
		planIDs = append(planIDs, p.Id)
	}
	subs, err := store.RoutingSubscriptions(ctx, planIDs)
	if err != nil {
		return nil, nil, err
	}
	accountIDs := make([]int64, 0, len(subs))
	for _, sub := range subs {
		accountIDs = append(accountIDs, sub.UserId)
	}
	enabled, err := store.RoutingEnabledAccounts(ctx, accountIDs)
	if err != nil {
		return nil, nil, err
	}
	valid := make(map[int64]bool, len(enabled))
	for _, id := range enabled {
		valid[id] = true
	}
	users := make([]dto.ServerUser, 0, len(subs))
	entries := make([]snapshotEntitlement, 0)
	seenUUID := map[string]bool{}
	for _, sub := range subs {
		p := byID[sub.SubscribeId]
		if p == nil {
			continue
		}
		if sub.Status == 0 {
			return nil, nil, errPendingSnapshotSubscription
		}
		if !valid[sub.UserId] {
			continue
		}
		if sub.Id <= 0 || sub.Id > math.MaxInt32 || sub.UserId <= 0 || sub.Traffic < 0 || sub.Download < 0 || sub.Upload < 0 || p.SpeedLimit < 0 || p.SpeedLimit > math.MaxInt32 || p.DeviceLimit < 0 || p.DeviceLimit > math.MaxInt32 {
			return nil, nil, fmt.Errorf("invalid subscription limits")
		}
		if !usersub.NoExpiry(sub.ExpireTime) && sub.ExpireTime.Unix() < now {
			continue
		}
		if sub.Traffic > 0 && (sub.Download >= sub.Traffic || sub.Upload >= sub.Traffic-sub.Download) {
			continue
		}
		parsed, err := uuid.Parse(sub.UUID)
		if err != nil || seenUUID[parsed.String()] {
			return nil, nil, fmt.Errorf("invalid or duplicate subscription identity")
		}
		seenUUID[parsed.String()] = true
		users = append(users, dto.ServerUser{Id: sub.Id, UUID: sub.UUID, AccountID: sub.UserId, SpeedLimit: p.SpeedLimit, DeviceLimit: p.DeviceLimit})
		entries = append(entries, snapshotEntitlement{Protocol: protocol, SubscriptionID: sub.Id, PlanID: sub.SubscribeId, ExpiresAt: sub.ExpireTime.UnixMilli()})
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Id < users[j].Id })
	return users, entries, nil
}
