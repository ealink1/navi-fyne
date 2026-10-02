package ui

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestTaskSignalWatchCoalescesRefreshAndJoinsOnShutdown(t *testing.T) {
	queue := make(chan func(), 8) // Small bounded dispatch fixture; only one callback should be queued.
	tasks := newTasks(func(update func()) { queue <- update })
	t.Cleanup(tasks.stop)
	signals := make(chan struct{})
	var latest, observed atomic.Int64
	tasks.watch(signals, func() { observed.Store(latest.Load()) })
	for i := int64(1); i <= 100; i++ {
		latest.Store(i)
		select {
		case signals <- struct{}{}:
		case <-time.After(time.Second):
			t.Fatal("signal listener stalled behind a queued refresh")
		}
	}
	if len(queue) != 1 || tasks.workers.Load() != 0 {
		t.Fatal("listener causes unbounded refreshes or permanently pending I/O")
	}
	(<-queue)()
	if observed.Load() != 100 {
		t.Fatal("callback used obsolete notification state")
	}
	latest.Store(101)
	select {
	case signals <- struct{}{}:
	case <-time.After(time.Second):
		t.Fatal("watch did not schedule a later refresh")
	}
	var final func()
	select {
	case final = <-queue:
	case <-time.After(time.Second):
		t.Fatal("next callback missing")
	}
	tasks.stop()
	final()
	if observed.Load() != 100 {
		t.Fatal("late refresh mutated a closed window")
	}
}
