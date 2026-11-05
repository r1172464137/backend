package orm

import (
	"net/url"
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
)

// GORM reads PostgreSQL timestamp columns in the DSN's zone only when the DSN
// names one; custom parameters that leave it out used to read every stored
// time back labelled UTC. The zone is now added like application_name.
func TestPostgresDSNAddsTheSessionZoneToCustomParameters(t *testing.T) {
	for _, tc := range []struct {
		name     string
		params   string
		location string
		want     string
	}{
		{"parameters without a zone", "sslmode=require", "", DefaultLocation},
		{"parameters without a zone in a location", "sslmode=require", "Europe/Paris", "Europe/Paris"},
		{"explicit zone kept", "sslmode=require&TimeZone=UTC", "Europe/Paris", "UTC"},
		{"lower-case zone key kept", "sslmode=require&timezone=Asia/Tokyo", "", "Asia/Tokyo"},
		{"underscore zone key kept", "sslmode=require&time_zone=Asia/Tokyo", "", "Asia/Tokyo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := Mysql{Config: Config{Driver: DriverPostgres, Addr: "db:5432", Dbname: "ppanel", Username: "u", Password: "p", Config: tc.params}, Location: tc.location}
			dsn := m.Dsn()
			parsed, err := url.Parse(dsn)
			if err != nil {
				t.Fatalf("parse %q: %v", dsn, err)
			}
			query := parsed.Query()
			zones := 0
			for _, key := range postgresTimeZoneKeys {
				zones += len(query[key])
			}
			if zones != 1 {
				t.Fatalf("DSN %q names the zone %d times, want once", dsn, zones)
			}
			if got := postgresTimeZone(query); got != tc.want {
				t.Fatalf("DSN %q session zone = %q, want %q", dsn, got, tc.want)
			}
			if query.Get("sslmode") != "require" {
				t.Fatalf("DSN %q lost the custom sslmode", dsn)
			}
			// GORM extracts the zone from the raw DSN, where the slash must
			// stay visible.
			if strings.Contains(dsn, "%2F") {
				t.Fatalf("DSN %q escapes the zone's slash, which GORM reads literally", dsn)
			}
			if got := m.SessionLocation(); got != tc.want {
				t.Fatalf("SessionLocation = %q, want the DSN's zone %q", got, tc.want)
			}
		})
	}
}

// The default PostgreSQL parameters encrypt the connection when the server
// offers TLS and still connect to a server without it.
func TestDefaultPostgresQueryPrefersTLS(t *testing.T) {
	query, err := url.ParseQuery(DefaultPostgresQuery(""))
	if err != nil {
		t.Fatal(err)
	}
	if got := query.Get("sslmode"); got != "prefer" {
		t.Fatalf("default sslmode = %q, want prefer", got)
	}
}

// golang-migrate's mysql driver parses the DSN with the MySQL driver and then
// URL-unescapes the user name and password, so both must be escaped in the
// migration DSN or a password with %, + or @ authenticates as something else.
// The check replays that sequence.
func TestMigrationDsnEscapesMySQLCredentials(t *testing.T) {
	for _, tc := range []struct{ user, password string }{
		{"root", "Ab+cd9%2F"},
		{"pp@nel", "p@ss/word?x=1&y=2"},
		{"user name", "100% sure+more"},
		{"plain", "plain"},
		{"", ""},
	} {
		t.Run(tc.password, func(t *testing.T) {
			m := Mysql{Config: Config{Driver: DriverMySQL, Addr: "db:3306", Dbname: "ppanel", Username: tc.user, Password: tc.password, Config: "charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai"}}
			dsn := m.MigrationDsn()
			if strings.Contains(dsn, "://") {
				t.Fatalf("migration DSN %q carries a scheme; the migration adds it", dsn)
			}
			cfg, err := mysqldriver.ParseDSN(dsn)
			if err != nil {
				t.Fatalf("the MySQL driver rejects the migration DSN %q: %v", dsn, err)
			}
			user, err := url.QueryUnescape(cfg.User)
			if err != nil {
				t.Fatal(err)
			}
			password, err := url.QueryUnescape(cfg.Passwd)
			if err != nil {
				t.Fatal(err)
			}
			if user != tc.user || password != tc.password {
				t.Fatalf("golang-migrate would connect as %q/%q, want %q/%q (DSN %q)", user, password, tc.user, tc.password, dsn)
			}
			if cfg.Addr != "db:3306" || cfg.DBName != "ppanel" || cfg.Loc == nil || cfg.Loc.String() != DefaultLocation {
				t.Fatalf("migration DSN %q lost the address, database or parameters: %+v", dsn, cfg)
			}
			// The application's own connection keeps the raw credentials.
			if appCfg, err := mysqldriver.ParseDSN(m.Dsn()); err != nil || appCfg.User != tc.user || appCfg.Passwd != tc.password {
				t.Fatalf("application DSN %q parses to %+v (%v), want the raw credentials", m.Dsn(), appCfg, err)
			}
		})
	}
	// PostgreSQL DSNs are URLs already, with escaped credentials.
	pg := Mysql{Config: Config{Driver: DriverPostgres, Addr: "db:5432", Dbname: "ppanel", Username: "u", Password: "p@ss/word"}}
	if pg.MigrationDsn() != pg.Dsn() || !strings.Contains(pg.MigrationDsn(), "p%40ss%2Fword@") {
		t.Fatalf("PostgreSQL migration DSN = %q, want the application URL", pg.MigrationDsn())
	}
}
