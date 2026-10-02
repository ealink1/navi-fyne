package datafile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"
)

type xlsxWriter struct {
	file   *excelize.File
	stream *excelize.StreamWriter
}

func newXLSXWriter(columns []string) (*xlsxWriter, error) {
	x := &xlsxWriter{file: excelize.NewFile()}
	stream, err := x.file.NewStreamWriter("Sheet1")
	if err != nil {
		_ = x.file.Close()
		return nil, err
	}
	x.stream = stream
	header := make([]any, len(columns))
	for i, c := range columns {
		header[i] = c
	}
	if err = x.stream.SetRow("A1", header); err != nil {
		_ = x.file.Close()
		return nil, err
	}
	return x, nil
}
func (x *xlsxWriter) row(number int64, row []any) error {
	if number > 1048576 {
		return errors.New("XLSX exceeds Excel's 1,048,576-row limit; use CSV or JSON")
	}
	values := make([]any, len(row))
	for i, v := range row {
		if v == nil {
			continue
		}
		text := Text(v)
		if len([]rune(text)) > 32767 {
			return errors.New("XLSX cell exceeds Excel's 32,767-character limit")
		}
		switch v.(type) {
		case int, int64, uint64, json.Number:
			if len(strings.TrimLeft(text, "+-")) > 15 {
				values[i] = text
			} else {
				values[i] = v
			}
		case string, float64, float32, bool:
			values[i] = v
		default:
			values[i] = text
		}
	}
	return x.stream.SetRow(fmt.Sprintf("A%d", number), values)
}
func (x *xlsxWriter) finish(output io.Writer) error {
	if err := x.stream.Flush(); err != nil {
		return err
	}
	return x.file.Write(output)
}
