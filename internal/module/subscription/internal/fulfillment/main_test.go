package fulfillment

import (
	"os"
	"testing"
	"time"
)

// TestMain runs the package on UTC, the zone of the test databases'
// sessions, as the server runs on its AppLocation: PostgreSQL's zone-less
// timestamps keep the wall clock written, so the writer's zone and the
// session's must agree for times to round-trip.
func TestMain(m *testing.M) {
	time.Local = time.UTC
	os.Exit(m.Run())
}
