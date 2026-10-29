package schema

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/perfect-panel/server/pkg/orm"
	"gorm.io/gorm"
)

func TestMigrateMySQL(t *testing.T) {
	dsn := os.Getenv("PPANEL_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set PPANEL_TEST_MYSQL_DSN to run MySQL/MariaDB migration test")
	}
	runMigration(t, orm.DriverMySQL, dsn)
}

func TestMigratePostgres(t *testing.T) {
	dsn := os.Getenv("PPANEL_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set PPANEL_TEST_POSTGRES_DSN to run PostgreSQL migration test")
	}
	runMigration(t, orm.DriverPostgres, dsn)
}

func runMigration(t *testing.T, driver, dsn string) {
	t.Helper()
	err := Up(driver, dsn)
	if err != nil && !errors.Is(err, NoChange) {
		t.Fatalf("%s migration failed: %v", driver, err)
	}
	cfg := orm.ParseDSN(dsn)
	if cfg == nil {
		t.Fatalf("%s dsn parse failed", driver)
	}
	cfg.Driver = orm.NormalizeDriver(driver)
	db, err := orm.ConnectDatabase(orm.Mysql{Config: *cfg})
	if err != nil {
		t.Fatalf("%s connect failed: %v", driver, err)
	}
	sqlDB, err := db.DB()
	if err == nil {
		defer sqlDB.Close()
	}
	assertStructuredRouteRuleSchema(t, db)
	assertOrderedNodeRoutingSchema(t, db, driver)
	if err := CreateAdminUser(fmt.Sprintf("admin-%s@example.com", driver), "password", db); err != nil {
		t.Fatalf("%s create admin failed: %v", driver, err)
	}
	if err := CreateAdminUser("", "password", db); err != nil {
		t.Fatalf("%s existing admin must skip email validation: %v", driver, err)
	}
}

func assertStructuredRouteRuleSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	columns, err := db.Migrator().ColumnTypes("server_config_overrides")
	if err != nil {
		t.Fatalf("inspect server_config_overrides columns: %v", err)
	}
	byName := make(map[string]gorm.ColumnType, len(columns))
	for _, column := range columns {
		byName[strings.ToLower(column.Name())] = column
	}
	for _, name := range []string{"block_ips", "block_protocols"} {
		column, ok := byName[name]
		if !ok {
			t.Fatalf("server_config_overrides.%s is missing after migration", name)
		}
		if got := strings.ToUpper(column.DatabaseTypeName()); got != "TEXT" {
			t.Errorf("server_config_overrides.%s type = %s, want TEXT", name, got)
		}
		if nullable, known := column.Nullable(); !known || !nullable {
			t.Errorf("server_config_overrides.%s nullable = %v (known=%v), want true", name, nullable, known)
		}
	}
	var count int64
	if err := db.Table("system").Where(map[string]any{
		"category": "server",
		"key":      []string{"BlockIPs", "BlockProtocols"},
	}).Count(&count).Error; err != nil {
		t.Fatalf("query structured route global config keys: %v", err)
	}
	if count != 2 {
		t.Fatalf("structured route global config key count = %d, want 2", count)
	}
}

func assertOrderedNodeRoutingSchema(t *testing.T, db *gorm.DB, driver string) {
	t.Helper()
	columns, err := db.Migrator().ColumnTypes("server_config_overrides")
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]gorm.ColumnType, len(columns))
	for _, column := range columns {
		byName[strings.ToLower(column.Name())] = column
	}
	routing, ok := byName["routing"]
	if !ok {
		t.Fatal("server_config_overrides.routing is missing")
	}
	wantType := "TEXT"
	if driver == orm.DriverMySQL {
		wantType = "MEDIUMTEXT"
	}
	if got := strings.ToUpper(routing.DatabaseTypeName()); got != wantType {
		t.Errorf("routing type = %s, want %s", got, wantType)
	}
	if nullable, known := routing.Nullable(); !known || !nullable {
		t.Errorf("routing must be nullable: nullable=%v known=%v", nullable, known)
	}
	revision, ok := byName["routing_revision"]
	if !ok {
		t.Fatal("routing_revision is missing")
	}
	revisionType := strings.ToUpper(revision.DatabaseTypeName())
	if revisionType != "BIGINT" && !(driver == orm.DriverPostgres && revisionType == "INT8") {
		t.Fatalf("routing_revision type = %s, want BIGINT", revisionType)
	}
	if nullable, known := revision.Nullable(); !known || nullable {
		t.Errorf("routing_revision must be non-null: nullable=%v known=%v", nullable, known)
	}
	var rows []struct{ Key, Value string }
	if err := db.Table("system").Where(map[string]any{"category": "server", "key": []string{"Routing", "RoutingRevision", "RoutingCutover"}}).Find(&rows).Error; err != nil {
		t.Fatalf("query ordered global keys: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("ordered global key count = %d, want 3", len(rows))
	}
	if driver == orm.DriverMySQL {
		systemColumns, err := db.Migrator().ColumnTypes("system")
		if err != nil {
			t.Fatal(err)
		}
		for _, col := range systemColumns {
			if strings.EqualFold(col.Name(), "value") && strings.ToUpper(col.DatabaseTypeName()) != "MEDIUMTEXT" {
				t.Errorf("system.value type = %s, want MEDIUMTEXT", col.DatabaseTypeName())
			}
		}
	}
}

