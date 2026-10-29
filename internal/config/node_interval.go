package config

import "fmt"

// Zero asks ppnode to use its default interval. Nonzero values are bounded
// before conversion to time.Duration; the ceiling is shared by this batch.
const MaxNodeIntervalSeconds int64 = 86400

func ValidateNodeInterval(field string, seconds int64) error {
	if seconds < 0 || seconds > MaxNodeIntervalSeconds {
		return fmt.Errorf("%s must be 0 (default) or between 1 and %d seconds", field, MaxNodeIntervalSeconds)
	}
	return nil
}
