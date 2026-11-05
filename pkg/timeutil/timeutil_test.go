package timeutil

import (
	"testing"
	"time"
)

// restoreZone puts the package's and the process's timezones back when the
// test ends.
func restoreZone(t *testing.T) {
	t.Helper()
	mu.RLock()
	oldLoc, oldName := loc, name
	mu.RUnlock()
	oldLocal := time.Local
	t.Cleanup(func() {
		mu.Lock()
		loc, name = oldLoc, oldName
		mu.Unlock()
		time.Local = oldLocal
	})
}

func TestLoadLocationKeepsTheZoneOnAnUnknownName(t *testing.T) {
	restoreZone(t)
	if err := LoadLocation("Asia/Tokyo"); err != nil {
		t.Fatal(err)
	}
	if err := LoadLocation("Not/AZone"); err == nil {
		t.Fatal("an unknown zone was accepted")
	}
	if LocationName() != "Asia/Tokyo" || Location().String() != "Asia/Tokyo" || Now().Location().String() != "Asia/Tokyo" {
		t.Fatalf("zone = %s (%s), want Asia/Tokyo kept", LocationName(), Location())
	}
}

// The process zone follows the application zone, so time.Now and time.Unix
// read the same wall clock as Now.
func TestLoadProcessLocationSetsTheProcessZone(t *testing.T) {
	restoreZone(t)
	if err := LoadProcessLocation("America/New_York"); err != nil {
		t.Fatal(err)
	}
	if time.Local.String() != "America/New_York" || time.Unix(0, 0).Location().String() != "America/New_York" {
		t.Fatalf("process zone = %s, want America/New_York", time.Local)
	}
	now := Now()
	if local := time.Now(); local.Hour() != now.Hour() && local.Sub(now) < time.Minute {
		t.Fatalf("time.Now() = %v and Now() = %v read different wall clocks", local, now)
	}

	if err := LoadProcessLocation("Not/AZone"); err == nil {
		t.Fatal("an unknown zone was accepted")
	}
	if time.Local.String() != "America/New_York" {
		t.Fatalf("a failed load changed the process zone to %s", time.Local)
	}
}
