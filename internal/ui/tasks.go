package ui

import (
	"context"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"
)

// tasks tracks every UI worker. Shutdown cancels work and joins it before closing
// clients and storage. A late completion cannot update a closed application.
type tasks struct {
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	workers  atomic.Int64
	closing  atomic.Bool
	dispatch func(func())
}

func newTasks(executors ...func(func())) *tasks {
	ctx, cancel := context.WithCancel(context.Background())
	dispatch := fyne.Do
	if len(executors) > 0 && executors[0] != nil {
		dispatch = executors[0]
	}
	return &tasks{ctx: ctx, cancel: cancel, dispatch: dispatch}
}
func (t *tasks) run(work func(context.Context) (any, error), done func(any, error)) context.CancelFunc {
	return t.runWithCancel(nil, work, done)
}

// Assign the cancellation handle before work can complete. A completion may
// immediately start another operation (e.g. scope discovery then object loading),
// and the original caller must not overwrite that new operation's handle.
func (t *tasks) runWithCancel(handle *context.CancelFunc, work func(context.Context) (any, error), done func(any, error)) context.CancelFunc {
	ctx, cancel := context.WithCancel(t.ctx)
	if handle != nil {
		*handle = cancel
	}
	if t.closing.Load() {
		cancel()
		return cancel
	}
	t.wg.Add(1)
	t.workers.Add(1)
	go func() {
		defer t.workers.Add(-1)
		defer t.wg.Done()
		defer cancel()
		value, err := work(ctx)
		if !t.closing.Load() {
			t.dispatch(func() {
				if !t.closing.Load() {
					done(value, err)
				}
			})
		}
	}()
	return cancel
}
func (t *tasks) stop() { t.closing.Store(true); t.cancel(); t.wg.Wait() }
