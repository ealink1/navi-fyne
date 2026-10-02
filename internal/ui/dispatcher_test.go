package ui

import (
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
)

var testUIQueues sync.Map

// Fyne's test driver executes Do inline on the worker rather than dispatching
// to its UI loop. Replay callbacks on the test goroutine, as the native driver
// does, so asynchronous fixtures and rendering obey the same thread contract.
func newTestWindow(t *testing.T, app fyne.App, deps Dependencies) *Window {
	t.Helper()
	queue := make(chan func(), 1024)
	deps.dispatch = func(callback func()) { queue <- callback }
	w := New(app, deps)
	testUIQueues.Store(w, queue)
	t.Cleanup(func() { testUIQueues.Delete(w) })
	return w
}

func waitUI(t *testing.T, w *Window) {
	t.Helper()
	value, ok := testUIQueues.Load(w)
	if !ok {
		t.Fatal("use newTestWindow for asynchronous UI tests")
	}
	queue := value.(chan func())
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		if w.jobs.workers.Load() == 0 {
			select {
			case callback := <-queue:
				callback()
				continue
			default:
				return
			}
		}
		select {
		case callback := <-queue:
			callback()
		case <-time.After(time.Millisecond):
		case <-deadline.C:
			t.Fatal("UI workers or callbacks did not finish")
		}
	}
}

func waitReady(t *testing.T, w *Window) {
	t.Helper()
	waitUI(t, w)
	select {
	case <-w.Ready():
	default:
		t.Fatal("window did not become ready")
	}
}
