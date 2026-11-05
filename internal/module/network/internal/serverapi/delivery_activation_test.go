package serverapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	networkrepo "github.com/perfect-panel/server/internal/module/network/internal/repo"
	"github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// The test port represents a real, mutable pending subscription. Read-only
// scope checks must not invoke its activation even on a cache miss.
type pendingScope struct {
	repository.SubscribeRepo
	repository.UserSubscriptionRepo
	status      uint8
	activations int
}

func (p *pendingScope) FindByNodeScope(_ context.Context, ids []int64, _ []string) ([]*subscribe.Subscribe, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return []*subscribe.Subscribe{{Id: 7, DeviceLimit: 1}}, nil
}
func (p *pendingScope) FindUsersSubscribeBySubscribeIds(_ context.Context, ids []int64) ([]*usersub.Subscribe, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return []*usersub.Subscribe{{Id: 21, UserId: 100, SubscribeId: 7, UUID: "uuid-21", Status: p.status}}, nil
}
func (p *pendingScope) ActivatePendingSubscribesBySubscribeIds(_ context.Context, ids []int64) error {
	if len(ids) > 0 {
		p.activations++
		p.status = usersub.SubscribeStatusActive
	}
	return nil
}

func TestScopeCacheUsesServerIndexAndGeneration(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	repo := networkrepo.NewNodeRepo(nil, client)
	ctx := t.Context()
	const serverID = 48
	key := serverUserScopeCacheKey(serverID, "vless")
	generation, err := repo.ServerCacheGeneration(ctx, serverID)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SetServerCache(ctx, serverID, key, "fresh", generation); err != nil {
		t.Fatal(err)
	}
	if !mini.Exists(key) {
		t.Fatal("scope cache was not stored")
	}
	if err := repo.ClearServerCache(ctx, serverID); err != nil {
		t.Fatal(err)
	}
	if mini.Exists(key) {
		t.Fatal("indexed invalidation left scope cache")
	}
	if err := repo.SetServerCache(ctx, serverID, key, "stale", generation); err != nil {
		t.Fatal(err)
	}
	if mini.Exists(key) {
		t.Fatal("stale generation repopulated scope cache")
	}
}

func TestScopeReadsCanUseDeliveryCacheWithoutActivationPort(t *testing.T) {
	deps, _, client := newScopeDeps(t)
	deps.Subscriptions = nil
	payload := `{"users":[{"id":21,"uuid":"uuid-21","speed_limit":0,"device_limit":1}]}`
	if err := client.Set(t.Context(), serverUserListCacheKey(4, "vless"), payload, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	ids, err := NewService(deps).servedSubscriptionIDs(t.Context(), 4, "vless")
	if err != nil || len(ids) != 1 {
		t.Fatalf("cached scope: %v, %v", ids, err)
	}
	if _, ok := ids[21]; !ok {
		t.Fatalf("cached scope lost subscription: %v", ids)
	}
}

func TestColdScopeReadsDoNotActivateOrPoisonDeliveryCache(t *testing.T) {
	for _, operation := range []string{"scope", "alive", "device", "traffic", "online"} {
		t.Run(operation, func(t *testing.T) {
			deps, store, client := newScopeDeps(t)
			p := &pendingScope{status: usersub.SubscribeStatusPending}
			deps.Subscriptions = subscription.New(subscription.Deps{Plans: p, UserSubs: p})
			deps.Online = &onlineRecorder{}
			svc := NewService(deps)
			common := dto.ServerCommon{ServerId: 4, Protocol: "vless"}
			read := func() error { return readPendingScope(t.Context(), svc, common, operation) }
			if err := read(); err != nil {
				t.Fatal(err)
			}
			if err := read(); err != nil {
				t.Fatal(err)
			}
			if p.status != usersub.SubscribeStatusPending || p.activations != 0 {
				t.Fatalf("scope read activated: status=%d calls=%d", p.status, p.activations)
			}
			if store.lookups != 1 {
				t.Fatalf("scope rebuilt %d times", store.lookups)
			}
			if _, err := client.Get(t.Context(), serverUserListCacheKey(4, "vless")).Result(); !errors.Is(err, redis.Nil) {
				t.Fatalf("delivery cache should be empty: %v", err)
			}
			if ttl := client.TTL(t.Context(), serverUserScopeCacheKey(4, "vless")).Val(); ttl <= 0 || ttl > time.Minute {
				t.Fatalf("scope cache TTL: %v", ttl)
			}
			resp, _, err := svc.GetServerUserList(t.Context(), &dto.GetServerUserListRequest{ServerCommon: common}, RequestMeta{})
			if err != nil || len(resp.Users) != 1 || resp.Users[0].Id != 21 {
				t.Fatalf("delivery: %+v, %v", resp, err)
			}
			if p.status != usersub.SubscribeStatusActive || p.activations != 1 || store.lookups != 2 {
				t.Fatalf("delivery activation: status=%d calls=%d lookups=%d", p.status, p.activations, store.lookups)
			}
			if _, _, err := svc.GetServerUserList(t.Context(), &dto.GetServerUserListRequest{ServerCommon: common}, RequestMeta{}); err != nil {
				t.Fatal(err)
			}
			if err := read(); err != nil {
				t.Fatal(err)
			}
			if p.activations != 1 || store.lookups != 2 {
				t.Fatalf("warm delivery was rebuilt: activations=%d lookups=%d", p.activations, store.lookups)
			}
		})
	}
}

func readPendingScope(ctx context.Context, svc *Service, common dto.ServerCommon, operation string) error {
	switch operation {
	case "scope":
		_, err := svc.servedSubscriptionIDs(ctx, 4, "vless")
		return err
	case "alive":
		_, err := svc.GetServerAliveList(ctx, &dto.GetServerUserListRequest{ServerCommon: common})
		return err
	case "device":
		_, err := svc.ClaimServerDevice(ctx, &dto.ClaimServerDeviceRequest{ServerCommon: common, SID: 21, IP: "192.0.2.1"})
		return err
	case "traffic":
		return svc.ServerPushUserTraffic(ctx, &dto.ServerPushUserTrafficRequest{ServerCommon: common, Traffic: []dto.UserTraffic{{SID: 21, Upload: 10}}})
	default:
		return svc.PushOnlineUsers(ctx, &dto.OnlineUsersRequest{ServerCommon: common, Users: []dto.OnlineUser{{SID: 21, IP: "192.0.2.1"}}})
	}
}
