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
	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/network/internal/nodeconfig"
	"github.com/perfect-panel/server/internal/module/network/internal/routingstatus"
	"github.com/perfect-panel/server/internal/module/platform"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
)

var errPendingSnapshotSubscription = errors.New("pending subscription appeared during delivery snapshot")

// plansForProtocol follows the same scope resolution as the existing user endpoint.
func plansForProtocol(ctx context.Context, nodes repository.NodeRepo, plans repository.SubscribeRepo, serverID int64, protocol string) ([]*subscribe.Subscribe, error) {
	matches, err := nodes.ListNodes(ctx, &node.FilterNodeParams{ServerId: []int64{serverID}, Protocol: protocol})
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(matches))
	var tags []string
	for _, n := range matches {
		ids = append(ids, n.Id)
		if n.Tags != "" {
			tags = append(tags, strings.Split(n.Tags, ",")...)
		}
	}
	return plans.FindByNodeScope(ctx, ids, tags)
}

func (l *QueryServerProtocolConfigLogic) v2Delivery(req *dto.QueryServerConfigRequest, observed *dto.QueryServerConfigResponse) (*dto.QueryServerConfigResponse, error) {
	filter, err := routingstatus.CanonicalFilter(req.Protocols)
	if err != nil {
		return nil, err
	}
	// Existing pending activation is a write, so complete it before the read-only snapshot.
	planIDs := make(map[int64]bool)
	for _, protocol := range observed.Protocols {
		plans, err := plansForProtocol(l.ctx, l.deps.Store.Node(), l.deps.Store.Subscribe(), req.ServerID, protocol.Type)
		if err != nil {
			return nil, err
		}
		for _, p := range plans {
			planIDs[p.Id] = true
		}
	}
	ids := make([]int64, 0, len(planIDs))
	for id := range planIDs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if err := l.deps.Store.UserSubscription().ActivatePendingSubscribesBySubscribeIds(l.ctx, ids); err != nil {
		return nil, err
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
		if err := node.ValidateNodeProtocols(server); err != nil {
			return err
		}
		raw, err := server.UnmarshalProtocols()
		if err != nil {
			return err
		}
		raw = node.SanitizeProtocolsForNodeDistribution(raw)
		filters := make(map[string]bool)
		for _, value := range req.Protocols {
			p, e := node.NormalizeProtocolForStorage(node.Protocol{Type: value})
			if e == nil {
				filters[p.Type] = true
			} else {
				filters[value] = true
			}
		}
		protocols := make([]dto.Protocol, 0, len(raw))
		enabledCount := 0
		// Use the same projection as the existing server configuration endpoint.
		for _, item := range raw {
			if !item.Enable {
				continue
			}
			enabledCount++
			if len(filters) > 0 && !filters[item.Type] {
				continue
			}
			var p dto.Protocol
			mapping.DeepCopy(&p, item)
			protocols = append(protocols, p)
		}
		if len(filters) > 0 && len(protocols) == 0 && enabledCount > 0 {
			return fmt.Errorf("protocol filter excludes all enabled listeners")
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
		type entitlement struct {
			Protocol       string `json:"protocol"`
			SubscriptionID int64  `json:"subscription_id"`
			PlanID         int64  `json:"plan_id"`
			ExpiresAt      int64  `json:"expires_at"`
		}
		entitlements := make([]entitlement, 0)
		now := time.Now().Unix()
		seenProtocol := make(map[string]bool)
		for _, protocol := range protocols {
			if seenProtocol[protocol.Type] {
				continue
			}
			seenProtocol[protocol.Type] = true
			plans, err := plansForProtocol(l.ctx, store.Node(), store.Subscribe(), req.ServerID, protocol.Type)
			if err != nil {
				return err
			}
			byID := make(map[int64]*subscribe.Subscribe, len(plans))
			planIDs := make([]int64, 0, len(plans))
			for _, p := range plans {
				byID[p.Id] = p
				planIDs = append(planIDs, p.Id)
			}
			subs, err := store.UserSubscription().FindUsersSubscribeBySubscribeIds(l.ctx, planIDs)
			if err != nil {
				return err
			}
			accountIDs := make([]int64, 0, len(subs))
			for _, s := range subs {
				accountIDs = append(accountIDs, s.UserId)
			}
			enabled, err := store.User().FindEnabledUserIDs(l.ctx, accountIDs)
			if err != nil {
				return err
			}
			valid := make(map[int64]bool, len(enabled))
			for _, id := range enabled {
				valid[id] = true
			}
			users := make([]dto.ServerUser, 0, len(subs))
			seenUUID := map[string]bool{}
			for _, s := range subs {
				p := byID[s.SubscribeId]
				if p == nil {
					continue
				}
				if s.Status == 0 {
					return errPendingSnapshotSubscription
				}
				if !valid[s.UserId] {
					continue
				}
				if s.Id <= 0 || s.Id > math.MaxInt32 || s.UserId <= 0 || s.Traffic < 0 || s.Download < 0 || s.Upload < 0 || p.SpeedLimit < 0 || p.SpeedLimit > math.MaxInt32 || p.DeviceLimit < 0 || p.DeviceLimit > math.MaxInt32 {
					return fmt.Errorf("invalid subscription limits")
				}
				expiry := s.ExpireTime.Unix()
				if expiry != 0 && expiry < now {
					continue
				}
				if s.Traffic > 0 && (s.Download >= s.Traffic || s.Upload >= s.Traffic-s.Download) {
					continue
				}
				parsed, parseErr := uuid.Parse(s.UUID)
				if parseErr != nil || seenUUID[parsed.String()] {
					return fmt.Errorf("invalid or duplicate subscription identity")
				}
				seenUUID[parsed.String()] = true
				users = append(users, dto.ServerUser{Id: s.Id, UUID: s.UUID, AccountID: s.UserId, SpeedLimit: p.SpeedLimit, DeviceLimit: p.DeviceLimit})
				entitlements = append(entitlements, entitlement{Protocol: protocol.Type, SubscriptionID: s.Id, PlanID: s.SubscribeId, ExpiresAt: s.ExpireTime.UnixMilli()})
			}
			sort.Slice(users, func(i, j int) bool { return users[i].Id < users[j].Id })
			result.IdentitySnapshot.Protocols = append(result.IdentitySnapshot.Protocols, dto.ProtocolUserSnapshot{Protocol: protocol.Type, Users: users})
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
			Entitlements []entitlement                  `json:"entitlements"`
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
