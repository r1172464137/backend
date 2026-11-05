package supporttest

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
)

var faults atomic.Int64

// Refuse makes the database refuse the statements of kind op on table with
// err once the first skip of them succeeded, until the test ends or the
// returned function lifts the refusal, so a flow's failure paths run against
// the real repositories. op is the GORM callback kind: "create", "query",
// "update", "delete" or "row" (a scanned select). A refused statement is not
// sent to the database.
func (e *Env) Refuse(t testing.TB, op, table string, skip int, err error) (lift func()) {
	t.Helper()
	callbacks := e.DB.Callback()
	processor := callbacks.Create()
	switch op {
	case "create":
	case "query":
		processor = callbacks.Query()
	case "update":
		processor = callbacks.Update()
	case "delete":
		processor = callbacks.Delete()
	case "row":
		processor = callbacks.Row()
	default:
		t.Fatalf("unknown statement kind %q", op)
	}
	name := fmt.Sprintf("supporttest:refuse:%d", faults.Add(1))
	passed := 0
	refuse := func(db *gorm.DB) {
		if db.Statement.Table != table {
			return
		}
		if passed < skip {
			passed++
			return
		}
		_ = db.AddError(err)
	}
	if err := processor.Before("gorm:"+op).Register(name, refuse); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	lift = func() { once.Do(func() { _ = processor.Remove(name) }) }
	t.Cleanup(lift)
	return lift
}
