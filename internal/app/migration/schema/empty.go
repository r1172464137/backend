package schema

import (
	"context"
	"fmt"
	"strings"

	"github.com/perfect-panel/server/pkg/orm"
	"gorm.io/gorm"
)

// RequireNoBusinessObjects proves the selected schema is empty. When
// allowMigrationTable is true, only the migration engine's bookkeeping table
// may exist; callers must separately validate its exact contents.
func RequireNoBusinessObjects(ctx context.Context, db *gorm.DB, allowMigrationTable bool) error {
	if db == nil {
		return fmt.Errorf("database is unavailable")
	}
	var objects []string
	switch orm.NormalizeDriver(db.Dialector.Name()) {
	case orm.DriverMySQL:
		if err := requireMySQLCatalogVisibility(ctx, db); err != nil {
			return err
		}
		if err := db.WithContext(ctx).Raw("SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE()").Scan(&objects).Error; err != nil {
			return fmt.Errorf("inspect database objects: %w", err)
		}
		for _, query := range []string{
			"SELECT COUNT(*) FROM information_schema.routines WHERE routine_schema = DATABASE()",
			"SELECT COUNT(*) FROM information_schema.triggers WHERE trigger_schema = DATABASE()",
			"SELECT COUNT(*) FROM information_schema.events WHERE event_schema = DATABASE()",
		} {
			var count int64
			if err := db.WithContext(ctx).Raw(query).Row().Scan(&count); err != nil {
				return fmt.Errorf("inspect database objects: %w", err)
			}
			if count != 0 {
				return fmt.Errorf("database contains existing objects")
			}
		}
	case orm.DriverPostgres:
		// Inspect the entire database, not merely search_path's first schema.
		// A second tenant schema must never be mistaken for an empty database.
		var otherSchemas int64
		if err := db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM pg_catalog.pg_namespace WHERE nspname NOT IN ('pg_catalog','information_schema','public',current_schema()) AND nspname NOT LIKE 'pg_toast%' AND nspname NOT LIKE 'pg_temp_%'`).Row().Scan(&otherSchemas); err != nil {
			return fmt.Errorf("inspect database schemas: %w", err)
		}
		if otherSchemas != 0 {
			return fmt.Errorf("database contains additional schemas")
		}
		if err := db.WithContext(ctx).Raw(`SELECT CASE WHEN n.nspname=current_schema() THEN c.relname ELSE n.nspname||'.'||c.relname END FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%' AND n.nspname NOT LIKE 'pg_temp_%' AND c.relkind IN ('r','p','v','m','S','f','c')`).Scan(&objects).Error; err != nil {
			return fmt.Errorf("inspect database objects: %w", err)
		}
		for _, query := range []string{
			`SELECT COUNT(*) FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%' AND n.nspname NOT LIKE 'pg_temp_%'`,
			`SELECT COUNT(*) FROM pg_catalog.pg_type t JOIN pg_catalog.pg_namespace n ON n.oid=t.typnamespace WHERE n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%' AND n.nspname NOT LIKE 'pg_temp_%' AND t.typtype IN ('e','d','r','m')`,
		} {
			var count int64
			if err := db.WithContext(ctx).Raw(query).Row().Scan(&count); err != nil {
				return fmt.Errorf("inspect database objects: %w", err)
			}
			if count != 0 {
				return fmt.Errorf("database contains existing objects")
			}
		}
	default:
		return fmt.Errorf("unsupported database dialect")
	}
	for _, name := range objects {
		if allowMigrationTable && name == "schema_migrations" {
			continue
		}
		return fmt.Errorf("database contains existing objects")
	}
	return nil
}

// information_schema hides objects from accounts without privileges. A full
// schema-level catalog view is required before claiming that MySQL is empty.
func requireMySQLCatalogVisibility(ctx context.Context, db *gorm.DB) error {
	var current string
	if err := db.WithContext(ctx).Raw("SELECT CURRENT_USER()").Row().Scan(&current); err != nil {
		return fmt.Errorf("inspect database privileges: %w", err)
	}
	i := strings.LastIndex(current, "@")
	if i <= 0 || i == len(current)-1 {
		return fmt.Errorf("cannot prove database catalog visibility")
	}
	quote := func(s string) string { return strings.ReplaceAll(s, "'", "''") }
	grantee := "'" + quote(current[:i]) + "'@'" + quote(current[i+1:]) + "'"
	var privileges []string
	query := `SELECT PRIVILEGE_TYPE FROM information_schema.USER_PRIVILEGES WHERE GRANTEE = ? UNION SELECT PRIVILEGE_TYPE FROM information_schema.SCHEMA_PRIVILEGES WHERE GRANTEE = ? AND TABLE_SCHEMA = DATABASE()`
	if err := db.WithContext(ctx).Raw(query, grantee, grantee).Scan(&privileges).Error; err != nil {
		return fmt.Errorf("inspect database privileges: %w", err)
	}
	known := map[string]bool{}
	for _, p := range privileges {
		known[strings.ToUpper(p)] = true
	}
	for _, p := range []string{"SELECT", "SHOW VIEW", "TRIGGER", "EVENT", "EXECUTE"} {
		if !known[p] {
			return fmt.Errorf("cannot prove database catalog visibility: missing %s privilege", p)
		}
	}
	return nil
}
