package routingstatus

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/redis/go-redis/v9"
)

func TestSequenceOrderingIndependentOfWallClock(t *testing.T) {
	s := miniredis.RunT(t)
	start := time.Unix(1800000000, 0)
	s.SetTime(start)
	client := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { client.Close() })
	ctx := context.Background()
	id := func(rev int64) *dto.RoutingCandidateIdentity {
		return &dto.RoutingCandidateIdentity{Mode: "ordered-v1-inherited", Source: "global", SourceRevision: rev}
	}
	a := &dto.ServerPushStatusRequest{RoutingVersion: 1, InstanceID: "11111111-1111-4111-8111-111111111111", RoutingSequence: 1, UpdatedAt: 100, RoutingApplied: id(1)}
	record := func(v *dto.ServerPushStatusRequest) {
		t.Helper()
		if err := Record(ctx, client, 7, v); err != nil {
			t.Fatal(err)
		}
	}
	read := func() dto.RoutingStatusView {
		t.Helper()
		v, err := Read(ctx, client, 7)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	record(a)
	// Same millisecond and a rolled-back clock both carry newer content when
	// the sequence advances. Old in-flight reports renew only liveness.
	newer := *a
	newer.RoutingSequence = 2
	newer.RoutingApplied = id(2)
	record(&newer)
	newer.RoutingSequence = 3
	newer.UpdatedAt = 1
	newer.RoutingApplied = id(3)
	record(&newer)
	s.FastForward(100 * time.Second)
	s.SetTime(start.Add(100 * time.Second))
	record(a)
	v := read()
	if !v.Known || v.Report.Applied.SourceRevision != 3 || v.Report.RoutingSequence != 3 || v.Report.ReceivedAt != start.Unix() || v.Report.LivenessAt != start.Add(100*time.Second).Unix() {
		t.Fatalf("late report: %#v", v)
	}
	// Identical sequence and routing content but different clock is a retry,
	// not a conflict. Same sequence and different content is ambiguous.
	same := newer
	same.UpdatedAt = 0
	record(&same)
	if !read().Known {
		t.Fatal("same content/sequence conflicted due to timestamp")
	}
	conflicting := same
	conflicting.RoutingPending = id(4)
	record(&conflicting)
	if read().Known {
		t.Fatal("different content with same sequence accepted")
	}
	record(&same)
	if read().Known {
		t.Fatal("equal sequence resolved conflict")
	}
	conflicting.RoutingSequence = 4
	record(&conflicting)
	if v = read(); !v.Known || v.Report.Pending.SourceRevision != 4 {
		t.Fatalf("newer sequence did not resolve conflict: %#v", v)
	}
	// No float conversion of uint64 sequences or int64 revision content in Lua.
	conflicting.RoutingSequence = math.MaxUint64
	conflicting.RoutingApplied = id(math.MaxInt64)
	record(&conflicting)
	old := conflicting
	old.RoutingSequence = math.MaxUint64 - 1
	old.RoutingApplied = id(1)
	record(&old)
	if v = read(); !v.Known || v.Report.RoutingSequence != math.MaxUint64 || v.Report.Applied.SourceRevision != math.MaxInt64 {
		t.Fatalf("integer precision lost: %#v", v)
	}
}

func TestOldAndDuplicateSequenceCannotExtendContentFreshness(t *testing.T) {
	for _, sequence := range []uint64{1, 3} {
		t.Run(fmt.Sprintf("sequence_%d", sequence), func(t *testing.T) {
			s := miniredis.RunT(t)
			start := time.Unix(1800000000, 0)
			s.SetTime(start)
			client := redis.NewClient(&redis.Options{Addr: s.Addr()})
			t.Cleanup(func() { _ = client.Close() })
			ctx := context.Background()
			applied := &dto.RoutingCandidateIdentity{Mode: "ordered-v1-inherited", Source: "global", SourceRevision: 3}
			fresh := &dto.ServerPushStatusRequest{RoutingVersion: 1, InstanceID: "11111111-1111-4111-8111-111111111111", RoutingSequence: 3, RoutingApplied: applied}
			if err := Record(ctx, client, 7, fresh); err != nil {
				t.Fatal(err)
			}
			replayed := *fresh
			replayed.RoutingSequence = sequence
			if sequence == 1 {
				replayed.RoutingApplied = &dto.RoutingCandidateIdentity{Mode: "ordered-v1-inherited", Source: "global", SourceRevision: 1}
			}
			previous := start
			for _, elapsed := range []time.Duration{100 * time.Second, TTL - time.Second, TTL, TTL + 100*time.Second} {
				current := start.Add(elapsed)
				// FastForward moves Redis key TTL; SetTime is Redis TIME for Lua.
				s.FastForward(current.Sub(previous))
				s.SetTime(current)
				previous = current
				if err := Record(ctx, client, 7, &replayed); err != nil {
					t.Fatal(err)
				}
				view, err := Read(ctx, client, 7)
				if err != nil {
					t.Fatal(err)
				}
				if elapsed < TTL {
					if !view.Known || view.Report == nil || view.Report.Applied.SourceRevision != 3 || view.Report.ReceivedAt != start.Unix() || view.Report.LivenessAt != current.Unix() {
						t.Fatalf("before content expiry: %#v", view)
					}
				} else if view.Known || view.Report != nil {
					t.Fatalf("replayed sequence extended content freshness: %#v", view)
				}
			}
			// The instance is still active in Redis, although its applied
			// identity is unknown. Only a higher sequence refreshes content.
			if !client.HExists(ctx, key(7), fresh.InstanceID).Val() {
				t.Fatal("liveness record unexpectedly expired")
			}
			fresh.RoutingSequence = 4
			if err := Record(ctx, client, 7, fresh); err != nil {
				t.Fatal(err)
			}
			view, err := Read(ctx, client, 7)
			if err != nil || !view.Known || view.Report.RoutingSequence != 4 || view.Report.ReceivedAt != start.Add(TTL+100*time.Second).Unix() {
				t.Fatalf("new sequence did not refresh: %#v %v", view, err)
			}
		})
	}
}

