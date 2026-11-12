package orm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/perfect-panel/server/pkg/logger"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

const (
	DriverMySQL     = "mysql"
	DriverPostgres  = "postgres"
	DriverPostgres2 = "postgresql"

	DefaultMySQLConfig             = "charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai&interpolateParams=true"
	legacyDefaultMySQLConfig       = "charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai"
	DefaultPostgresConfig          = "sslmode=disable&TimeZone=Asia/Shanghai&application_name=perfect-panel"
	DefaultSlowThresholdMs         = 1000
	DefaultConnMaxLifetimeSeconds  = 1800
	DefaultConnMaxIdleTimeSeconds  = 300
	defaultPostgresApplicationName = "perfect-panel"
)

type Config struct {
	Driver          string `yaml:"Driver" default:"mysql"`
	Addr            string `yaml:"Addr"`
	Username        string `yaml:"Username"`
	Password        string `yaml:"Password"`
	Dbname          string `yaml:"Dbname"`
	Config          string `yaml:"Config" default:"charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai&interpolateParams=true"`
	MaxIdleConns    int    `yaml:"MaxIdleConns" default:"10"`
	MaxOpenConns    int    `yaml:"MaxOpenConns" default:"10"`
	ConnMaxLifetime int64  `yaml:"ConnMaxLifetime" default:"1800"`
	ConnMaxIdleTime int64  `yaml:"ConnMaxIdleTime" default:"300"`
	SlowThreshold   int64  `yaml:"SlowThreshold" default:"1000"`
}

type Mysql struct {
	Config Config
}

func NormalizeDriver(driver string) string {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "", DriverMySQL:
		return DriverMySQL
	case DriverPostgres, DriverPostgres2, "pgsql":
		return DriverPostgres
	default:
		return strings.ToLower(strings.TrimSpace(driver))
	}
}

func (m Mysql) Driver() string {
	return NormalizeDriver(m.Config.Driver)
}

func (m Mysql) Dsn() string {
	switch m.Driver() {
	case DriverPostgres:
		return m.postgresDsn()
	default:
		return m.mysqlDsn()
	}
}

// MigrationDsn URL-escapes MySQL credentials: golang-migrate's mysql driver
// unescapes them, so a password with %, + or @ would change (9ce3bf78).
func (m Mysql) MigrationDsn() string {
	if m.Driver() != DriverMySQL {
		return m.Dsn()
	}
	escaped := m
	escaped.Config.Username = url.QueryEscape(m.Config.Username)
	escaped.Config.Password = url.QueryEscape(m.Config.Password)
	return escaped.mysqlDsn()
}

func (m Mysql) mysqlDsn() string {
	query := m.Config.Config
	if query == "" {
		query = DefaultMySQLConfig
	} else {
		query = withDefaultMySQLParams(query)
	}
	return m.Config.Username + ":" + m.Config.Password + "@tcp(" + m.Config.Addr + ")/" + m.Config.Dbname + "?" + query
}

// withDefaultMySQLParams enables client-side placeholder interpolation only
// for the UTF-8 connection settings the application supports by default. It
// saves a prepare/execute/close round trip for ordinary GORM queries, while an
// explicit false value and custom legacy multibyte character sets are left
// untouched.
func withDefaultMySQLParams(query string) string {
	params, err := url.ParseQuery(query)
	if err != nil {
		return query
	}
	if _, configured := params["interpolateParams"]; configured {
		return query
	}
	charset := strings.ToLower(params.Get("charset"))
	collation := strings.ToLower(params.Get("collation"))
	if charset != "utf8mb4" && charset != "utf8mb4,utf8" {
		return query
	}
	if collation != "" && !strings.HasPrefix(collation, "utf8mb4_") && !strings.HasPrefix(collation, "utf8_") {
		return query
	}
	params.Set("interpolateParams", "true")
	return params.Encode()
}

func (m Mysql) postgresDsn() string {
	query := m.Config.Config
	if query == "" || query == DefaultMySQLConfig || query == legacyDefaultMySQLConfig {
		query = DefaultPostgresConfig
	}
	u := url.URL{
		Scheme: DriverPostgres,
		Host:   m.Config.Addr,
		Path:   "/" + m.Config.Dbname,
	}
	if m.Config.Username != "" {
		u.User = url.UserPassword(m.Config.Username, m.Config.Password)
	}
	params, err := url.ParseQuery(query)
	if err != nil {
		u.RawQuery = query
	} else {
		if params.Get("application_name") == "" {
			params.Set("application_name", defaultPostgresApplicationName)
		}
		u.RawQuery = encodePostgresParams(params)
	}
	return u.String()
}

// encodePostgresParams keeps IANA time-zone separators visible in the final
// DSN. pgx decodes ordinary URL query values correctly, but GORM's PostgreSQL
// dialector also extracts TimeZone directly from the raw DSN with a regular
// expression. Leaving the slash as %2F therefore makes GORM ask both Go and
// PostgreSQL for a literal zone such as "Asia%2FShanghai".
//
// Only the slash in supported time-zone parameters is unescaped; all other
// parameter values remain URL encoded. This preserves credentials and custom
// settings while accepting both legacy Asia%2FShanghai configuration and the
// clearer Asia/Shanghai form.
func encodePostgresParams(params url.Values) string {
	query := params.Encode()
	for _, key := range []string{"TimeZone", "timezone", "time_zone"} {
		for _, value := range params[key] {
			encodedKey := url.QueryEscape(key)
			encodedValue := url.QueryEscape(value)
			visibleValue := strings.ReplaceAll(encodedValue, "%2F", "/")
			if visibleValue == encodedValue {
				continue
			}
			query = strings.Replace(query, encodedKey+"="+encodedValue, encodedKey+"="+visibleValue, 1)
		}
	}
	return query
}

