package bootstrap

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/perfect-panel/server/internal/app/migration/schema"
	"github.com/perfect-panel/server/pkg/orm"
)

// routingSchemaVersion is the only schema version this build runs on. The
// routing format 3 migration changes data only; it adds no DDL.
const routingSchemaVersion = 2180

// schemaVersion is strictly read-only: it never creates schema_migrations.
func schemaVersion(ctx context.Context, cfg orm.Config) (uint64, bool, error) {
	version, exists, _, err := schemaVersionState(ctx, cfg)
	return version, exists, err
}

// schemaVersionState may recover a dirty first migration only after proving
// that no business objects exist. It performs no DDL or metadata writes.
func schemaVersionState(ctx context.Context, cfg orm.Config) (uint64, bool, bool, error) {
	db, err := orm.ConnectDatabase(orm.Mysql{Config: cfg})
	if err != nil {
		return 0, false, false, err
	}
	pool, err := db.DB()
	if err != nil {
		return 0, false, false, err
	}
	defer pool.Close()
	var count int64
	if orm.NormalizeDriver(cfg.Driver) == orm.DriverMySQL {
		err = db.WithContext(ctx).Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", "schema_migrations").Row().Scan(&count)
	} else {
		err = db.WithContext(ctx).Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = ?", "schema_migrations").Row().Scan(&count)
	}
	if err != nil {
		return 0, false, false, err
	}
	if count == 0 {
		if err := schema.RequireNoBusinessObjects(ctx, db, false); err != nil {
			return 0, false, false, err
		}
		return 0, false, false, nil
	}
	var rows int64
	if err := db.WithContext(ctx).Raw("SELECT COUNT(*) FROM schema_migrations").Row().Scan(&rows); err != nil {
		return 0, false, false, err
	}
	if rows == 0 {
		if err := schema.RequireNoBusinessObjects(ctx, db, true); err != nil {
			return 0, false, false, err
		}
		return 0, false, false, nil
	}
	if rows != 1 {
		return 0, false, false, fmt.Errorf("invalid migration version table")
	}
	var version int64
	var dirty bool
	err = db.WithContext(ctx).Raw("SELECT version, dirty FROM schema_migrations").Row().Scan(&version, &dirty)
	if err == sql.ErrNoRows {
		return 0, false, false, fmt.Errorf("migration version disappeared")
	}
	if err != nil {
		return 0, false, false, err
	}
	if dirty {
		if version == 1 {
			if err := schema.RequireNoBusinessObjects(ctx, db, true); err != nil {
				return 0, false, false, fmt.Errorf("dirty first migration has existing objects: %w", err)
			}
			return 0, false, true, nil
		}
		return 0, false, false, fmt.Errorf("database migration is dirty")
	}
	if version < 0 {
		return 0, false, false, fmt.Errorf("invalid migration version")
	}
	return uint64(version), true, false, nil
}
