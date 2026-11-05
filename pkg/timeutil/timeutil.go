// Package timeutil provides centralized timezone handling for the application.
// Call LoadProcessLocation once during initialization to set the canonical
// timezone, then use Now() and Location() for business times and database
// values instead of time.Now() and time.Local. time.Now stays right for what
// no zone affects: durations, deadlines and Unix timestamps.
package timeutil

import (
	"sync"
	"time"
)

var (
	mu   sync.RWMutex
	loc  *time.Location
	name string
)

// LoadLocation loads the timezone by name (e.g., "Asia/Shanghai", "UTC").
// Must be called once at startup before any other function in this package.
// An unknown name is an error and leaves the current timezone in place.
func LoadLocation(tzName string) error {
	mu.Lock()
	defer mu.Unlock()

	l, err := time.LoadLocation(tzName)
	if err != nil {
		return err
	}
	loc = l
	name = tzName
	return nil
}

// LoadProcessLocation loads the timezone like LoadLocation and makes it the
// process's local timezone as well, so time.Now, time.Unix and every library
// reading time.Local (GORM's automatic timestamps among them) keep a single
// clock with Now. It matters for PostgreSQL: a zone-less timestamp column
// stores the written value's wall clock, so values written in two zones stop
// comparing. Call it once at startup, before other goroutines read the time.
func LoadProcessLocation(tzName string) error {
	if err := LoadLocation(tzName); err != nil {
		return err
	}
	time.Local = Location()
	return nil
}

// Location returns the canonical application timezone.
// Falls back to time.Local if LoadLocation was never called.
func Location() *time.Location {
	mu.RLock()
	defer mu.RUnlock()
	if loc == nil {
		return time.Local
	}
	return loc
}

// LocationName returns the configured timezone name, or "Local" when
// LoadLocation was never called.
func LocationName() string {
	mu.RLock()
	defer mu.RUnlock()
	if name == "" {
		return "Local"
	}
	return name
}

// Now returns the current time in the application timezone.
// Falls back to time.Now() if LoadLocation was never called.
func Now() time.Time {
	return time.Now().In(Location())
}
