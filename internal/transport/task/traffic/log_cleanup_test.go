package traffic

import (
	"context"
	"errors"
	"testing"
)

// shellModule records the module calls of the traffic task shells.
type shellModule struct {
	cleanups, stats int
	err             error
}

var (
	_ LogCleaner   = (*shellModule)(nil)
	_ StatRecorder = (*shellModule)(nil)
)

func (m *shellModule) CleanupLogs(context.Context) error {
	m.cleanups++
	return m.err
}

func (m *shellModule) RecordDailyTrafficStatistics(context.Context) error {
	m.stats++
	return m.err
}

// The shells run their module once per task and return its failure, so
// asynq retries the task.
func TestTrafficTaskShellsRunTheirModule(t *testing.T) {
	module := &shellModule{}
	if err := NewLogCleanupHandler(module).ProcessTask(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if err := NewStatHandler(module).ProcessTask(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if module.cleanups != 1 || module.stats != 1 {
		t.Fatalf("module calls = %d cleanups, %d stats; want one each", module.cleanups, module.stats)
	}

	module.err = errors.New("module failed")
	if err := NewLogCleanupHandler(module).ProcessTask(context.Background(), nil); !errors.Is(err, module.err) {
		t.Fatalf("cleanup error = %v, want the module's", err)
	}
	if err := NewStatHandler(module).ProcessTask(context.Background(), nil); !errors.Is(err, module.err) {
		t.Fatalf("stat error = %v, want the module's", err)
	}
}
