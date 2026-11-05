package dashboard

import "time"

// demoSeries builds a demo dashboard's two series, oldest point first: the
// last seven days and the last six months. base gives a point's base value
// from how many days (or months) ago it lies; point shapes it.
func demoSeries[T any](now time.Time, dayBase, monthBase func(ago int) int64, point func(date string, base int64) T) (days, months []T) {
	days = make([]T, 7)
	for i := range days {
		ago := len(days) - 1 - i
		days[i] = point(now.AddDate(0, 0, -ago).Format(time.DateOnly), dayBase(ago))
	}
	months = make([]T, 6)
	for i := range months {
		ago := len(months) - 1 - i
		months[i] = point(now.AddDate(0, -ago, 0).Format("2006-01"), monthBase(ago))
	}
	return days, months
}
