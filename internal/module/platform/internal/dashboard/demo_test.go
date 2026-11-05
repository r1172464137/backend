package dashboard

import (
	"testing"
	"time"
)

// The demo series run oldest first and end today and this month; each point's
// base comes from how far back it lies.
func TestDemoSeriesRunOldestFirst(t *testing.T) {
	now := time.Date(2026, 9, 28, 15, 0, 0, 0, time.UTC)
	type point struct {
		date string
		base int64
	}
	days, months := demoSeries(now,
		func(ago int) int64 { return int64(100 + ago) },
		func(ago int) int64 { return int64(1000 + ago) },
		func(date string, base int64) point { return point{date, base} })

	if len(days) != 7 || days[0] != (point{"2026-09-22", 106}) || days[6] != (point{"2026-09-28", 100}) {
		t.Fatalf("days = %+v", days)
	}
	if len(months) != 6 || months[0] != (point{"2026-04", 1005}) || months[5] != (point{"2026-09", 1000}) {
		t.Fatalf("months = %+v", months)
	}
	if got := mockRevenueStatistics(); len(got.Monthly.List) != 7 || len(got.All.List) != 6 {
		t.Fatalf("demo revenue series = %d/%d points", len(got.Monthly.List), len(got.All.List))
	}
	if got := mockUserStatistics(); len(got.Monthly.List) != 7 || len(got.All.List) != 6 {
		t.Fatalf("demo user series = %d/%d points", len(got.Monthly.List), len(got.All.List))
	}
}
