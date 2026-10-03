package ui

import (
	"fmt"
	"path"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	transport "github.com/ealink1/super-link/internal/infra/shell"
)

func newShellFilesView(f *shellFiles) *fyne.Container {
	f.search = widget.NewEntry()
	f.search.SetPlaceHolder("搜索文件…")
	f.search.OnChanged = func(string) { f.filterFiles() }
	f.list = widget.NewList(func() int { return len(f.shown) }, func() fyne.CanvasObject { return newShellFileRow() }, func(id widget.ListItemID, object fyne.CanvasObject) {
		object.(*shellFileRow).update(f.files[f.shown[id]])
	})
	f.list.OnSelected = func(id widget.ListItemID) { f.selected = f.shown[id] }
	f.directory.OnSubmitted = func(string) { f.refresh() }
	up := shellButtonView(shellButton("上级", "", false, func() { f.directory.SetText(path.Join(f.directory.Text, "..")); f.refresh() }))
	directory := shellFixed(shellBorder(nil, nil, up, shellButtonView(shellButton("进入", "folder", false, f.enter)), f.directory), 0, 30)
	actions := shellHBox(shellButtonView(shellButton("刷新", "refresh-cw", false, f.refresh)), shellFixed(layout.NewSpacer(), 8, 0), shellButtonView(shellButton("上传", "", false, f.upload)), shellFixed(layout.NewSpacer(), 8, 0), shellButtonView(shellButton("下载", "", false, f.download)))
	f.cancelButton = shellButton("取消传输", "x", false, func() {
		if f.cancel != nil {
			f.cancel()
			f.status.SetText("正在取消…")
		}
	})
	f.cancelButton.Disable()
	headings := newShellFileRow()
	headings.name.SetText("名称 ↑")
	headings.size.SetText("大小")
	headings.modified.SetText("时间")
	headings.icon.Hide()
	header := shellVBox(directory, shellFixed(layout.NewSpacer(), 0, 8), actions, shellFixed(layout.NewSpacer(), 0, 8), f.search, shellFixed(layout.NewSpacer(), 0, 8), headings, shellLine())
	footer := shellVBox(shellLine(), shellFixed(layout.NewSpacer(), 0, 6), shellBorder(nil, nil, nil, shellButtonView(f.cancelButton), shellLabel(f.status, 11)))
	return shellInset(shellBorder(header, footer, nil, nil, f.list), 12)
}

func (f *shellFiles) filterFiles() {
	f.shown = nil
	needle := strings.ToLower(f.search.Text)
	for i, file := range f.files {
		if strings.Contains(strings.ToLower(file.Name), needle) {
			f.shown = append(f.shown, i)
		}
	}
	f.selected = -1
	f.list.UnselectAll()
	f.list.Refresh()
}

type shellFileRow struct {
	widget.BaseWidget
	icon                 *canvas.Image
	name, size, modified *widget.Label
}

func newShellFileRow() *shellFileRow {
	r := &shellFileRow{icon: canvas.NewImageFromResource(shellIcon("file-code", false)), name: widget.NewLabel(""), size: widget.NewLabel(""), modified: widget.NewLabel("")}
	r.icon.FillMode = canvas.ImageFillContain
	for _, label := range []*widget.Label{r.name, r.size, r.modified} {
		label.Truncation = fyne.TextTruncateEllipsis
	}
	r.size.Importance, r.modified.Importance = widget.LowImportance, widget.LowImportance
	r.ExtendBaseWidget(r)
	return r
}
func (r *shellFileRow) update(file transport.File) {
	name := "file-code"
	if file.Directory {
		name = "folder"
	}
	r.icon.Resource = shellIcon(name, file.Directory)
	r.icon.Refresh()
	r.name.SetText(file.Name)
	size := "—"
	if !file.Directory {
		size = shellFileSize(file.Size)
	}
	r.size.SetText(size)
	modified := "—"
	if !file.Modified.IsZero() {
		modified = file.Modified.Format("01/02 15:04")
	}
	r.modified.SetText(modified)
}
func shellFileSize(size int64) string {
	for i, unit := range []string{"GiB", "MiB", "KiB"} {
		divisor := float64(int64(1) << uint(30-i*10))
		if float64(size) >= divisor {
			return fmt.Sprintf("%.1f %s", float64(size)/divisor, unit)
		}
	}
	return fmt.Sprintf("%d B", size)
}
func (r *shellFileRow) CreateRenderer() fyne.WidgetRenderer {
	return &shellFileRowRenderer{objects: []fyne.CanvasObject{r.icon, shellLabel(r.name, 12), shellLabel(r.size, 11), shellLabel(r.modified, 11)}}
}

type shellFileRowRenderer struct {
	objects []fyne.CanvasObject
}

func (*shellFileRowRenderer) MinSize() fyne.Size { return fyne.NewSize(260, 36) }
func (r *shellFileRowRenderer) Layout(size fyne.Size) {
	edge := max(90, size.Width-150)
	positions := []fyne.Position{fyne.NewPos(0, 10), fyne.NewPos(22, 0), fyne.NewPos(edge, 0), fyne.NewPos(size.Width-84, 0)}
	sizes := []fyne.Size{fyne.NewSquareSize(16), fyne.NewSize(max(0, edge-26), 36), fyne.NewSize(62, 36), fyne.NewSize(84, 36)}
	for i, object := range r.objects {
		object.Move(positions[i])
		object.Resize(sizes[i])
	}
}
func (r *shellFileRowRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (*shellFileRowRenderer) Destroy()                       {}
func (r *shellFileRowRenderer) Refresh() {
	for _, object := range r.objects {
		object.Refresh()
	}
}
