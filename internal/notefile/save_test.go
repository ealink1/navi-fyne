package notefile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ealink1/navi-fyne/internal/domain"
)

func TestSavePreservesExistingMarkdownOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.md")
	if err := os.WriteFile(path, []byte("previous"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name, body string
		ctx        context.Context
		overwrite  bool
	}{
		{"conflict", "new", context.Background(), false}, {"cancelled", "new", ctx, true}, {"oversize", strings.Repeat("x", domain.MaxNoteBody+1), context.Background(), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := Save(tc.ctx, path, tc.body, tc.overwrite); err == nil {
				t.Fatal("invalid replacement accepted")
			}
			raw, err := os.ReadFile(path)
			if err != nil || string(raw) != "previous" {
				t.Fatal("previous file lost", err)
			}
		})
	}
	if err := Save(context.Background(), path, "# 中文笔记", true); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "# 中文笔记" {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatal("temporary file retained", err)
	}
	if err := Save(context.Background(), path, "new", false); !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
}
