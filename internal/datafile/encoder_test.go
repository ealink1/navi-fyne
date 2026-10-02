package datafile

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func encodeFixture(t *testing.T, format string) []byte {
	t.Helper()
	var output bytes.Buffer
	e, err := NewEncoder(context.Background(), &output, Options{Format: format, Dialect: "postgres", Table: `"items"`, Columns: []int{2, 0, 1}})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err = e.SetColumns([]string{"text", "large", "null"}); err != nil {
		t.Fatal(err)
	}
	if err = e.ConsumeRowValues([]any{"=HYPERLINK('x') | <script>\nO'Reilly", json.Number("9223372036854775807"), nil}); err != nil {
		t.Fatal(err)
	}
	if err = e.Finish(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
func TestSixFormatsPreserveColumnsValuesAndNeutralizeActiveContent(t *testing.T) {
	for _, format := range Formats {
		t.Run(format, func(t *testing.T) {
			raw := encodeFixture(t, format)
			if len(raw) == 0 {
				t.Fatal("empty file")
			}
			switch format {
			case "CSV":
				rows, err := csv.NewReader(bytes.NewReader(raw)).ReadAll()
				if err != nil || rows[0][0] != "null" || rows[1][0] != "NULL" || !strings.HasPrefix(rows[1][1], "'=") || rows[1][2] != "9223372036854775807" {
					t.Fatal(rows, err)
				}
			case "JSON":
				d := json.NewDecoder(bytes.NewReader(raw))
				d.UseNumber()
				var rows []map[string]any
				if err := d.Decode(&rows); err != nil || rows[0]["null"] != nil || rows[0]["large"] != json.Number("9223372036854775807") {
					t.Fatal(rows, err)
				}
			case "HTML":
				if strings.Contains(string(raw), "<script>") || !strings.Contains(string(raw), "&lt;script&gt;") {
					t.Fatal("unescaped active content")
				}
			case "Markdown":
				if !strings.Contains(string(raw), `\|`) || !strings.Contains(string(raw), "<br>") {
					t.Fatal("markdown structure damaged")
				}
			case "INSERT SQL":
				if !strings.Contains(string(raw), "NULL, E'") || !strings.Contains(string(raw), "O''Reilly") || !strings.Contains(string(raw), "9223372036854775807") {
					t.Fatal(string(raw))
				}
			case "XLSX":
				book, err := excelize.OpenReader(bytes.NewReader(raw))
				if err != nil {
					t.Fatal(err)
				}
				defer book.Close()
				if value, _ := book.GetCellValue("Sheet1", "C2"); value != "9223372036854775807" {
					t.Fatal("large integer rounded", value)
				}
				if formula, _ := book.GetCellFormula("Sheet1", "B2"); formula != "" {
					t.Fatal("string became an Excel formula", formula)
				}
			}
		})
	}
}
func TestAtomicExportNeverReplacesExistingFileOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.csv")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := SaveAtomic(context.Background(), path, true, Options{Format: "CSV"}, func(e *Encoder) error {
		if err := e.SetColumns([]string{"id"}); err != nil {
			return err
		}
		e.ConsumeRowValues([]any{1})
		return errors.New("fixture read interrupted")
	})
	if err == nil {
		t.Fatal("read failure ignored")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "original" {
		t.Fatal("original replaced by partial file")
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".navifyne-export-*"))
	if len(matches) != 0 {
		t.Fatal("temporary export leaked")
	}
	if _, err = SaveAtomic(context.Background(), path, false, Options{Format: "JSON"}, func(*Encoder) error { return nil }); !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
}
func TestExportRejectsEmptySelectionMalformedRowsAndCancellation(t *testing.T) {
	for _, indices := range [][]int{{}, {0, 0}, {-1}, {1}} {
		e, _ := NewEncoder(context.Background(), &bytes.Buffer{}, Options{Format: "CSV", Columns: indices})
		if err := e.SetColumns([]string{"a"}); err == nil {
			t.Fatal("bad columns accepted", indices)
		}
	}
	e, _ := NewEncoder(context.Background(), &bytes.Buffer{}, Options{Format: "CSV"})
	e.SetColumns([]string{"a", "b"})
	if err := e.ConsumeRowValues([]any{1}); err == nil {
		t.Fatal("malformed row accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e, _ = NewEncoder(ctx, &bytes.Buffer{}, Options{Format: "JSON"})
	if err := e.SetColumns([]string{"a"}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
