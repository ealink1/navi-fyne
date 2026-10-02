package datafile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/simplifiedchinese"
)

func importFile(t *testing.T, name string, raw []byte, options ReadOptions) (Dataset, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return ReadFile(context.Background(), path, options)
}

func TestCSVImportPreservesNullLiteralAndChineseEncoding(t *testing.T) {
	text := "\ufeffid,name,optional\n9223372036854775807,中文,\n2,NULL,保留\n"
	data, err := importFile(t, "items.csv", []byte(text), ReadOptions{EmptyAsNull: true})
	if err != nil || data.Rows[0][0] != "9223372036854775807" || data.Rows[0][2] != nil || data.Rows[1][1] != "NULL" {
		t.Fatal(data, err)
	}
	gbk, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("id,name\n1,中文\n"))
	if err != nil {
		t.Fatal(err)
	}
	data, err = importFile(t, "items.csv", gbk, ReadOptions{Encoding: "GBK"})
	if err != nil || data.Rows[0][1] != "中文" {
		t.Fatal(data, err)
	}
}

func TestJSONImportUnionsColumnsWithoutLosingIntegerPrecision(t *testing.T) {
	data, err := importFile(t, "items.json", []byte(`[{"id":9223372036854775807,"name":"中文"},{"id":2,"optional":null,"extra":{"x":1}}]`), ReadOptions{})
	if err != nil || len(data.Columns) != 4 || data.Columns[0] != "id" || data.Rows[0][0] != json.Number("9223372036854775807") || data.Rows[0][2] != nil {
		t.Fatal(data, err)
	}
}

func TestXLSXImportUsesSelectedSheetAndPreservesStrings(t *testing.T) {
	book := excelize.NewFile()
	defer book.Close()
	book.NewSheet("导入数据")
	book.SetSheetRow("导入数据", "A1", &[]any{"id", "name", "optional"})
	book.SetSheetRow("导入数据", "A2", &[]any{"9223372036854775807", "=not-a-formula", ""})
	raw, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	data, err := importFile(t, "items.xlsx", raw.Bytes(), ReadOptions{Sheet: "导入数据", EmptyAsNull: true})
	if err != nil || data.Rows[0][0] != "9223372036854775807" || data.Rows[0][1] != "=not-a-formula" || data.Rows[0][2] != nil {
		t.Fatal(data, err)
	}
	if _, err = importFile(t, "items.xlsx", raw.Bytes(), ReadOptions{Sheet: "missing"}); err == nil {
		t.Fatal("missing worksheet accepted")
	}
}

func TestImportRejectsMalformedFilesBoundsAndCancellation(t *testing.T) {
	for _, test := range []struct{ name, text string }{
		{"duplicate.csv", "a,a\n1,2\n"},
		{"shape.csv", "a,b\n1\n"},
		{"tail.json", `[{"a":1}] {"tail":2}`},
		{"nonobjects.json", `[1]`},
		{"duplicate.json", `[{"a":1,"a":2}]`},
		{"invalid-utf8.json", string([]byte{'[', '{', '"', 'a', '"', ':', '"', 255, '"', '}', ']'})},
		{"large.csv", "a\n" + strings.Repeat("x", (1<<20)+1) + "\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := importFile(t, test.name, []byte(test.text), ReadOptions{}); err == nil {
				t.Fatal("invalid file accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readCSV(ctx, bytes.NewBufferString("a\n1\n"), ReadOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	d := Dataset{Columns: make([]string, 1024), Rows: make([][]any, 2000000/1024)}
	if err := d.appendRow(make([]any, 1024)); err == nil {
		t.Fatal("cell memory cap bypassed")
	}
}

func TestSQLExportUsesNativeBooleanAndRejectsMalformedNumbers(t *testing.T) {
	for _, test := range []struct {
		dialect string
		value   bool
		want    string
	}{{"postgres", true, "TRUE"}, {"mysql", true, "1"}, {"postgres", false, "FALSE"}, {"sqlserver", false, "0"}} {
		if value, err := SQLLiteral(test.dialect, test.value); err != nil || value != test.want {
			t.Fatal(value, err)
		}
	}
	for _, value := range []json.Number{"NaN", "Infinity", "+1", "01", `"123"`, `[1]`, "true"} {
		if _, err := SQLLiteral("postgres", value); err == nil {
			t.Fatal("invalid numeric SQL", value)
		}
	}
}
