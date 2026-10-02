package datafile

import (
	"context"
	"errors"
	"io"

	"github.com/xuri/excelize/v2"
)

func readXLSX(ctx context.Context, reader io.Reader, options ReadOptions) (Dataset, error) {
	book, err := excelize.OpenReader(reader, excelize.Options{UnzipSizeLimit: MaxImportBytes, UnzipXMLSizeLimit: 16 << 20, RawCellValue: true})
	if err != nil {
		return Dataset{}, err
	}
	defer book.Close()
	sheet := options.Sheet
	if sheet == "" {
		list := book.GetSheetList()
		if len(list) == 0 {
			return Dataset{}, errors.New("XLSX has no worksheets")
		}
		sheet = list[0]
	}
	rows, err := book.Rows(sheet)
	if err != nil {
		return Dataset{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return Dataset{}, errors.New("XLSX worksheet has no header")
	}
	columns, err := rows.Columns(excelize.Options{RawCellValue: true})
	if err != nil {
		return Dataset{}, err
	}
	if err = validateColumns(columns); err != nil {
		return Dataset{}, err
	}
	data := Dataset{Columns: columns}
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			return Dataset{}, err
		}
		record, err := rows.Columns(excelize.Options{RawCellValue: true})
		if err != nil {
			return Dataset{}, err
		}
		if len(record) > len(columns) {
			return Dataset{}, errors.New("XLSX row has fields beyond its header")
		}
		row := make([]any, len(columns))
		for i := range columns {
			v := ""
			if i < len(record) {
				v = record[i]
			}
			if options.EmptyAsNull && v == "" {
				continue
			}
			row[i] = v
		}
		if err = data.appendRow(row); err != nil {
			return Dataset{}, err
		}
	}
	return data, rows.Error()
}
