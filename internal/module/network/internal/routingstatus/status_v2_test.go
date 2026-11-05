package routingstatus

import (
	"context"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/redis/go-redis/v9"
)

func TestV2StatusAndDeliveryFilter(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close redis: %v", err)
		}
	})
	ctx := context.Background()
	first := strings.Repeat("a", 64)
	second := strings.Repeat("b", 64)
	if err := RememberFilter(ctx, client, 7, first, []string{"vless", "trojan", "vless"}); err != nil {
		t.Fatal(err)
	}
	filters, err := ReadFilter(ctx, client, 7, first)
	if err != nil || len(filters) != 2 || filters[0] != "trojan" || filters[1] != "vless" {
		t.Fatalf("filter %v %v", filters, err)
	}
	report := &dto.ServerPushStatusRequest{RoutingVersion: 2, InstanceID: "11111111-1111-4111-8111-111111111111", RoutingSequence: 1, RoutingApplied: &dto.RoutingCandidateIdentity{Mode: "ordered-v2-inherited", Source: "global", SourceRevision: 3, SnapshotID: first}}
	if err := Record(ctx, client, 7, report); err != nil {
		t.Fatal(err)
	}
	report.RoutingSequence = 2
	report.RoutingPending = &dto.RoutingCandidateIdentity{Mode: "ordered-v2-inherited", Source: "global", SourceRevision: 3, SnapshotID: second}
	if err := Record(ctx, client, 7, report); err != nil {
		t.Fatal(err)
	}
	view, err := Read(ctx, client, 7)
	if err != nil || !view.Known || view.Report.Applied.SnapshotID != first || view.Report.Pending.SnapshotID != second {
		t.Fatalf("view %+v %v", view, err)
	}
	report.RoutingSequence = 3
	report.RoutingPending.SnapshotID = "invalid"
	if err := Record(ctx, client, 7, report); err == nil {
		t.Fatal("invalid snapshot ID accepted")
	}
}
