package ui

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
	"github.com/ealink1/navi-fyne/internal/domain"
	"github.com/ealink1/navi-fyne/internal/notefile"
)

func (n *noteWorkspace) importMarkdown() {
	if !n.loaded {
		return
	}
	chooser := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			n.status.SetText(err.Error())
			return
		}
		if reader == nil {
			return
		}
		name := strings.TrimSuffix(reader.URI().Name(), filepath.Ext(reader.URI().Name()))
		n.owner.jobs.run(func(ctx context.Context) (value any, resultErr error) {
			defer func() { resultErr = errors.Join(resultErr, reader.Close()) }()
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			raw, err := io.ReadAll(io.LimitReader(reader, domain.MaxNoteBody+1))
			if err != nil {
				return nil, err
			}
			if len(raw) > domain.MaxNoteBody || !utf8.Valid(raw) {
				return nil, errors.New("请选择不超过 256 KiB 的 UTF-8 Markdown 文件")
			}
			return string(raw), nil
		}, func(value any, err error) {
			if err != nil {
				n.status.SetText(err.Error())
				return
			}
			previous := len(n.book.Notes)
			n.newNote()
			if len(n.book.Notes) != previous+1 {
				return
			}
			note := n.current()
			if note == nil {
				return
			}
			n.title.SetText(truncateShellTitle(name, 200))
			n.editor.SetText(value.(string))
			n.save()
		})
	}, n.owner.Window)
	chooser.SetFilter(storage.NewExtensionFileFilter([]string{".md", ".markdown", ".txt"}))
	chooser.Show()
}
func (n *noteWorkspace) exportMarkdown() {
	note := n.current()
	if note == nil {
		return
	}
	body, filename := note.Body, noteFilename(note.Title)
	chooser := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil {
			n.status.SetText(err.Error())
			return
		}
		if uri == nil {
			return
		}
		name := widget.NewEntry()
		name.SetText(filename)
		modal := dialog.NewForm("导出 Markdown", "导出", "取消", []*widget.FormItem{widget.NewFormItem("文件名", name)}, func(ok bool) {
			if ok {
				n.saveMarkdownFile(filepath.Join(uri.Path(), noteFilename(strings.TrimSuffix(name.Text, ".md"))), body, false)
			}
		}, n.owner.Window)
		modal.Show()
	}, n.owner.Window)
	chooser.Show()
}
func (n *noteWorkspace) saveMarkdownFile(path, body string, overwrite bool) {
	n.owner.jobs.run(func(ctx context.Context) (any, error) { return nil, notefile.Save(ctx, path, body, overwrite) }, func(_ any, err error) {
		if errors.Is(err, os.ErrExist) && !overwrite {
			dialog.ShowConfirm("覆盖 Markdown 文件", "替换「"+filepath.Base(path)+"」？", func(ok bool) {
				if ok {
					n.saveMarkdownFile(path, body, true)
				}
			}, n.owner.Window)
			return
		}
		if err != nil {
			n.status.SetText("导出失败，目标文件保留：" + err.Error())
			return
		}
		n.status.SetText("Markdown 已导出")
	})
}
func noteFilename(title string) string {
	title = strings.TrimSpace(title)
	title = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, title)
	title = strings.Trim(title, ". ")
	if title == "" {
		title = "note"
	}
	return truncateShellTitle(title, 80) + ".md"
}
