package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/perfect-panel/server/internal/config"

	"github.com/perfect-panel/server/internal/app/migration/schema"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/pkg/orm"
)

const orderedMigrationVersion = 2180

// schemaVersion is strictly read-only: it never creates schema_migrations.
func schemaVersion(ctx context.Context, cfg orm.Config) (uint64, bool, error) {
	version, exists, _, err := schemaVersionState(ctx, cfg)
	return version, exists, err
}

// A dirty first version can be reset only in a provably empty business schema.
func schemaVersionState(ctx context.Context, cfg orm.Config) (v uint64, present, recover bool, resultErr error) {
	db, err := orm.ConnectDatabase(orm.Mysql{Config: cfg})
	if err != nil {
		return 0, false, false, err
	}
	pool, err := db.DB()
	if err != nil {
		return 0, false, false, err
	}
	defer func() { resultErr = errors.Join(resultErr, pool.Close()) }()
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
		return 0, false, false, schema.RequireNoBusinessObjects(ctx, db, false)
	}
	var rows int64
	if err := db.WithContext(ctx).Raw("SELECT COUNT(*) FROM schema_migrations").Row().Scan(&rows); err != nil {
		return 0, false, false, err
	}
	if rows == 0 {
		return 0, false, false, schema.RequireNoBusinessObjects(ctx, db, true)
	}
	if rows != 1 {
		return 0, false, false, errors.New("invalid migration version table")
	}
	var version int64
	var dirty bool
	err = db.WithContext(ctx).Raw("SELECT version, dirty FROM schema_migrations").Row().Scan(&version, &dirty)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, false, errors.New("migration version disappeared")
	}
	if err != nil {
		return 0, false, false, err
	}
	if dirty {
		if version != 1 {
			return 0, false, false, errors.New("database migration is dirty")
		}
		if err := schema.RequireNoBusinessObjects(ctx, db, true); err != nil {
			return 0, false, false, fmt.Errorf("dirty first migration has existing objects: %w", err)
		}
		return 0, false, true, nil
	}
	if version < 0 {
		return 0, false, false, errors.New("invalid migration version")
	}
	return uint64(version), true, false, nil
}

// Only actual embedded post-cutover versions are accepted; gaps and future
// versions may belong to a different migration history.
func knownOrderedVersion(v uint64) bool {
	switch v {
	case 2180, 3000, 3001, 3020, 3021, 3022, 3023, 3024:
		return true
	default:
		return false
	}
}

