package orm

import (
	"net/url"
	"os"
	"testing"
	"time"
)

type zoneProbeRow struct {
	ID int64     `gorm:"primaryKey"`
	At time.Time `gorm:"type:timestamp(3)"`
}

// A PostgreSQL timestamp column written in the application's zone must read
// back as the same instant, also through connection parameters that name no
// TimeZone: GORM registers the zone-aware timestamp codec from the DSN's
// zone, and without one the stored wall clock came back labelled UTC, hours
// off. Set PPANEL_TEST_POSTGRES_DSN to run it.
func TestPostgresTimestampRoundTripsWithCustomParameters(t *testing.T) {
	dsn := os.Getenv("PPANEL_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set PPANEL_TEST_POSTGRES_DSN to run the PostgreSQL time-zone round trip")
	}
	cfg := ParseDSN(dsn)
	if cfg == nil {
		t.Fatalf("parse PostgreSQL test DSN %q", dsn)
	}
	params, err := url.ParseQuery(cfg.Config)
	if err != nil {
		t.Fatal(err)
	}
	// Custom parameters without a zone, the configuration the audit measured
	// an eight-hour skew with.
	for _, key := range postgresTimeZoneKeys {
		params.Del(key)
	}
	cfg.Config = params.Encode()
	if cfg.Config == "" {
		cfg.Config = "sslmode=disable"
	}

	zone, err := time.LoadLocation(DefaultLocation)
	if err != nil {
		t.Fatal(err)
	}
	db, err := ConnectDatabase(Mysql{Config: *cfg})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Migrator().DropTable(&zoneProbeRow{}); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&zoneProbeRow{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Migrator().DropTable(&zoneProbeRow{}) })

	written := time.Date(2026, 9, 28, 20, 0, 0, 0, zone)
	if err := db.Create(&zoneProbeRow{ID: 1, At: written}).Error; err != nil {
		t.Fatal(err)
	}
	var read zoneProbeRow
	if err := db.First(&read, 1).Error; err != nil {
		t.Fatal(err)
	}
	if skew := read.At.Sub(written); skew != 0 {
		t.Fatalf("timestamp read back as %s, written %s: skew %s", read.At, written, skew)
	}
	// The SQL side compares in the same zone: the row is in the past there
	// too. (The application's naming strategy keeps table names singular.)
	var expired int64
	if err := db.Raw("SELECT count(*) FROM zone_probe_row WHERE at < ?", written.Add(time.Hour)).Scan(&expired).Error; err != nil {
		t.Fatal(err)
	}
	if expired != 1 {
		t.Fatalf("SQL-side comparison found %d expired rows, want 1", expired)
	}
	var session string
	if err := sqlDB.QueryRow("SHOW timezone").Scan(&session); err != nil {
		t.Fatal(err)
	}
	if session != DefaultLocation {
		t.Fatalf("session time zone = %q, want %s from the DSN", session, DefaultLocation)
	}
}
