package mysql2postgres

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"log"
	"strings"
	"testing"
)

var errCloseFailed = errors.New("close failed")

type failingCloser struct{ err error }

func (c failingCloser) Close() error { return c.err }

// Close failures used to be dropped; they now join whatever the function
// returns.
func TestCloseRowsJoinsCloseFailures(t *testing.T) {
	var err error
	closeRows(failingCloser{errCloseFailed}, &err)
	if !errors.Is(err, errCloseFailed) {
		t.Fatalf("err = %v, want the close failure", err)
	}

	earlier := errors.New("scan failed")
	err = earlier
	closeRows(failingCloser{errCloseFailed}, &err)
	if !errors.Is(err, earlier) || !errors.Is(err, errCloseFailed) {
		t.Fatalf("err = %v, want both the earlier error and the close failure", err)
	}

	err = nil
	closeRows(failingCloser{}, &err)
	if err != nil {
		t.Fatalf("a clean close reported %v", err)
	}
}

// The connection pools close after every table committed, so a failure is
// logged instead of failing a completed migration.
func TestCloseDatabaseLogsFailures(t *testing.T) {
	var out bytes.Buffer
	previous, flags := log.Writer(), log.Flags()
	log.SetOutput(&out)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(previous)
		log.SetFlags(flags)
	})

	closeDatabase("mysql", failingCloser{errCloseFailed})

	if got := out.String(); !strings.Contains(got, "close mysql connection") || !strings.Contains(got, errCloseFailed.Error()) {
		t.Fatalf("log = %q, want the close failure", got)
	}
}

// A listing that stops early (here on a Scan error) closes its rows itself,
// and the driver's close failure reaches the caller next to the scan error.
func TestListingReportsRowsCloseFailure(t *testing.T) {
	db := sql.OpenDB(closeFailingConnector{})
	t.Cleanup(func() { _ = db.Close() })

	_, err := listMySQLTables(context.Background(), db)

	if !errors.Is(err, errCloseFailed) {
		t.Fatalf("listMySQLTables() error = %v, want the rows close failure", err)
	}
	if !strings.Contains(err.Error(), "destination arguments") {
		t.Fatalf("listMySQLTables() error = %v, want the scan error kept", err)
	}
}

// closeFailingConnector serves one row of two columns, which a one-column
// Scan rejects, from rows whose Close fails.
type closeFailingConnector struct{}

func (closeFailingConnector) Connect(context.Context) (driver.Conn, error) { return fakeConn{}, nil }
func (closeFailingConnector) Driver() driver.Driver                        { return nil }

type fakeConn struct{}

func (fakeConn) Prepare(string) (driver.Stmt, error) { return fakeStmt{}, nil }
func (fakeConn) Close() error                        { return nil }
func (fakeConn) Begin() (driver.Tx, error)           { return nil, errors.New("not supported") }

type fakeStmt struct{}

func (fakeStmt) Close() error                               { return nil }
func (fakeStmt) NumInput() int                              { return -1 }
func (fakeStmt) Exec([]driver.Value) (driver.Result, error) { return nil, errors.New("not supported") }
func (fakeStmt) Query([]driver.Value) (driver.Rows, error)  { return &fakeRows{}, nil }

type fakeRows struct{ served bool }

func (*fakeRows) Columns() []string { return []string{"table_name", "extra"} }
func (*fakeRows) Close() error      { return errCloseFailed }
func (r *fakeRows) Next(dest []driver.Value) error {
	if r.served {
		return io.EOF
	}
	r.served = true
	dest[0], dest[1] = "user", "unexpected"
	return nil
}
