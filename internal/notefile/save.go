// Package notefile exports complete local Markdown documents atomically.
package notefile

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/ealink1/super-link/internal/domain"
)

// Save never truncates a previous document before the replacement is complete.
func Save(ctx context.Context, path, body string, overwrite bool) (resultErr error) {
	if len(body) > domain.MaxNoteBody || !utf8.ValidString(body) || strings.ContainsRune(body, 0) {
		return errors.New("Markdown 必须为有效 UTF-8，正文最多 256 KiB")
	}
	if !filepath.IsAbs(path) {
		return errors.New("请选择完整的导出路径")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".superlink-note-*")
	if err != nil {
		return err
	}
	defer func() {
		err := os.Remove(file.Name())
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, err)
		}
	}()
	written, err := io.WriteString(file, body)
	if err == nil && written != len(body) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close(), ctx.Err())
	if err != nil {
		return err
	}
	if overwrite {
		return os.Rename(file.Name(), path)
	}
	return os.Link(file.Name(), path)
}
