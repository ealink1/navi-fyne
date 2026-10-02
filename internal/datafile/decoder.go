package datafile

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

type Dataset struct {
	Columns []string
	Rows    [][]any
}
type ReadOptions struct {
	Format, Encoding, Sheet string
	Delimiter               rune
	EmptyAsNull             bool
}

const MaxImportBytes = 64 << 20
const MaxImportRows = 100000

func ReadFile(ctx context.Context, path string, options ReadOptions) (Dataset, error) {
	file, err := os.Open(path)
	if err != nil {
		return Dataset{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Dataset{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxImportBytes {
		return Dataset{}, errors.New("import requires a regular file no larger than 64 MiB")
	}
	if options.Format == "" {
		options.Format = strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), "."))
	}
	reader := io.Reader(&contextReader{ctx: ctx, reader: io.LimitReader(file, MaxImportBytes+1)})
	if options.Format != "XLSX" {
		switch options.Encoding {
		case "", "UTF-8":
		case "GBK":
			reader = transform.NewReader(reader, simplifiedchinese.GBK.NewDecoder())
		case "GB18030":
			reader = transform.NewReader(reader, simplifiedchinese.GB18030.NewDecoder())
		default:
			return Dataset{}, errors.New("unsupported file encoding")
		}
		reader = transform.NewReader(reader, &utf8Validator{})
	}
	switch options.Format {
	case "CSV", "TSV":
		return readCSV(ctx, reader, options)
	case "JSON":
		return readJSON(ctx, reader)
	case "XLSX":
		return readXLSX(ctx, reader, options)
	default:
		return Dataset{}, errors.New("import supports CSV, JSON and XLSX")
	}
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
func readCSV(ctx context.Context, reader io.Reader, options ReadOptions) (Dataset, error) {
	csvReader := csv.NewReader(reader)
	csvReader.ReuseRecord = true
	if options.Delimiter != 0 {
		csvReader.Comma = options.Delimiter
	} else if options.Format == "TSV" {
		csvReader.Comma = '\t'
	}
	headers, err := csvReader.Read()
	if err != nil {
		return Dataset{}, fmt.Errorf("read CSV headers: %w", err)
	}
	columns := append([]string(nil), headers...)
	if len(columns) > 0 {
		columns[0] = strings.TrimPrefix(columns[0], "\ufeff")
	}
	if err = validateColumns(columns); err != nil {
		return Dataset{}, err
	}
	data := Dataset{Columns: columns}
	for {
		if err = ctx.Err(); err != nil {
			return Dataset{}, err
		}
		record, readErr := csvReader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return Dataset{}, fmt.Errorf("CSV row %d: %w", len(data.Rows)+2, readErr)
		}
		row := make([]any, len(record))
		for i, v := range record {
			if options.EmptyAsNull && v == "" {
				continue
			}
			row[i] = v
		}
		if err = data.appendRow(row); err != nil {
			return Dataset{}, err
		}
	}
	return data, nil
}
func validateColumns(columns []string) error {
	if len(columns) == 0 || len(columns) > 1024 {
		return errors.New("import requires 1–1024 columns")
	}
	seen := map[string]bool{}
	for _, name := range columns {
		if name == "" || len(name) > 512 || seen[name] {
			return errors.New("file columns must have distinct non-empty names")
		}
		seen[name] = true
	}
	return nil
}
func (d *Dataset) appendRow(row []any) error {
	if (len(d.Rows)+1)*len(d.Columns) > 2000000 {
		return errors.New("import exceeds 2,000,000 cells")
	}
	if len(d.Rows) >= MaxImportRows {
		return errors.New("import exceeds 100,000 rows")
	}
	if len(row) != len(d.Columns) {
		return errors.New("file row and column counts do not match")
	}
	for _, v := range row {
		if len(Text(v)) > 1<<20 {
			return errors.New("import field exceeds 1 MiB")
		}
	}
	d.Rows = append(d.Rows, row)
	return nil
}
