package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/queryfile"
	"github.com/google/uuid"
)

func (s *workspace) openSQLFile() {
	profile, scope, schema := s.profile, s.scope.Text, s.schemaName
	modal := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			s.owner.showError(err)
			return
		}
		if reader == nil {
			return
		}
		path := reader.URI().Path()
		if err = reader.Close(); err != nil {
			s.owner.showError(err)
			return
		}
		s.owner.jobs.run(func(ctx context.Context) (any, error) { return queryfile.Read(ctx, path) }, func(value any, err error) {
			if s.closed {
				return
			}
			if err != nil {
				s.owner.showError(err)
				return
			}
			opened := s.owner.openWorkspace(profile, domain.Draft{ID: uuid.NewString(), ProfileID: profile.ID, Title: filepath.Base(path), Scope: scope, Schema: schema, Text: value.(string)})
			if opened != nil {
				opened.refreshObjects()
			}
		})
	}, s.owner.Window)
	modal.SetFilter(storage.NewExtensionFileFilter([]string{".sql"}))
	modal.Show()
}

func (s *workspace) exportSQLFile() {
	if s.closed {
		return
	}
	text := s.editor.Text
	modal := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil {
			s.owner.showError(err)
			return
		}
		if uri == nil || s.closed {
			return
		}
		s.chooseSQLName(uri.Path(), text)
	}, s.owner.Window)
	modal.Show()
}

func (s *workspace) saveSQLFile(path, text string, overwrite bool) {
	s.owner.jobs.run(func(ctx context.Context) (any, error) { return nil, queryfile.Save(ctx, path, text, overwrite) }, func(_ any, err error) {
		if s.closed {
			return
		}
		if errors.Is(err, os.ErrExist) && !overwrite {
			dialog.ShowConfirm("覆盖 SQL 文件", "替换「"+filepath.Base(path)+"」？", func(ok bool) {
				if ok && !s.closed {
					s.saveSQLFile(path, text, true)
				}
			}, s.owner.Window)
			return
		}
		if err != nil {
			s.owner.showError(err)
			return
		}
		s.status.SetText("SQL 文件已导出：" + path)
	})
}

func sqlFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00") {
		return ""
	}
	if !strings.HasSuffix(strings.ToLower(name), ".sql") {
		name += ".sql"
	}
	return name
}
