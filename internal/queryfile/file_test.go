package queryfile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSQLFileRoundTripPermissionsAndNoClobber(t *testing.T) {
	path := filepath.Join(t.TempDir(), "查询.sql")
	text := "SELECT '中文', E'\\n';\n-- 注释\n"
	if err := Save(t.Context(), path, text, false); err != nil {
		t.Fatal(err)
	}
	if err := Save(t.Context(), path, "replaced", false); !errors.Is(err, os.ErrExist) {
		t.Fatal("overwrote an existing file", err)
	}
	read, err := Read(t.Context(), path)
	if err != nil || read != text {
		t.Fatal("SQL text changed", err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("SQL file is not private")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := Save(ctx, path, "cancelled", true); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := Save(t.Context(), path, strings.Repeat("x", MaxSize+1), true); err == nil {
		t.Fatal("oversized file published")
	}
	read, _ = Read(t.Context(), path)
	if read != text {
		t.Fatal("failed write damaged document")
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".navifyne-sql-*"))
	if len(matches) != 0 {
		t.Fatal("temporary SQL file leaked")
	}
}

func TestSQLReadAcceptsUTF8BOMAndRejectsMalformedText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "query.sql")
	for _, fixture := range []struct {
		raw, want string
		valid     bool
	}{{"\ufeffSELECT 1", "SELECT 1", true}, {"\xff\xfeS\x00", "", false}, {strings.Repeat("x", MaxSize+4), "", false}} {
		if err := os.WriteFile(path, []byte(fixture.raw), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := Read(t.Context(), path)
		if fixture.valid && (err != nil || got != fixture.want) || !fixture.valid && err == nil {
			t.Fatal("wrong encoding or size handling", err)
		}
	}
}
