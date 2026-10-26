package mysql2postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// A strict driver script exercises the production SQL/reset path locally. Real
// serial/identity behavior remains covered separately by the PostgreSQL tests.
type sequenceStep struct {
	match    string
	args     []driver.Value
	values   [][]driver.Value
	err      error
	closeErr error
	exec     bool
}

type sequenceScript struct {
	steps []sequenceStep
	next  int
}

func (s *sequenceScript) take(query string, args []driver.NamedValue, exec bool) (sequenceStep, error) {
	if s.next >= len(s.steps) {
		return sequenceStep{}, fmt.Errorf("unexpected SQL: %s", query)
	}
	step := s.steps[s.next]
	s.next++
	var values []driver.Value
	for _, arg := range args {
		values = append(values, arg.Value)
	}
	if step.exec != exec || !strings.Contains(query, step.match) || !reflect.DeepEqual(values, step.args) {
		return sequenceStep{}, fmt.Errorf("unexpected SQL: %s args=%v; want %s args=%v exec=%t", query, values, step.match, step.args, step.exec)
	}
	return step, step.err
}

func (s *sequenceScript) Connect(context.Context) (driver.Conn, error) {
	return &sequenceConn{script: s}, nil
}
func (*sequenceScript) Driver() driver.Driver { return nil }

type sequenceConn struct{ script *sequenceScript }

func (*sequenceConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (*sequenceConn) Close() error { return nil }
func (*sequenceConn) Begin() (driver.Tx, error) {
	return nil, errors.New("unexpected transaction")
}
func (c *sequenceConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	step, err := c.script.take(query, args, false)
	if err != nil {
		return nil, err
	}
	return &sequenceRows{step: step}, nil
}
func (c *sequenceConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if _, err := c.script.take(query, args, true); err != nil {
		return nil, err
	}
	return driver.RowsAffected(1), nil
}

type sequenceRows struct {
	step sequenceStep
	next int
}

func (r *sequenceRows) Columns() []string {
	if strings.Contains(r.step.match, "information_schema.columns") {
		return []string{"table_name", "column_name"}
	}
	return []string{"value"}
}
func (r *sequenceRows) Close() error { return r.step.closeErr }
func (r *sequenceRows) Next(dest []driver.Value) error {
	if r.next == len(r.step.values) {
		return io.EOF
	}
	copy(dest, r.step.values[r.next])
	r.next++
	return nil
}

func sequenceTestDB(t *testing.T, steps []sequenceStep) *sql.DB {
	t.Helper()
	script := &sequenceScript{steps: steps}
	db := sql.OpenDB(script)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
		if script.next != len(script.steps) {
			t.Errorf("executed %d SQL steps, want %d", script.next, len(script.steps))
		}
	})
	return db
}

func sequenceResetSteps(maxID int64) []sequenceStep {
	steps := []sequenceStep{
		{match: "FROM information_schema.columns", args: []driver.Value{"public"}, values: [][]driver.Value{{"selected", "id"}}},
		{match: "SELECT pg_get_serial_sequence($1, $2)", args: []driver.Value{`"public"."selected"`, "id"}, values: [][]driver.Value{{"public.selected_id_seq"}}},
		{match: `SELECT COALESCE(MAX("id"), 0) FROM "public"."selected"`, values: [][]driver.Value{{maxID}}},
	}
	if maxID > 0 {
		return append(steps, sequenceStep{exec: true, match: "SELECT setval($1::regclass, $2, true)", args: []driver.Value{"public.selected_id_seq", maxID}})
	}
	return append(steps, sequenceStep{exec: true, match: "SELECT setval($1::regclass, 1, false)", args: []driver.Value{"public.selected_id_seq"}})
}

func TestResetSequencesOnlyTouchesCompletedPlan(t *testing.T) {
	steps := sequenceResetSteps(42)
	// The unselected relation can disappear while another package applies its
	// schema migrations. Looking up its sequence would fail with SQLSTATE 42P01.
	steps[0].values = [][]driver.Value{{"email_task", "id"}, {"selected", "id"}, {"excluded", "id"}}
	db := sequenceTestDB(t, steps)
	if err := resetSequences(context.Background(), db, "public", []tablePlan{{Name: "selected"}}); err != nil {
		t.Fatal(err)
	}
}

func TestResetSequencesNoPlanDoesNothing(t *testing.T) {
	// No database access at all; an empty plan must never mean the whole schema.
	if err := resetSequences(context.Background(), nil, "public", nil); err != nil {
		t.Fatal(err)
	}
}

func TestResetSequencesNoPlannedSequenceDoesNotLookUpOtherTables(t *testing.T) {
	db := sequenceTestDB(t, []sequenceStep{{
		match: "FROM information_schema.columns", args: []driver.Value{"public"},
		values: [][]driver.Value{{"email_task", "id"}},
	}})
	if err := resetSequences(context.Background(), db, "public", []tablePlan{{Name: "plain_id"}}); err != nil {
		t.Fatal(err)
	}
}

func TestResetSequencesEmptyAndUnowned(t *testing.T) {
	t.Run("empty table restarts at one", func(t *testing.T) {
		db := sequenceTestDB(t, sequenceResetSteps(0))
		if err := resetSequences(context.Background(), db, "public", []tablePlan{{Name: "selected"}}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("no owned sequence is a no-op", func(t *testing.T) {
		steps := sequenceResetSteps(42)[:2]
		steps[1].values = [][]driver.Value{{nil}}
		db := sequenceTestDB(t, steps)
		if err := resetSequences(context.Background(), db, "public", []tablePlan{{Name: "selected"}}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestResetSequencesPreservesErrors(t *testing.T) {
	failure := errors.New("sequence failure")
	missingSelected := &pgconn.PgError{Code: "42P01", Message: "selected relation does not exist"}
	for _, tc := range []struct {
		name string
		step int
		err  error
		max  int64
		want string
	}{
		{"catalog", 0, failure, 42, "list postgres sequences"},
		{"selected relation missing", 1, missingSelected, 42, "get sequence for selected.id"},
		{"maximum", 2, failure, 42, "get max id for selected.id"},
		{"align populated sequence", 3, failure, 42, "set sequence for selected.id"},
		{"reset empty sequence", 3, failure, 0, "reset empty sequence for selected.id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			steps := sequenceResetSteps(tc.max)[:tc.step+1]
			steps[tc.step].err = tc.err
			db := sequenceTestDB(t, steps)
			err := resetSequences(context.Background(), db, "public", []tablePlan{{Name: "selected"}})
			if !errors.Is(err, tc.err) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("resetSequences = %v, want wrapped %v (%s)", err, tc.err, tc.want)
			}
		})
	}
}

func TestResetSequencesPreservesCatalogCloseError(t *testing.T) {
	steps := sequenceResetSteps(42)[:1]
	steps[0].closeErr = errCloseFailed
	db := sequenceTestDB(t, steps)
	if err := resetSequences(context.Background(), db, "public", []tablePlan{{Name: "selected"}}); !errors.Is(err, errCloseFailed) {
		t.Fatalf("resetSequences = %v, want catalog close failure", err)
	}
}
