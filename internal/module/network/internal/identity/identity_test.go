package identity

import (
	"context"
	"testing"
	"time"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
)

type fakeNodes struct {
	repository.NodeRepo
	byProtocol map[string][]*node.Node
}

func (f fakeNodes) ListNodes(_ context.Context, p *node.FilterNodeParams) ([]*node.Node, error) {
	return f.byProtocol[p.Protocol], nil
}

type fakePlans struct {
	repository.SubscribeRepo
	plans []*subscribe.Subscribe
}

func (f fakePlans) FindByNodeScope(_ context.Context, ids []int64, tags []string) ([]*subscribe.Subscribe, error) {
	return f.plans, nil
}

type fakeSubs struct {
	repository.UserSubscriptionRepo
	subs      []*usersub.Subscribe
	activated *bool
}

func (f fakeSubs) ActivatePendingSubscribesBySubscribeIds(context.Context, []int64) error {
	*f.activated = true
	return nil
}
func (f fakeSubs) FindUsersSubscribeBySubscribeIds(context.Context, []int64) ([]*usersub.Subscribe, error) {
	return f.subs, nil
}

type fakeUsers struct {
	repository.UserRepo
	enabled map[int64]bool
}

func (f fakeUsers) FindEnabledUserIDs(_ context.Context, ids []int64) ([]int64, error) {
	var out []int64
	for _, id := range ids {
		if f.enabled[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

type fakeReader struct {
	nodes fakeNodes
	plans fakePlans
	subs  fakeSubs
	users fakeUsers
}

func (r fakeReader) Node() repository.NodeRepo                         { return r.nodes }
func (r fakeReader) Subscribe() repository.SubscribeRepo               { return r.plans }
func (r fakeReader) UserSubscription() repository.UserSubscriptionRepo { return r.subs }
func (r fakeReader) User() repository.UserRepo                         { return r.users }

var now = time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)

func fixture() (fakeReader, *bool) {
	activated := false
	return fakeReader{
		nodes: fakeNodes{byProtocol: map[string][]*node.Node{"vless": {{Id: 1}}, "trojan": {{Id: 2}}}},
		plans: fakePlans{plans: []*subscribe.Subscribe{{Id: 10, SpeedLimit: 100, DeviceLimit: 3}}},
		subs: fakeSubs{activated: &activated, subs: []*usersub.Subscribe{
			{Id: 5, UserId: 42, SubscribeId: 10, UUID: "00000000-0000-4000-8000-000000000005", Status: 1, ExpireTime: now.Add(time.Hour)},
			{Id: 3, UserId: 43, SubscribeId: 10, UUID: "00000000-0000-4000-8000-000000000003", Status: 1, ExpireTime: time.UnixMilli(0)},          // never expires
			{Id: 7, UserId: 44, SubscribeId: 10, UUID: "00000000-0000-4000-8000-000000000007", Status: 1, ExpireTime: now.Add(-time.Minute)},      // expired, not yet swept
			{Id: 8, UserId: 45, SubscribeId: 10, UUID: "00000000-0000-4000-8000-000000000008", Status: 1, Traffic: 100, Upload: 60, Download: 40}, // exhausted
			{Id: 9, UserId: 46, SubscribeId: 10, UUID: "00000000-0000-4000-8000-000000000009", Status: 1},                                         // account disabled
			{Id: 11, UserId: 42, SubscribeId: 99, UUID: "00000000-0000-4000-8000-000000000011", Status: 1},                                        // plan not authorised here
		}},
		users: fakeUsers{enabled: map[int64]bool{42: true, 43: true, 44: true, 45: true}},
	}, &activated
}

func protocols(types ...string) []dto.Protocol {
	var out []dto.Protocol
	for i, t := range types {
		out = append(out, dto.Protocol{Type: t, Port: uint16(443 + i), Enable: true})
	}
	return out
}

func routing() *dto.NodeRoutingPolicy {
	return &dto.NodeRoutingPolicy{Version: config.RoutingVersion, Outbounds: []dto.NodeOutbound{}, Rules: []dto.NodeRoutingRule{},
		DefaultOutboundID: "direct", UserDefaults: []config.NodeUserDefault{}, DNS: config.DefaultDNSPolicy("g"),
		TargetIPPreference: "none", Mode: "inherited", Source: "global", SourceRevision: 1}
}

func TestBuildFiltersAndOrdersIdentities(t *testing.T) {
	r, activated := fixture()
	res, err := Build(context.Background(), r, 7, protocols("vless", "trojan"), now, false)
	if err != nil {
		t.Fatal(err)
	}
	if *activated {
		t.Fatal("read-only build activated pending subscriptions")
	}
	if len(res.Snapshot.Protocols) != 2 || res.Snapshot.Protocols[0].Protocol != "trojan" || res.Snapshot.Protocols[1].Protocol != "vless" {
		t.Fatalf("protocols not sorted/complete: %+v", res.Snapshot.Protocols)
	}
	users := res.Snapshot.Protocols[1].Users
	if len(users) != 2 || users[0].ID != 3 || users[1].ID != 5 || users[1].AccountID != 42 || users[1].SpeedLimit != 100 || users[1].DeviceLimit != 3 {
		t.Fatalf("users = %+v; want only valid, enabled, authorised subscriptions 3 and 5 sorted by id", users)
	}
	if _, err := Build(context.Background(), r, 7, protocols("vless"), now, true); err != nil || !*activated {
		t.Fatalf("node pull did not activate pending subscriptions: %v", err)
	}
}

func TestEmptyProtocolHasNoPlaceholder(t *testing.T) {
	r, _ := fixture()
	r.nodes.byProtocol = map[string][]*node.Node{}
	res, err := Build(context.Background(), r, 7, protocols("vless"), now, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Snapshot.Protocols) != 1 || res.Snapshot.Protocols[0].Users == nil || len(res.Snapshot.Protocols[0].Users) != 0 {
		t.Fatalf("zero authorised identities must be an empty array: %+v", res.Snapshot.Protocols)
	}
}

func TestDuplicateUUIDRejectsCandidate(t *testing.T) {
	r, _ := fixture()
	r.subs.subs = append(r.subs.subs, &usersub.Subscribe{Id: 12, UserId: 43, SubscribeId: 10, UUID: "00000000-0000-4000-8000-000000000005", Status: 1})
	if _, err := Build(context.Background(), r, 7, protocols("vless"), now, false); err == nil {
		t.Fatal("same UUID for two accounts accepted")
	}
	r, _ = fixture()
	r.subs.subs = append(r.subs.subs, &usersub.Subscribe{Id: 12, UserId: 42, SubscribeId: 10, UUID: "00000000-0000-4000-8000-000000000005", Status: 1})
	if _, err := Build(context.Background(), r, 7, protocols("vless"), now, false); err == nil {
		t.Fatal("duplicate UUID accepted")
	}
}

func TestSnapshotIDTracksEveryCandidateChange(t *testing.T) {
	id := func(mutate func(r *fakeReader, p *dto.NodeRoutingPolicy, protos []dto.Protocol) []dto.Protocol) string {
		r, _ := fixture()
		p := routing()
		protos := protocols("vless")
		if mutate != nil {
			protos = mutate(&r, p, protos)
		}
		res, err := Build(context.Background(), r, 7, protos, now, false)
		if err != nil {
			t.Fatal(err)
		}
		s, err := SnapshotID(p, protos, res)
		if err != nil {
			t.Fatal(err)
		}
		if len(s) != 64 {
			t.Fatalf("snapshot id %q", s)
		}
		return s
	}
	base := id(nil)
	if base != id(nil) {
		t.Fatal("snapshot id is not deterministic")
	}
	changes := map[string]func(*fakeReader, *dto.NodeRoutingPolicy, []dto.Protocol) []dto.Protocol{
		"renewal (expiry)": func(r *fakeReader, _ *dto.NodeRoutingPolicy, p []dto.Protocol) []dto.Protocol {
			r.subs.subs[0].ExpireTime = now.Add(48 * time.Hour)
			return p
		},
		"quota": func(r *fakeReader, _ *dto.NodeRoutingPolicy, p []dto.Protocol) []dto.Protocol {
			r.subs.subs[0].Traffic = 1 << 40
			return p
		},
		"speed limit": func(r *fakeReader, _ *dto.NodeRoutingPolicy, p []dto.Protocol) []dto.Protocol {
			r.plans.plans[0].SpeedLimit = 1
			return p
		},
		"device limit": func(r *fakeReader, _ *dto.NodeRoutingPolicy, p []dto.Protocol) []dto.Protocol {
			r.plans.plans[0].DeviceLimit = 1
			return p
		},
		"uuid rotation": func(r *fakeReader, _ *dto.NodeRoutingPolicy, p []dto.Protocol) []dto.Protocol {
			r.subs.subs[0].UUID = "00000000-0000-4000-8000-0000000000ff"
			return p
		},
		"revocation": func(r *fakeReader, _ *dto.NodeRoutingPolicy, p []dto.Protocol) []dto.Protocol {
			r.users.enabled[42] = false
			return p
		},
		"routing revision": func(_ *fakeReader, rp *dto.NodeRoutingPolicy, p []dto.Protocol) []dto.Protocol {
			rp.SourceRevision = 2
			return p
		},
		"dns": func(_ *fakeReader, rp *dto.NodeRoutingPolicy, p []dto.Protocol) []dto.Protocol {
			rp.DNS.QueryStrategy = "UseIPv4"
			return p
		},
		"preference": func(_ *fakeReader, rp *dto.NodeRoutingPolicy, p []dto.Protocol) []dto.Protocol {
			rp.TargetIPPreference = "prefer_ipv4"
			return p
		},
		"listener port": func(_ *fakeReader, _ *dto.NodeRoutingPolicy, p []dto.Protocol) []dto.Protocol {
			p[0].Port = 8443
			return p
		},
	}
	for name, change := range changes {
		if id(change) == base {
			t.Errorf("%s did not change the snapshot id", name)
		}
	}
}
