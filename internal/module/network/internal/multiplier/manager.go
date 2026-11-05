// Package multiplier applies the node traffic multipliers an administrator
// sets for times of day.
package multiplier

import "time"

var timeLayouts = []string{
	"15:04:05",
	"15:04",
	"15:04.000",
}

// TimePeriod is a daily time range, which may cross midnight, and the
// multiplier that applies within it.
type TimePeriod struct {
	StartTime  string  `json:"start_time"`
	EndTime    string  `json:"end_time"`
	Multiplier float32 `json:"multiplier"`
}

// Manager picks the multiplier in effect at a time from its periods.
type Manager struct {
	Periods []TimePeriod
}

// NewManager returns a manager over periods.
func NewManager(periods []TimePeriod) *Manager {
	return &Manager{
		Periods: periods,
	}
}

// GetMultiplier returns the multiplier of the first period containing
// current's time of day, or 1 when none does.
func (m *Manager) GetMultiplier(current time.Time) float32 {
	for _, period := range m.Periods {
		if m.isInTimePeriod(current, period.StartTime, period.EndTime) {
			return period.Multiplier
		}
	}
	return 1
}

func (m *Manager) isInTimePeriod(current time.Time, start, end string) bool {
	startTime, err := parseClock(start)
	if err != nil {
		return false
	}
	endTime, err := parseClock(end)
	if err != nil {
		return false
	}

	currentTime := time.Date(0, 1, 1, current.Hour(), current.Minute(), 0, 0, time.UTC)
	startTimeFormatted := time.Date(0, 1, 1, startTime.Hour(), startTime.Minute(), 0, 0, time.UTC)
	endTimeFormatted := time.Date(0, 1, 1, endTime.Hour(), endTime.Minute(), 0, 0, time.UTC)

	if startTimeFormatted.Before(endTimeFormatted) {
		return !currentTime.Before(startTimeFormatted) && !currentTime.After(endTimeFormatted)
	}
	// Handle ranges that cross midnight
	return !currentTime.Before(startTimeFormatted) || !currentTime.After(endTimeFormatted)
}

func parseClock(value string) (time.Time, error) {
	var lastErr error
	for _, layout := range timeLayouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}
