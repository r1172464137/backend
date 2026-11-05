package period

import (
	"testing"
	"time"
)

// LastReset is the most recent reset day at or before the given instant: the
// day itself when it is a reset day, else the previous one, moved to the
// month's last day when the start day does not exist in that month.
func TestLastResetIsTheMostRecentResetDayAtOrBefore(t *testing.T) {
	cal := In(time.UTC)
	on := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	start := time.Date(2026, 1, 31, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		cycle Cycle
		at    time.Time
		want  time.Time
		ok    bool
	}{
		{"first of month, later in the month", CycleFirstOfMonth, on(2026, 6, 3).Add(30 * time.Minute), on(2026, 6, 1), true},
		{"first of month, on the 1st", CycleFirstOfMonth, on(2026, 6, 1), on(2026, 6, 1), true},
		{"monthly, before the month's day", CycleMonthly, on(2026, 6, 15), on(2026, 5, 31), true},
		{"monthly, on the month's last day", CycleMonthly, on(2026, 6, 30), on(2026, 6, 30), true},
		{"monthly, after the month's day", CycleMonthly, on(2026, 7, 2), on(2026, 6, 30), true},
		{"yearly, before the anniversary", CycleYearly, on(2026, 1, 15), on(2025, 1, 31), true},
		{"yearly, on the anniversary", CycleYearly, on(2026, 1, 31), on(2026, 1, 31), true},
		{"no calendar reset", CycleNone, on(2026, 6, 3), time.Time{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := cal.LastReset(tt.cycle, start, tt.at)
			if ok != tt.ok || (ok && !got.Equal(tt.want)) {
				t.Fatalf("LastReset = %v, %v; want %v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}