type fakeMigrationRunner struct {
	upErr       error
	sourceErr   error
	databaseErr error
	closed      bool
}

func (f *fakeMigrationRunner) Up() error { return f.upErr }

func (f *fakeMigrationRunner) Close() (error, error) {
	f.closed = true
	return f.sourceErr, f.databaseErr
}

func TestUpAndCloseAlwaysClosesMigrationRunner(t *testing.T) {
	upFailure := errors.New("up failed")
	tests := []struct {
		name  string
		upErr error
	}{
		{name: "success"},
		{name: "no change", upErr: NoChange},
		{name: "migration failure", upErr: upFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeMigrationRunner{upErr: tt.upErr}
			err := upAndClose(runner)

			if !runner.closed {
				t.Fatal("migration runner was not closed")
			}
			if !errors.Is(err, tt.upErr) {
				t.Fatalf("upAndClose() error = %v, want %v", err, tt.upErr)
			}
		})
	}
}

func TestUpAndCloseDoesNotHideCloseFailureBehindNoChange(t *testing.T) {
	closeFailure := errors.New("database close failed")
	runner := &fakeMigrationRunner{
		upErr:       NoChange,
		databaseErr: closeFailure,
	}

	err := upAndClose(runner)

	if !runner.closed {
		t.Fatal("migration runner was not closed")
	}
	if !errors.Is(err, closeFailure) {
		t.Fatalf("upAndClose() error = %v, want close failure", err)
	}
	if errors.Is(err, NoChange) {
		t.Fatalf("upAndClose() error = %v hides a close failure behind ErrNoChange", err)
	}
}

func TestDialectMigrationVersionsStayAligned(t *testing.T) {
	for _, direction := range []string{"up", "down"} {
		mysqlFiles := migrationNames(t, "database/mysql", direction)
		postgresFiles := migrationNames(t, "database/postgres", direction)
		if !reflect.DeepEqual(mysqlFiles, postgresFiles) {
			t.Fatalf("%s migration versions differ:\nmysql:    %v\npostgres: %v", direction, mysqlFiles, postgresFiles)
		}
	}
}

func migrationNames(t *testing.T, directory, direction string) []string {
	t.Helper()
	entries, err := sqlFiles.ReadDir(directory)
	if err != nil {
		t.Fatalf("read %s migrations: %v", directory, err)
	}
	suffix := "." + direction + ".sql"
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), suffix) {
			continue
		}
		base := strings.TrimSuffix(entry.Name(), suffix)
		version, _, ok := strings.Cut(base, "_")
		if !ok {
			t.Fatalf("migration %s/%s has no version prefix", directory, entry.Name())
		}
		names = append(names, version)
	}
	sort.Strings(names)
	return names
}

