package schema

import (
	"path/filepath"
	"strings"
	"testing"
)

// These static checks complement (not replace) real MySQL/PostgreSQL migration
// tests. TestMigrateMySQL/Postgres inspect the applied schema when configured.
func TestOrderedNodeRoutingMigrationShapeAndDownGuard(t *testing.T) {
	for _, dialect := range []string{"mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			read := func(direction string) string {
				body, err := sqlFiles.ReadFile(filepath.Join("database", dialect, "02180_ordered_node_routing."+direction+".sql"))
				if err != nil {
					t.Fatal(err)
				}
				return strings.ToLower(string(body))
			}
			up, down := read("up"), read("down")
			for _, fragment := range []string{"server_config_overrides", "routing_revision", "routingrevision", "'routing'", "default 0"} {
				if !strings.Contains(up, fragment) {
					t.Errorf("up missing %q", fragment)
				}
			}
			if strings.Contains(up, "drop column") || strings.Contains(up, "delete from") {
				t.Error("up must retain legacy data")
			}
			if dialect == "mysql" && strings.Count(up, "mediumtext") != 2 {
				t.Error("mysql must widen system.value and store routing in MEDIUMTEXT")
			}
			if dialect == "postgres" && !strings.Contains(up, `"routing" text null`) {
				t.Error("postgres routing must use nullable TEXT")
			}
			firstDDL := strings.Index(down, "\nalter table")
			if firstDDL < 0 {
				t.Fatal("down has no DDL")
			}
			guard := down[:firstDDL]
			for _, fragment := range []string{"stop all writers", "server_config_overrides", "routing", "is not null", "exists"} {
				if !strings.Contains(guard, fragment) {
					t.Errorf("down pre-DDL guard missing %q", fragment)
				}
			}
			if dialect == "mysql" {
				for _, fragment := range []string{"octet_length(`value`) > 65535", "octet_length(`value`) <> 0", "prepare ppanel_routing_down_check"} {
					if !strings.Contains(guard, fragment) {
						t.Errorf("mysql down guard missing %q", fragment)
					}
				}
			} else if !strings.Contains(guard, "raise exception") {
				t.Error("postgres down must raise before DDL")
			}
		})
	}
}
