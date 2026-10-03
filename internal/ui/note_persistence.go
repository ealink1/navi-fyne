package ui

import (
	"context"
	"time"

	"github.com/ealink1/navi-fyne/internal/application"
)

// A single timer and a single notification coalesce typing bursts. Both workers
// are joined by the window's task supervisor, including event-loop shutdown.
func (n *noteWorkspace) startAutosave() {
	n.edits = make(chan struct{}, 1)
	ready := make(chan struct{}, 1)
	jobs := n.owner.jobs
	edits := n.edits
	jobs.watch(ready, func() { n.renderPreview(); n.filter(); n.save() })
	jobs.wg.Add(1)
	go func() {
		defer jobs.wg.Done()
		defer close(ready)
		timer := time.NewTimer(time.Hour)
		if !timer.Stop() {
			<-timer.C
		}
		defer timer.Stop()
		var clock <-chan time.Time
		for {
			select {
			case <-jobs.ctx.Done():
				return
			case <-edits:
				if !timer.Stop() && clock != nil {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(750 * time.Millisecond)
				clock = timer.C
			case <-clock:
				clock = nil
				select {
				case ready <- struct{}{}:
				default:
				}
			}
		}
	}()
}
func (n *noteWorkspace) save() {
	if !n.loaded || n.saving || n.savedRevision == n.editRevision || n.owner.shuttingDown {
		return
	}
	snapshot := n.book.Clone()
	snapshot.Revision = max(snapshot.Revision, n.service.CommittedRevision())
	version := n.editRevision
	n.saving = true
	n.status.SetText("正在加密保存…")
	n.saveButton.Disable()
	n.owner.jobs.run(func(ctx context.Context) (any, error) { return n.service.Save(ctx, snapshot) }, func(value any, err error) {
		n.saving = false
		if err != nil {
			n.status.SetText("保存失败，编辑仍保留：" + err.Error())
			n.saveButton.Enable()
			return
		}
		result := value.(application.NoteSave)
		n.book.Revision = result.Book.Revision
		n.savedRevision = version
		n.status.SetText("已保存于本机 · " + time.Now().Format("15:04:05"))
		if result.CleanupError != nil {
			n.status.SetText("内容已保存；旧加密快照清理失败")
		}
		if n.editRevision != version {
			n.save()
		}
	})
}

// The closure owns the UI snapshot. Joining workers before calling it lets us
// incorporate only this editor's already committed, undelivered save revision.
func (w *Window) noteShutdownSnapshot() func(context.Context) error {
	if w.switcher == nil || w.switcher.note == nil {
		return func(context.Context) error { return nil }
	}
	n := w.switcher.note
	if !n.loaded || n.editRevision == n.savedRevision && !n.saving {
		return func(context.Context) error { return nil }
	}
	snapshot := n.book.Clone()
	return func(ctx context.Context) error {
		snapshot.Revision = max(snapshot.Revision, n.service.CommittedRevision())
		_, err := n.service.Save(ctx, snapshot)
		return err
	}
}
func (w *Window) restartNoteAutosave() {
	if w.switcher.note != nil {
		w.switcher.note.saving = false
		w.switcher.note.startAutosave()
	}
}
