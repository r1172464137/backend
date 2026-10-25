package schema

import (
	"strings"
	"testing"
)

// MySQL requires a CURRENT_TIMESTAMP default's fractional precision to match
// its DATETIME column. PostgreSQL accepts its own CURRENT_TIMESTAMP defaults;
// do not rewrite either dialect's already-applied migrations to fix this DDL.
func TestEntitlementMySQLDefaultsKeepMillisecondPrecision(t *testing.T) {
	const name = "database/mysql/03000_subscription_entitlements.up.sql"
	data, err := sqlFiles.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	defaults := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "DATETIME(3)") && strings.Contains(line, "DEFAULT CURRENT_TIMESTAMP") {
			defaults++
			if !strings.Contains(line, "DEFAULT CURRENT_TIMESTAMP(3)") {
				t.Errorf("%s has mismatched timestamp precision: %s", name, line)
			}
		}
	}
	if defaults != 4 {
		t.Fatalf("checked %d timestamp defaults, want all 4 entitlement defaults", defaults)
	}
}
