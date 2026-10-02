package ui

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	transport "github.com/ealink1/navi-fyne/internal/infra/shell"
)

type shellFiles struct {
	pane         *shellPane
	remote       *transport.Remote
	directory    *widget.Entry
	list         *widget.List
	files        []transport.File
	selected     int
	busy         bool
	status       *widget.Label
	content      *fyne.Container
	cancel       context.CancelFunc
	cancelButton *widget.Button
}

func (p *shellPane) showFiles() {
	remote, ok := p.session.(*transport.Remote)
	if !ok || p.ended {
		p.status.SetText("SFTP 需要已连接的 SSH 会话")
		return
	}
	if p.filePane == nil {
		p.filePane = newShellFiles(p, remote)
	}
	p.aux.Objects = []fyne.CanvasObject{container.NewGridWrap(fyne.NewSize(410, 560), p.filePane.content)}
	p.aux.Show()
	p.aux.Refresh()
	p.workspace.content.Refresh()
}
func newShellFiles(p *shellPane, remote *transport.Remote) *shellFiles {
	f := &shellFiles{pane: p, remote: remote, selected: -1, directory: widget.NewEntry(), status: widget.NewLabel("SFTP")}
	f.status.Wrapping = fyne.TextWrapWord
	f.directory.SetText(".")
	f.list = widget.NewList(func() int { return len(f.files) }, func() fyne.CanvasObject { return widget.NewLabel("文件名\n大小 / 权限") }, func(id widget.ListItemID, object fyne.CanvasObject) {
		file := f.files[id]
		label := file.Name
		if file.Directory {
			label = "▸ " + label
		}
		object.(*widget.Label).SetText(fmt.Sprintf("%s\n%d B · %s", label, file.Size, file.Mode))
	})
	f.list.OnSelected = func(id widget.ListItemID) { f.selected = id }
	f.directory.OnSubmitted = func(string) { f.refresh() }
	actions := shellColumns(6, shellOutlined(shellButton("进入", "", false, f.enter)), shellOutlined(shellButton("上级", "", false, func() { f.directory.SetText(path.Join(f.directory.Text, "..")); f.refresh() })), shellOutlined(shellButton("刷新", "refresh-cw", false, f.refresh)), shellOutlined(shellButton("上传", "", false, f.upload)), shellOutlined(shellButton("下载", "", false, f.download)))
	f.cancelButton = shellButton("取消传输", "x", false, func() {
		if f.cancel != nil {
			f.cancel()
			f.status.SetText("正在取消…")
		}
	})
	f.cancelButton.Disable()
	header := shellVBox(shellText("远程文件 · SFTP", 14, true, shellTextColor), shellFixed(shellLine(), 0, 1), shellField("目录", f.directory), shellFixed(actions, 0, 30))
	f.content = shellInset(shellBorder(header, shellVBox(shellLabel(f.status, 12), shellOutlined(f.cancelButton)), nil, nil, f.list), 12)
	f.refresh()
	return f
}
func (f *shellFiles) refresh() {
	if f.busy || f.pane.closed || f.pane.ended {
		return
	}
	f.busy = true
	f.status.SetText("读取目录…")
	directory := f.directory.Text
	f.pane.workspace.owner.jobs.run(func(context.Context) (any, error) {
		ctx, cancel := context.WithTimeout(f.pane.ctx, 30*time.Second)
		defer cancel()
		return f.remote.Files(ctx, directory)
	}, func(value any, err error) {
		f.busy = false
		if f.pane.closed {
			return
		}
		if err != nil {
			f.status.SetText(err.Error())
			return
		}
		f.files = value.([]transport.File)
		sort.Slice(f.files, func(i, j int) bool {
			a, b := f.files[i], f.files[j]
			if a.Directory != b.Directory {
				return a.Directory
			}
			return a.Name < b.Name
		})
		f.selected = -1
		f.list.UnselectAll()
		f.list.Refresh()
		f.status.SetText(fmt.Sprintf("%d 项", len(f.files)))
	})
}
func (f *shellFiles) enter() {
	if f.selected >= 0 && f.selected < len(f.files) && f.files[f.selected].Directory {
		f.directory.SetText(path.Join(f.directory.Text, f.files[f.selected].Name))
		f.refresh()
	}
}
func (f *shellFiles) upload() {
	if f.busy || f.pane.ended {
		return
	}
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			f.status.SetText(err.Error())
			return
		}
		if reader == nil {
			return
		}
		source := reader.URI().Path()
		reader.Close()
		destination := path.Join(f.directory.Text, filepath.Base(source))
		f.transfer(func(ctx context.Context, progress func(int64)) error {
			return f.remote.Upload(ctx, source, destination, progress)
		})
	}, f.pane.workspace.owner.Window)
}
func (f *shellFiles) download() {
	if f.busy || f.pane.ended || f.selected < 0 || f.selected >= len(f.files) || f.files[f.selected].Directory {
		return
	}
	file := f.files[f.selected]
	if filepath.Base(file.Name) != file.Name || file.Name == "." || file.Name == ".." {
		f.status.SetText("远程文件名称无效")
		return
	}
	source := path.Join(f.directory.Text, file.Name)
	dialog.ShowFolderOpen(func(folder fyne.ListableURI, err error) {
		if err != nil {
			f.status.SetText(err.Error())
			return
		}
		if folder == nil {
			return
		}
		destination := filepath.Join(folder.Path(), file.Name)
		f.transfer(func(ctx context.Context, progress func(int64)) error {
			return f.remote.Download(ctx, source, destination, progress)
		})
	}, f.pane.workspace.owner.Window)
}
func (f *shellFiles) transfer(work func(context.Context, func(int64)) error) {
	if f.busy || f.pane.closed {
		return
	}
	f.busy = true
	f.status.SetText("正在传输…已有同名文件时会停止。")
	ctx, cancel := context.WithCancel(f.pane.ctx)
	f.cancel = cancel
	f.cancelButton.Enable()
	f.pane.workspace.owner.jobs.run(func(context.Context) (any, error) {
		defer cancel()
		var last time.Time
		progress := func(size int64) {
			if time.Since(last) < 250*time.Millisecond {
				return
			}
			last = time.Now()
			f.pane.workspace.owner.jobs.dispatch(func() {
				if !f.pane.closed && f.busy {
					f.status.SetText(fmt.Sprintf("已传输 %.1f MiB", float64(size)/(1<<20)))
				}
			})
		}
		err := work(ctx, progress)
		if err != nil && ctx.Err() != nil {
			err = errors.Join(err, ctx.Err())
		}
		return nil, err
	}, func(_ any, err error) {
		f.busy = false
		f.cancel = nil
		f.cancelButton.Disable()
		if f.pane.closed {
			return
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				f.status.SetText("传输已取消：" + err.Error())
				return
			}
			f.status.SetText("传输失败：" + err.Error())
			return
		}
		f.refresh()
	})
}