// Check retained legacy and routing columns even when there are no servers.
func checkOrderedSchema(ctx context.Context, cfg orm.Config) (resultErr error) {
	db, err := orm.ConnectDatabase(orm.Mysql{Config: cfg})
	if err != nil {
		return err
	}
	pool, err := db.DB()
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, pool.Close()) }()
	quote := `"`
	if orm.NormalizeDriver(cfg.Driver) == orm.DriverMySQL {
		quote = "`"
	}
	cols := []string{"server_id", "block", "block_ips", "block_protocols", "outbound", "dns", "routing", "routing_revision"}
	for i, c := range cols {
		cols[i] = quote + c + quote
	}
	rows, err := db.WithContext(ctx).Raw("SELECT " + strings.Join(cols, ",") + " FROM server_config_overrides WHERE 1=0").Rows()
	if err != nil {
		return fmt.Errorf("missing fork routing or legacy columns: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, rows.Close()) }()
	return rows.Err()
}

// Require unique fork records, and validate the policy when complete.
func orderedMarker(ctx context.Context, store network.CutoverStore) (string, error) {
	entries, err := store.System().GetNodeConfigUncached(ctx)
	if err != nil {
		return "", err
	}
	values := map[string]string{}
	for _, e := range entries {
		if e == nil || (e.Key != "RoutingCutover" && e.Key != "Routing" && e.Key != "RoutingRevision") {
			continue
		}
		if _, duplicate := values[e.Key]; duplicate {
			return "", errors.New("duplicate ordered routing setting")
		}
		values[e.Key] = e.Value
	}
	if len(values) != 3 {
		return "", errors.New("missing ordered routing settings; restore backup")
	}
	switch values["RoutingCutover"] {
	case "pending":
		return "pending", nil
	case "complete":
		if _, err := config.ParseNodeRouting([]byte(values["Routing"])); err != nil {
			return "", fmt.Errorf("invalid ordered routing policy: %w", err)
		}
		revision, err := strconv.ParseInt(values["RoutingRevision"], 10, 64)
		if err != nil || revision <= 0 {
			return "", errors.New("invalid ordered routing revision")
		}
		return "complete", nil
	default:
		return "", errors.New("missing or invalid ordered routing cutover marker; restore backup")
	}
}

// MigrateOrderedDatabase checks the effective legacy rules before DDL and only
// resumes an interrupted conversion whose durable marker is still pending.
func MigrateOrderedDatabase(ctx context.Context, cfg orm.Config, store network.CutoverStore) error {
	version, exists, recoverFirst, err := schemaVersionState(ctx, cfg)
	if err != nil {
		return err
	}
	if exists && version != orderedMigrationVersion-1 && !knownOrderedVersion(version) {
		return errors.New("unknown ordered routing migration version; restore backup or upgrade to 02179 first")
	}
	plan, err := preflightOrdered(ctx, cfg, store, version, exists)
	if err != nil {
		return err
	}
	db := orm.Mysql{Config: cfg}
	if recoverFirst {
		client, err := schema.Migrate(db.Driver(), db.MigrationDsn())
		if err != nil {
			return err
		}
		resetErr := client.Force(-1)
		sourceErr, databaseErr := client.Close()
		if err := errors.Join(resetErr, sourceErr, databaseErr); err != nil {
			return err
		}
	}
	changed := false
	if !exists || version == orderedMigrationVersion-1 {
		marker, didChange, err := migrateThroughCutover(ctx, cfg, store)
		if err != nil {
			return err
		}
		changed = didChange
		if marker == "complete" {
			plan = nil
		}
	}
	if plan != nil {
		if err := network.ApplyOrderedCutover(ctx, store, plan); err != nil {
			return err
		}
		changed = true
	}
	marker, err := orderedMarker(ctx, store)
	if err != nil {
		return err
	}
	if marker != "complete" {
		return errors.New("ordered routing cutover not complete")
	}
	err = schema.Up(db.Driver(), db.MigrationDsn())
	if err != nil && !errors.Is(err, schema.NoChange) {
		return err
	}
	if err == nil || changed {
		return nil
	}
	return err
}

// preflightOrdered refuses unproven schemas before any DDL.
func preflightOrdered(ctx context.Context, cfg orm.Config, store network.CutoverStore, version uint64, exists bool) (*network.OrderedCutoverPlan, error) {
	if !exists || version == orderedMigrationVersion-1 {
		return network.PreflightOrderedCutover(ctx, store, !exists)
	}
	if err := checkOrderedSchema(ctx, cfg); err != nil {
		return nil, err
	}
	marker, err := orderedMarker(ctx, store)
	if err != nil {
		return nil, err
	}
	if marker == "pending" {
		return network.PreflightOrderedCutover(ctx, store, false)
	}
	return nil, nil
}

// migrateThroughCutover never applies official migrations before the marker.
func migrateThroughCutover(ctx context.Context, cfg orm.Config, store network.CutoverStore) (string, bool, error) {
	db := orm.Mysql{Config: cfg}
	err := schema.UpThrough(db.Driver(), db.MigrationDsn(), orderedMigrationVersion)
	if err != nil && !errors.Is(err, schema.NoChange) {
		return "", false, err
	}
	changed := err == nil
	version, exists, err := schemaVersion(ctx, cfg)
	if err != nil {
		return "", false, err
	}
	if !exists || !knownOrderedVersion(version) {
		return "", false, errors.New("unexpected version after bounded migration")
	}
	if err = checkOrderedSchema(ctx, cfg); err != nil {
		return "", false, err
	}
	marker, err := orderedMarker(ctx, store)
	return marker, changed, err
}
