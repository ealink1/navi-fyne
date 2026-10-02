// Package datafile reads and writes bounded data files independently of the UI.
package datafile

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type Options struct {
	Format         string
	Columns        []int // nil selects every column; order is preserved.
	Dialect, Table string
	BOM            bool
	Delimiter      rune
}

func Extension(format string) string {
	return map[string]string{"XLSX": ".xlsx", "CSV": ".csv", "JSON": ".json", "Markdown": ".md", "HTML": ".html", "INSERT SQL": ".sql"}[format]
}

var Formats = []string{"XLSX", "CSV", "JSON", "Markdown", "HTML", "INSERT SQL"}

type Encoder struct {
	ctx               context.Context
	output            io.Writer
	options           Options
	columns           []string
	indices           []int
	csv               *csv.Writer
	xlsx              *xlsxWriter
	Count             int64
	started, finished bool
}

func NewEncoder(ctx context.Context, output io.Writer, options Options) (*Encoder, error) {
	if Extension(options.Format) == "" {
		return nil, errors.New("unsupported export format")
	}
	return &Encoder{ctx: ctx, output: &quotaWriter{Writer: output, remaining: 8 << 30}, options: options}, nil
}
func (e *Encoder) SetColumns(columns []string) error {
	if err := e.ctx.Err(); err != nil {
		return err
	}
	if e.started || len(columns) == 0 || len(columns) > 16384 {
		return errors.New("invalid or repeated export columns")
	}
	if e.options.Columns != nil {
		e.indices = make([]int, len(e.options.Columns))
		copy(e.indices, e.options.Columns)
	}
	if e.indices == nil {
		for i := range columns {
			e.indices = append(e.indices, i)
		}
	}
	seen := map[int]bool{}
	for _, i := range e.indices {
		if i < 0 || i >= len(columns) || seen[i] {
			return errors.New("invalid export column selection")
		}
		seen[i] = true
		e.columns = append(e.columns, columns[i])
	}
	if len(e.columns) == 0 {
		return errors.New("select at least one export column")
	}
	e.started = true
	return e.start()
}
func (e *Encoder) ConsumeRowValues(values []any) error {
	if err := e.ctx.Err(); err != nil {
		return err
	}
	if !e.started || e.finished {
		return errors.New("export encoder is not accepting rows")
	}
	if e.Count >= 5000000 {
		return errors.New("export exceeds 5,000,000 rows")
	}
	row := make([]any, len(e.indices))
	for col, index := range e.indices {
		if index >= len(values) {
			return errors.New("export row has fewer fields than its columns")
		}
		row[col] = values[index]
		if len(Text(row[col])) > 1<<20 {
			return fmt.Errorf("column %s exceeds 1 MiB; export stopped without replacing the destination", e.columns[col])
		}
	}
	if err := e.row(row); err != nil {
		return err
	}
	e.Count++
	return nil
}
func (e *Encoder) Finish() error {
	if err := e.ctx.Err(); err != nil {
		return err
	}
	if !e.started || e.finished {
		return errors.New("export has no columns or is already finished")
	}
	e.finished = true
	switch e.options.Format {
	case "CSV":
		e.csv.Flush()
		return e.csv.Error()
	case "JSON":
		_, err := io.WriteString(e.output, "\n]\n")
		return err
	case "HTML":
		_, err := io.WriteString(e.output, "</tbody></table></body></html>\n")
		return err
	case "XLSX":
		return e.xlsx.finish(e.output)
	}
	return nil
}
func (e *Encoder) Close() error {
	if e.xlsx != nil {
		return e.xlsx.file.Close()
	}
	return nil
}

func CSVText(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if strings.HasPrefix(value, "\t") || strings.HasPrefix(value, "\r") || strings.HasPrefix(value, "\n") || len(trimmed) > 0 && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}

type quotaWriter struct {
	io.Writer
	remaining int64
}

func (q *quotaWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > q.remaining {
		return 0, errors.New("export exceeds 8 GiB")
	}
	n, err := q.Writer.Write(p)
	q.remaining -= int64(n)
	if n < len(p) && err == nil {
		err = io.ErrShortWrite
	}
	return n, err
}
func jsonRow(columns []string, row []any) ([]byte, error) {
	object := make(map[string]any, len(columns))
	for i, c := range columns {
		if _, exists := object[c]; exists {
			return nil, errors.New("JSON export requires distinct column names; alias duplicate columns")
		}
		object[c] = row[i]
	}
	return json.Marshal(object)
}
