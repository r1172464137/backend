package lifecycle

import (
	"testing"
)

// A stop-only service has its place in the stop order, nothing to run: its
// Start returns at once and Stop runs the func. StopOrder shows the reverse
// of the add order, which is what Stop follows.
func TestWithStopAndStopOrder(t *testing.T) {
	var order []string
	group := NewServiceGroup()
	group.Add(WithStop(func() { order = append(order, "flush") }))
	worker := &mockedService{quit: make(chan struct{}), result: newProduct(), multiplier: 1}
	group.Add(worker)
	group.Add(WithStop(func() { order = append(order, "http") }))

	stopOrder := group.StopOrder()
	if len(stopOrder) != 3 || stopOrder[1] != worker {
		t.Fatalf("StopOrder = %v, want the three services with the worker in the middle", stopOrder)
	}
	if _, ok := stopOrder[0].(stopOnlyService); !ok {
		t.Fatalf("StopOrder[0] = %T, want the last-added stop-only service first", stopOrder[0])
	}

	started := make(chan struct{})
	go func() {
		group.Start()
		close(started)
	}()
	<-worker.result.started
	group.Stop()
	<-started

	if len(order) != 2 || order[0] != "http" || order[1] != "flush" {
		t.Fatalf("stop order = %v, want http before flush", order)
	}
}