func TestPostgresOnlineIndexMigrationsContainOneConcurrentStatement(t *testing.T) {
	entries, err := sqlFiles.ReadDir("database/postgres")
	if err != nil {
		t.Fatalf("read postgres migrations: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name < "02155" || name > "02169_zzzz" {
			continue
		}
		data, err := sqlFiles.ReadFile(filepath.Join("database/postgres", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		sql := string(data)
		if !strings.Contains(sql, "CONCURRENTLY") {
			t.Errorf("%s must build or drop its index concurrently", name)
		}
		if got := strings.Count(sql, ";"); got != 1 {
			t.Errorf("%s contains %d statements; online index migrations must contain exactly one", name, got)
		}
	}
}

func TestMySQLPostgresOnlyMigrationsRemainExecutableNoOps(t *testing.T) {
	entries, err := sqlFiles.ReadDir("database/mysql")
	if err != nil {
		t.Fatalf("read mysql migrations: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name < "02155" || name > "02172_zzzz" {
			continue
		}
		data, err := sqlFiles.ReadFile(filepath.Join("database/mysql", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !strings.Contains(string(data), "SELECT 1;") {
			t.Errorf("%s must be an executable no-op, not an empty/comment-only query", name)
		}
	}
}

func TestPostgresMySQLOnlyMigrationsRemainExecutableNoOps(t *testing.T) {
	entries, err := sqlFiles.ReadDir("database/postgres")
	if err != nil {
		t.Fatalf("read postgres migrations: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name < "02173" || name > "02178_zzzz" {
			continue
		}
		data, err := sqlFiles.ReadFile(filepath.Join("database/postgres", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !strings.Contains(string(data), "SELECT 1;") {
			t.Errorf("%s must be an executable no-op, not an empty/comment-only query", name)
		}
	}
}

func TestMySQLTaskScopeGeneratedColumnUsesInstantDDL(t *testing.T) {
	for _, direction := range []string{"up", "down"} {
		name := "02177_mysql_task_scope_generated." + direction + ".sql"
		data, err := sqlFiles.ReadFile(filepath.Join("database/mysql", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		sql := string(data)
		if !strings.Contains(sql, "ALGORITHM=INSTANT") {
			t.Errorf("%s must add or drop the virtual column without rebuilding the table", name)
		}
		if strings.Contains(sql, "LOCK=") {
			t.Errorf("%s must not combine ALGORITHM=INSTANT with a LOCK clause", name)
		}
		if got := strings.Count(sql, ";"); got != 1 {
			t.Errorf("%s contains %d statements, want 1", name, got)
		}
	}
}

func TestMySQLTaskScopeIndexUsesPortableOnlineDDL(t *testing.T) {
	for _, direction := range []string{"up", "down"} {
		name := "02178_mysql_task_scope_index." + direction + ".sql"
		data, err := sqlFiles.ReadFile(filepath.Join("database/mysql", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		sql := string(data)
		// MySQL 8 can build this index INPLACE. MariaDB must choose its
		// online COPY path once a virtual generated column becomes indexed.
		if !strings.Contains(sql, "ALGORITHM=DEFAULT") || !strings.Contains(sql, "LOCK=NONE") {
			t.Errorf("%s must let each engine choose its compatible online algorithm", name)
		}
		if direction == "up" && (!strings.Contains(sql, "`created_at` DESC") || !strings.Contains(sql, "`id` DESC")) {
			t.Errorf("%s must match newest-first task pagination", name)
		}
		if got := strings.Count(sql, ";"); got != 1 {
			t.Errorf("%s contains %d statements, want 1", name, got)
		}
	}
}

func TestMySQLHotIndexMigrationsUseOnlineDDL(t *testing.T) {
	entries, err := sqlFiles.ReadDir("database/mysql")
	if err != nil {
		t.Fatalf("read mysql migrations: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name < "02173" || name > "02176_zzzz" {
			continue
		}
		data, err := sqlFiles.ReadFile(filepath.Join("database/mysql", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		sql := string(data)
		if !strings.Contains(sql, "ALGORITHM=INPLACE") || !strings.Contains(sql, "LOCK=NONE") {
			t.Errorf("%s must request online InnoDB DDL", name)
		}
		if got := strings.Count(sql, ";"); got != 1 {
			t.Errorf("%s contains %d statements; each table optimization must be atomic", name, got)
		}
	}
}

func TestStructuredRouteRuleMigrationIsAdditiveForBothDialects(t *testing.T) {
	for _, dialect := range []string{"mysql", "postgres"} {
		upName := filepath.Join("database", dialect, "02179_structured_route_rules.up.sql")
		upData, err := sqlFiles.ReadFile(upName)
		if err != nil {
			t.Fatalf("read %s: %v", upName, err)
		}
		up := strings.ToLower(string(upData))
		for _, fragment := range []string{"alter table", "server_config_overrides", "block_ips", "block_protocols", "text", "blockips", "blockprotocols"} {
			if !strings.Contains(up, fragment) {
				t.Errorf("%s does not contain %q", upName, fragment)
			}
		}
		if strings.Contains(up, "not null") {
			t.Errorf("%s must add nullable override columns", upName)
		}

		downName := filepath.Join("database", dialect, "02179_structured_route_rules.down.sql")
		downData, err := sqlFiles.ReadFile(downName)
		if err != nil {
			t.Fatalf("read %s: %v", downName, err)
		}
		down := strings.ToLower(string(downData))
		if !strings.Contains(down, "drop column") || !strings.Contains(down, "block_ips") || !strings.Contains(down, "block_protocols") {
			t.Errorf("%s must remove both additive columns", downName)
		}
		if !strings.Contains(down, "destructive manual down") || !strings.Contains(down, "never invokes this rollback automatically") {
			t.Errorf("%s must document its destructive operator-only prerequisite", downName)
		}
	}
}
