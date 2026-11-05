package mapping

import (
	"testing"
	"time"
)

// The DTO mapping policy: a time.Time becomes int64 Unix milliseconds,
// slices are copied rather than shared, and zero source fields are copied
// too, clearing the destination's.
func TestCopyPreservesDTOMappingPolicy(t *testing.T) {
	source := struct {
		CreatedAt time.Time
		Enabled   bool
		Labels    []string
	}{
		CreatedAt: time.UnixMilli(1700000000123), Labels: []string{"one"},
	}
	target := struct {
		CreatedAt int64
		Enabled   bool
		Labels    []string
	}{Enabled: true}
	if err := Copy(&target, &source); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if target.CreatedAt != 1700000000123 || target.Enabled || len(target.Labels) != 1 {
		t.Fatalf("mapping policy changed: %+v", target)
	}
	target.Labels[0] = "changed"
	if source.Labels[0] != "one" {
		t.Fatal("deep copy retained slice alias")
	}
}

// Copy reports a copy it cannot make instead of leaving the destination
// silently half-filled.
func TestCopyReportsMismatchedTypes(t *testing.T) {
	type source struct{ Name string }
	type target struct{ Name string }
	var dst target
	if err := Copy(&dst, &source{Name: "x"}); err != nil || dst.Name != "x" {
		t.Fatalf("Copy = %+v, %v; want the name copied", dst, err)
	}
	if err := Copy(dst, &source{Name: "y"}); err == nil {
		t.Fatal("copying into a non-pointer was not reported")
	}
}