func (m *Mysql) GetSlowThreshold() time.Duration {
	return time.Duration(m.Config.SlowThreshold) * time.Millisecond
}
func (m *Mysql) GetColorful() bool {
	return true
}

func ConnectMysql(m Mysql) (*gorm.DB, error) {
	return ConnectDatabase(m)
}

func ConnectDatabase(m Mysql) (*gorm.DB, error) {
	return OpenDatabase(context.Background(), m)
}

// OpenDatabase connects and verifies the database within ctx. Every network
// step - TCP dial, authentication handshake and the MySQL version probe - is
// bound to ctx; GORM itself is opened without any further I/O, so a server
// that accepts the TCP connection and then stops answering cannot hold the
// caller past its deadline. On error the pool is closed.
func OpenDatabase(ctx context.Context, m Mysql) (*gorm.DB, error) {
	if m.Config.Dbname == "" {
		return nil, errors.New("database name is empty")
	}
	gormConfig := &gorm.Config{
		Logger: &logger.GormLogger{SlowThreshold: m.GetSlowThreshold()},
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		DisableAutomaticPing: true,
	}
	switch m.Driver() {
	case DriverMySQL:
		return openMySQL(ctx, m, gormConfig)
	case DriverPostgres:
		db, err := gorm.Open(postgres.Open(m.Dsn()), gormConfig)
		if err != nil {
			return nil, err
		}
		sqldb, err := db.DB()
		if err != nil {
			return nil, err
		}
		applyPool(sqldb, m)
		if err := sqldb.PingContext(ctx); err != nil {
			_ = sqldb.Close()
			return nil, err
		}
		return db, nil
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", m.Config.Driver)
	}
}

func openMySQL(ctx context.Context, m Mysql, gormConfig *gorm.Config) (*gorm.DB, error) {
	dsn := m.Dsn()
	sqldb, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	applyPool(sqldb, m)
	fail := func(err error) (*gorm.DB, error) {
		_ = sqldb.Close()
		return nil, err
	}
	if err := sqldb.PingContext(ctx); err != nil {
		return fail(err)
	}
	var version string
	if err := sqldb.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		return fail(err)
	}
	cfg := mysqlDialectorConfig(version)
	cfg.Conn = sqldb
	cfg.DSN = dsn
	db, err := gorm.Open(mysql.New(cfg), gormConfig)
	if err != nil {
		return fail(err)
	}
	return db, nil
}

// mysqlDialectorConfig reproduces the feature flags gorm.io/driver/mysql
// derives from SELECT VERSION(), so the dialector can skip its own probe
// (which uses an unbounded context). RETURNING support for MariaDB 10.5+ is
// not enabled; GORM then uses LastInsertId, which is equivalent for inserts.
func mysqlDialectorConfig(version string) mysql.Config {
	cfg := mysql.Config{ServerVersion: version, SkipInitializeWithVersion: true}
	switch {
	case strings.Contains(version, "MariaDB"):
		cfg.DontSupportRenameIndex = true
		cfg.DontSupportRenameColumn = true
		cfg.DontSupportForShareClause = true
		cfg.DontSupportNullAsDefaultValue = true
	case strings.HasPrefix(version, "5.6."):
		cfg.DontSupportRenameIndex = true
		cfg.DontSupportRenameColumn = true
		cfg.DontSupportForShareClause = true
		cfg.DontSupportDropConstraint = true
	case strings.HasPrefix(version, "5.7."):
		cfg.DontSupportRenameColumn = true
		cfg.DontSupportForShareClause = true
		cfg.DontSupportDropConstraint = true
	case strings.HasPrefix(version, "5."):
		cfg.DisableDatetimePrecision = true
		cfg.DontSupportRenameIndex = true
		cfg.DontSupportRenameColumn = true
		cfg.DontSupportForShareClause = true
		cfg.DontSupportDropConstraint = true
	}
	if strings.Contains(version, "TiDB") {
		cfg.DontSupportRenameColumnUnique = true
	}
	return cfg
}

func applyPool(sqldb *sql.DB, m Mysql) {
	sqldb.SetMaxIdleConns(m.Config.MaxIdleConns)
	sqldb.SetMaxOpenConns(m.Config.MaxOpenConns)
	if m.Config.ConnMaxLifetime > 0 {
		sqldb.SetConnMaxLifetime(time.Duration(m.Config.ConnMaxLifetime) * time.Second)
	}
	if m.Config.ConnMaxIdleTime > 0 {
		sqldb.SetConnMaxIdleTime(time.Duration(m.Config.ConnMaxIdleTime) * time.Second)
	}
}

func Ping(dsn string) bool {
	return PingDatabase(DriverMySQL, dsn)
}

func PingDatabase(driver, dsn string) bool {
	var dialector gorm.Dialector
	switch NormalizeDriver(driver) {
	case DriverMySQL:
		dialector = mysql.Open(dsn)
	case DriverPostgres:
		dialector = postgres.Open(dsn)
	default:
		fmt.Printf("unsupported database driver: %s\n", driver)
		return false
	}
	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		fmt.Printf("connect database failed, err: %v\n", err.Error())
		return false
	}
	sqlDB, err := db.DB()
	if err != nil {
		fmt.Printf("get database connection failed, err: %v\n", err)
		return false
	}
	defer sqlDB.Close()
	return sqlDB.Ping() == nil
}