func TestPreviousRedisRecordsWithoutContentTimestampFailClosed(t *testing.T) {
	s := miniredis.RunT(t)
	start := time.Unix(1800000000, 0)
	s.SetTime(start)
	client := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	instance := "11111111-1111-4111-8111-111111111111"
	// Shape emitted by the previous backend: liveness cannot be treated as
	// trusted content time retroactively.
	old := fmt.Sprintf(`{"received_at":%d,"sequence":"3","verified":true,"fingerprint":"old","payload":"{\"routing_version\":1}"}`, start.Unix())
	if err := client.HSet(ctx, key(7), instance, old).Err(); err != nil {
		t.Fatal(err)
	}
	if view, err := Read(ctx, client, 7); err != nil || view.Known {
		t.Fatalf("old record trusted: %#v %v", view, err)
	}
	newer := &dto.ServerPushStatusRequest{RoutingVersion: 1, InstanceID: instance, RoutingSequence: 4}
	if err := Record(ctx, client, 7, newer); err != nil {
		t.Fatal(err)
	}
	if view, err := Read(ctx, client, 7); err != nil || !view.Known || view.Report.RoutingSequence != 4 {
		t.Fatalf("new report did not recover: %#v %v", view, err)
	}
}

func TestExpiredLivenessRetainsSequenceWatermark(t *testing.T) {
	for _, readBefore := range []bool{false, true} {
		for _, sequence := range []uint64{1, 3} {
			t.Run(fmt.Sprintf("read_before_%v_sequence_%d", readBefore, sequence), func(t *testing.T) {
				s := miniredis.RunT(t)
				start := time.Unix(1800000000, 0)
				s.SetTime(start)
				client := redis.NewClient(&redis.Options{Addr: s.Addr()})
				t.Cleanup(func() { _ = client.Close() })
				ctx := context.Background()
				fresh := &dto.ServerPushStatusRequest{RoutingVersion: 1, InstanceID: "11111111-1111-4111-8111-111111111111", RoutingSequence: 3, RoutingApplied: &dto.RoutingCandidateIdentity{Mode: "ordered-v1-inherited", Source: "global", SourceRevision: 3}}
				if err := Record(ctx, client, 7, fresh); err != nil {
					t.Fatal(err)
				}
				s.FastForward(TTL + time.Second)
				s.SetTime(start.Add(TTL + time.Second))
				if readBefore {
					if view, err := Read(ctx, client, 7); err != nil || view.Known {
						t.Fatalf("silent instance: %#v %v", view, err)
					}
				}
				replayed := *fresh
				replayed.RoutingSequence = sequence
				if sequence == 1 {
					replayed.RoutingApplied = &dto.RoutingCandidateIdentity{Mode: "ordered-v1-inherited", Source: "global", SourceRevision: 1}
				}
				if err := Record(ctx, client, 7, &replayed); err != nil {
					t.Fatal(err)
				}
				if view, err := Read(ctx, client, 7); err != nil || view.Known {
					t.Fatalf("old sequence revived expired applied: %#v %v", view, err)
				}
				fresh.RoutingSequence = 4
				if err := Record(ctx, client, 7, fresh); err != nil {
					t.Fatal(err)
				}
				if view, err := Read(ctx, client, 7); err != nil || !view.Known || view.Report.RoutingSequence != 4 {
					t.Fatalf("new sequence did not recover: %#v %v", view, err)
				}
			})
		}
	}
}

