package datafile

import (
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ealink1/navi-fyne/internal/sqlworkbench"
)

func Text(value any) string {
	switch v := value.(type) {
	case nil:
		return "NULL"
	case []byte:
		return base64.StdEncoding.EncodeToString(v)
	case time.Time:
		return v.Format(time.RFC3339Nano)
	case map[string]any, []any:
		raw, _ := json.Marshal(v)
		return string(raw)
	default:
		return fmt.Sprint(v)
	}
}
func (e *Encoder) start() error {
	var err error
	switch e.options.Format {
	case "CSV":
		if e.options.BOM {
			if _, err = io.WriteString(e.output, "\ufeff"); err != nil {
				return err
			}
		}
		e.csv = csv.NewWriter(e.output)
		if e.options.Delimiter != 0 {
			e.csv.Comma = e.options.Delimiter
		}
		header := make([]string, len(e.columns))
		for i, c := range e.columns {
			header[i] = CSVText(c)
		}
		return e.csv.Write(header)
	case "JSON":
		_, err = io.WriteString(e.output, "[\n")
	case "Markdown":
		_, err = fmt.Fprintf(e.output, "| %s |\n| %s |\n", strings.Join(mdValues(e.columns), " | "), strings.TrimSuffix(strings.Repeat("--- | ", len(e.columns)), " | "))
	case "HTML":
		_, err = io.WriteString(e.output, "<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"><title>Query result</title><style>body{font:14px system-ui}table{border-collapse:collapse}td,th{border:1px solid #d1d5db;padding:8px;text-align:left}.null{color:#9ca3af}</style></head><body><table><thead><tr>")
		if err != nil {
			return err
		}
		for _, c := range e.columns {
			if _, err = fmt.Fprintf(e.output, "<th>%s</th>", html.EscapeString(c)); err != nil {
				return err
			}
		}
		_, err = io.WriteString(e.output, "</tr></thead><tbody>\n")
	case "INSERT SQL":
		if e.options.Table == "" {
			return errors.New("INSERT SQL requires a destination table")
		}
	case "XLSX":
		e.xlsx, err = newXLSXWriter(e.columns)
	}
	return err
}
func (e *Encoder) row(row []any) error {
	texts := make([]string, len(row))
	for i, v := range row {
		texts[i] = Text(v)
	}
	switch e.options.Format {
	case "CSV":
		for i, v := range row {
			if _, ok := v.(string); ok {
				texts[i] = CSVText(texts[i])
			}
		}
		return e.csv.Write(texts)
	case "JSON":
		raw, err := jsonRow(e.columns, row)
		if err != nil {
			return err
		}
		if e.Count > 0 {
			if _, err = io.WriteString(e.output, ",\n"); err != nil {
				return err
			}
		}
		_, err = e.output.Write(raw)
		return err
	case "Markdown":
		_, err := fmt.Fprintf(e.output, "| %s |\n", strings.Join(mdValues(texts), " | "))
		return err
	case "HTML":
		if _, err := io.WriteString(e.output, "<tr>"); err != nil {
			return err
		}
		for i, v := range texts {
			class := ""
			if row[i] == nil {
				class = " class=\"null\""
			}
			if _, err := fmt.Fprintf(e.output, "<td%s>%s</td>", class, html.EscapeString(v)); err != nil {
				return err
			}
		}
		_, err := io.WriteString(e.output, "</tr>\n")
		return err
	case "INSERT SQL":
		return e.insertRow(row)
	case "XLSX":
		return e.xlsx.row(e.Count+2, row)
	}
	return errors.New("unknown export format")
}
func mdValues(values []string) []string {
	result := make([]string, len(values))
	for i, v := range values {
		v = html.EscapeString(v)
		v = strings.ReplaceAll(v, "\\", "\\\\")
		v = strings.ReplaceAll(v, "|", "\\|")
		v = strings.ReplaceAll(v, "\r\n", "<br>")
		v = strings.ReplaceAll(v, "\n", "<br>")
		result[i] = v
	}
	return result
}
func (e *Encoder) insertRow(row []any) error {
	columns := make([]string, len(e.columns))
	values := make([]string, len(row))
	for i, c := range e.columns {
		q, err := sqlworkbench.Quote(e.options.Dialect, c)
		if err != nil {
			return err
		}
		columns[i] = q
	}
	for i, v := range row {
		text, err := SQLLiteral(e.options.Dialect, v)
		if err != nil {
			return err
		}
		values[i] = text
	}
	_, err := fmt.Fprintf(e.output, "INSERT INTO %s (%s) VALUES (%s);\n", e.options.Table, strings.Join(columns, ", "), strings.Join(values, ", "))
	return err
}
func SQLLiteral(dialect string, value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "NULL", nil
	case bool:
		switch dialect {
		case "postgres", "kingbase", "highgo", "vastbase", "opengauss", "gaussdb", "duckdb", "trino":
			return strings.ToUpper(strconv.FormatBool(v)), nil
		}
		if v {
			return "1", nil
		}
		return "0", nil
	case json.Number:
		if len(v) == 0 || v[0] != '-' && (v[0] < '0' || v[0] > '9') || !json.Valid([]byte(v)) {
			return "", errors.New("invalid numeric export value")
		}
		return string(v), nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprint(v), nil
	case float32:
		return SQLLiteral(dialect, float64(v))
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return "", errors.New("non-finite number cannot be exported to SQL")
		}
		return strconv.FormatFloat(v, 'g', -1, 64), nil
	case []byte:
		h := hex.EncodeToString(v)
		switch dialect {
		case "postgres", "kingbase", "highgo", "vastbase", "opengauss", "gaussdb":
			return "decode('" + h + "', 'hex')", nil
		case "oracle", "dameng":
			return "HEXTORAW('" + h + "')", nil
		case "sqlserver":
			return "0x" + h, nil
		default:
			return "X'" + h + "'", nil
		}
	default:
		return sqlworkbench.TextLiteral(dialect, Text(v)), nil
	}
}
