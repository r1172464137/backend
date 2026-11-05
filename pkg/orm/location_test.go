package orm

import (
	"testing"
	"time"
)

// loc=Local names the process zone, which the server sets to AppLocation at
// startup, so the session zone is that zone's name and the startup
// comparison with AppLocation holds instead of reporting a false mismatch.
func TestSessionLocationResolvesLocalToTheProcessZone(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	previous := time.Local
	time.Local = paris
	t.Cleanup(func() { time.Local = previous })

	m := Mysql{Config: Config{Driver: DriverMySQL, Config: "charset=utf8mb4&parseTime=true&loc=Local"}}
	if got := m.SessionLocation(); got != "Europe/Paris" {
		t.Fatalf("SessionLocation = %q, want the process zone Europe/Paris", got)
	}
}

// Only a zone both drivers accept may reach the connection parameters:
// PostgreSQL rejects "Local", and a misspelt zone would fail every connect.
func TestLocationOrDefaultKeepsOnlyUsableZones(t *testing.T) {
	for in, want := range map[string]string{
		"":             DefaultLocation,
		"Local":        DefaultLocation,
		"Mars/Olympus": DefaultLocation,
		"UTC":          "UTC",
		"Europe/Paris": "Europe/Paris",
	} {
		if got := locationOrDefault(in); got != want {
			t.Errorf("locationOrDefault(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSessionLocation(t *testing.T) {
	for _, tc := range []struct {
		name string
		m    Mysql
		want string
	}{
		{"mysql defaults", Mysql{Config: Config{Driver: DriverMySQL}}, DefaultLocation},
		{"mysql defaults in a location", Mysql{Config: Config{Driver: DriverMySQL}, Location: "Europe/Paris"}, "Europe/Paris"},
		{"mysql explicit loc", Mysql{Config: Config{Driver: DriverMySQL, Config: "charset=utf8mb4&parseTime=true&loc=Asia%2FTokyo"}}, "Asia/Tokyo"},
		{"mysql parameters without loc use the driver's UTC", Mysql{Config: Config{Driver: DriverMySQL, Config: "charset=utf8mb4&parseTime=true"}}, "UTC"},
		{"postgres defaults", Mysql{Config: Config{Driver: DriverPostgres}}, DefaultLocation},
		{"postgres explicit zone", Mysql{Config: Config{Driver: DriverPostgres, Config: "sslmode=disable&TimeZone=UTC"}}, "UTC"},
		{"postgres lower-case zone key", Mysql{Config: Config{Driver: DriverPostgres, Config: "sslmode=disable&timezone=Europe/Paris"}}, "Europe/Paris"},
		// Custom parameters without a zone get Location's zone in the DSN.
		{"postgres parameters without a zone", Mysql{Config: Config{Driver: DriverPostgres, Config: "sslmode=disable"}}, DefaultLocation},
		{"postgres parameters without a zone in a location", Mysql{Config: Config{Driver: DriverPostgres, Config: "sslmode=require"}, Location: "Europe/Paris"}, "Europe/Paris"},
		{"postgres parameters that do not parse", Mysql{Config: Config{Driver: DriverPostgres, Config: "sslmode=%zz"}}, ""},
	} {
		if got := tc.m.SessionLocation(); got != tc.want {
			t.Errorf("%s: SessionLocation = %q, want %q", tc.name, got, tc.want)
		}
	}
}