func TestRetainedWatermarkLimitFailsClosed(t *testing.T) {
	s := miniredis.RunT(t)
	start := time.Unix(1800000000, 0)
	s.SetTime(start)
	client := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	send := func(instance string) {
		t.Helper()
		err := storeScript.Run(ctx, client, []string{key(7)}, `{}`, instance, int(TTL.Seconds()), maxInstances, "1", "1", "hash", int(watermarkRetention.Seconds()), 2).Err()
		if err != nil {
			t.Fatal(err)
		}
	}
	send("11111111-1111-4111-8111-111111111111")
	send("22222222-2222-4222-8222-222222222222")
	s.FastForward(TTL + time.Second)
	s.SetTime(start.Add(TTL + time.Second))
	// Both live slots expired, but their sequence watermarks still occupy the
	// deliberately small history budget. An untracked process is ambiguous.
	send("33333333-3333-4333-8333-333333333333")
	if !client.HExists(ctx, key(7), "__overflow").Val() || client.HExists(ctx, key(7), "33333333-3333-4333-8333-333333333333").Val() {
		t.Fatal("retention overflow was not recorded safely")
	}
	if view, err := Read(ctx, client, 7); err != nil || view.Known {
		t.Fatalf("overflow did not fail closed: %#v %v", view, err)
	}
}

func TestStaleContentStillCountsAsLiveInstance(t *testing.T) {
	s := miniredis.RunT(t)
	start := time.Unix(1800000000, 0)
	s.SetTime(start)
	client := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	a := &dto.ServerPushStatusRequest{RoutingVersion: 1, InstanceID: "11111111-1111-4111-8111-111111111111", RoutingSequence: 3}
	b := &dto.ServerPushStatusRequest{RoutingVersion: 1, InstanceID: "22222222-2222-4222-8222-222222222222", RoutingSequence: 1}
	for _, r := range []*dto.ServerPushStatusRequest{a, b} {
		if err := Record(ctx, client, 7, r); err != nil {
			t.Fatal(err)
		}
	}
	previous := start
	for _, elapsed := range []time.Duration{100 * time.Second, 181 * time.Second, 281 * time.Second} {
		current := start.Add(elapsed)
		s.FastForward(current.Sub(previous))
		s.SetTime(current)
		previous = current
		a.RoutingSequence++
		if err := Record(ctx, client, 7, a); err != nil {
			t.Fatal(err)
		}
		// B only replays sequence 1; its content expires but its process
		// remains active and must keep the server ambiguous.
		if err := Record(ctx, client, 7, b); err != nil {
			t.Fatal(err)
		}
		if view, err := Read(ctx, client, 7); err != nil || view.Known {
			t.Fatalf("two live instances: %#v %v", view, err)
		}
	}
	s.FastForward(TTL + time.Second)
	s.SetTime(start.Add(281*time.Second + TTL + time.Second))
	a.RoutingSequence++
	if err := Record(ctx, client, 7, a); err != nil {
		t.Fatal(err)
	}
	if view, err := Read(ctx, client, 7); err != nil || !view.Known || view.Report.InstanceID != a.InstanceID {
		t.Fatalf("only A live: %#v %v", view, err)
	}
}

func TestIndependentInstancesFreshnessAndUnknown(t *testing.T) {
	s := miniredis.RunT(t)
	start := time.Unix(1800000000, 0)
	s.SetTime(start)
	client := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { client.Close() })
	ctx := context.Background()
	a := &dto.ServerPushStatusRequest{RoutingVersion: 1, InstanceID: "11111111-1111-4111-8111-111111111111", RoutingSequence: 1, UpdatedAt: 100}
	b := &dto.ServerPushStatusRequest{RoutingVersion: 1, InstanceID: "22222222-2222-4222-8222-222222222222", RoutingSequence: 1, UpdatedAt: 100, RoutingError: &dto.RoutingReportError{Stage: "bind", Message: "token=secret"}}
	for _, r := range []*dto.ServerPushStatusRequest{a, b} {
		if err := Record(ctx, client, 7, r); err != nil {
			t.Fatal(err)
		}
	}
	if v, _ := Read(ctx, client, 7); v.Known {
		t.Fatal("two fresh instances must be unknown")
	}
	s.FastForward(100 * time.Second)
	s.SetTime(start.Add(100 * time.Second))
	b.RoutingSequence++
	b.UpdatedAt--
	if err := Record(ctx, client, 7, b); err != nil {
		t.Fatal(err)
	}
	s.FastForward(81 * time.Second)
	s.SetTime(start.Add(181 * time.Second))
	v, err := Read(ctx, client, 7)
	if err != nil || !v.Known || v.Report.InstanceID != b.InstanceID || v.Report.Error.Message == b.RoutingError.Message {
		t.Fatalf("one fresh sanitized reporter: %#v %v", v, err)
	}
	missing := *b
	missing.RoutingSequence = 0
	if err := Record(ctx, client, 7, &missing); err == nil {
		t.Fatal("missing sequence accepted")
	}
	if v, _ := Read(ctx, client, 7); v.Known {
		t.Fatal("invalid report did not make server unknown")
	}
	s.FastForward(TTL + time.Second)
	s.SetTime(start.Add(400 * time.Second))
	if v, _ := Read(ctx, client, 7); v.Known {
		t.Fatal("expired status treated as known")
	}
	b.RoutingSequence++
	if err := Record(ctx, client, 7, b); err != nil {
		t.Fatal(err)
	}
	s.Del(key(7))
	if v, _ := Read(ctx, client, 7); v.Known {
		t.Fatal("lost state treated as known")
	}
}
