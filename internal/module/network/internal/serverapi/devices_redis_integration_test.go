package serverapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/redis/go-redis/v9"
)

// Run only against an explicitly configured private, loopback Redis. Each
// invocation owns and deletes only its randomized test keys, never a database.
func TestNativeRedisDeviceAdmission(t *testing.T) {
	addr := os.Getenv("PPANEL_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set PPANEL_TEST_REDIS_ADDR to a dedicated loopback Redis")
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		t.Fatal("non-loopback Redis refused")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	deps, store, _ := newScopeDeps(t)
	deps.Redis, store.redis = client, client
	sid := time.Now().UnixNano() & 0x3fffffffffffffff
	serverID := sid + 1
	cacheKey := serverUserListCacheKey(serverID, "vless")
	leaseKey := onlineDeviceKey(sid)
	t.Cleanup(func() { _ = client.Del(context.Background(), cacheKey, leaseKey).Err() })
	body, err := json.Marshal(dto.GetServerUserListResponse{Users: []dto.ServerUser{{Id: sid, UUID: "native-test", DeviceLimit: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Set(t.Context(), cacheKey, body, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	svc := NewService(deps)
	claim := func(ip string) (*dto.ClaimServerDeviceResponse, error) {
		return svc.ClaimServerDevice(context.Background(), &dto.ClaimServerDeviceRequest{ServerCommon: dto.ServerCommon{ServerId: serverID, Protocol: "vless"}, SID: sid, IP: ip})
	}
	var accepted atomic.Int32
	var winner atomic.Value
	var wg sync.WaitGroup
	for i := 1; i <= 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ip := fmt.Sprintf("10.7.0.%d", i)
			r, e := claim(ip)
			if e != nil {
				t.Error(e)
				return
			}
			if r.Allowed {
				accepted.Add(1)
				winner.Store(ip)
			}
		}(i)
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatalf("accepted %d unique IPs, want 1", accepted.Load())
	}
	if r, e := claim("::ffff:" + winner.Load().(string)); e != nil || !r.Allowed || r.TTLSeconds != 300 {
		t.Fatalf("mapped IP renewal: %+v, %v", r, e)
	}
	alive, e := svc.GetServerAliveList(t.Context(), &dto.GetServerUserListRequest{ServerCommon: dto.ServerCommon{ServerId: serverID, Protocol: "vless"}})
	if e != nil || alive.Alive[sid] != 1 || len(alive.Alive) != 1 {
		t.Fatalf("alive: %+v, %v", alive, e)
	}
	if ttl := client.TTL(t.Context(), leaseKey).Val(); ttl <= 0 || ttl > 5*time.Minute {
		t.Fatalf("lease ttl %v", ttl)
	}
}
