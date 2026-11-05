package serverapi

import (
	"context"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/redis/go-redis/v9"
)

func TestDesiredSnapshotMissingEvidenceAndRedisFailure(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr(), MaxRetries: -1})
	status := dto.RoutingStatusView{Known: true, ReporterVerified: true, Report: &dto.RoutingStatusReport{Version: 2, Applied: &dto.RoutingCandidateIdentity{SnapshotID: strings.Repeat("a", 64)}}}
	svc := NewService(Deps{Redis: client})
	if id, err := svc.DesiredSnapshotID(context.Background(), 1, status); err != nil || id != nil {
		t.Fatalf("missing filter = %v, %v; want unknown", id, err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if id, err := svc.DesiredSnapshotID(context.Background(), 1, status); err == nil || id != nil {
		t.Fatalf("closed redis = %v, %v; want unknown and operational error", id, err)
	}
}
