package datafile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
)

func readJSON(ctx context.Context, reader io.Reader) (Dataset, error) {
	decoder := json.NewDecoder(reader)
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return Dataset{}, err
	}
	if token != json.Delim('[') {
		return Dataset{}, errors.New("JSON import requires an array of objects")
	}
	data := Dataset{}
	indices := map[string]int{}
	for decoder.More() {
		if err = ctx.Err(); err != nil {
			return Dataset{}, err
		}
		object, err := readJSONObject(decoder)
		if err != nil {
			return Dataset{}, fmt.Errorf("JSON row %d: %w", len(data.Rows)+1, err)
		}
		newColumns := []string{}
		for name := range object {
			if _, ok := indices[name]; !ok {
				newColumns = append(newColumns, name)
			}
		}
		sort.Strings(newColumns)
		for _, name := range newColumns {
			if (len(data.Columns)+1)*max(1, len(data.Rows)) > 2000000 {
				return Dataset{}, errors.New("import exceeds 2,000,000 cells")
			}
			indices[name] = len(data.Columns)
			data.Columns = append(data.Columns, name)
			for i := range data.Rows {
				data.Rows[i] = append(data.Rows[i], nil)
			}
		}
		if err = validateColumns(data.Columns); err != nil {
			return Dataset{}, err
		}
		row := make([]any, len(data.Columns))
		for name, value := range object {
			row[indices[name]] = value
		}
		if err = data.appendRow(row); err != nil {
			return Dataset{}, err
		}
	}
	if _, err = decoder.Token(); err != nil {
		return Dataset{}, err
	}
	var extra any
	if err = decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Dataset{}, errors.New("unexpected content after JSON array")
	}
	if len(data.Columns) == 0 {
		return Dataset{}, errors.New("JSON array has no fields")
	}
	return data, nil
}

func readJSONObject(decoder *json.Decoder) (map[string]any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if token != json.Delim('{') {
		return nil, errors.New("row must be an object")
	}
	object := map[string]any{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := key.(string)
		if !ok {
			return nil, errors.New("invalid object field")
		}
		if _, exists := object[name]; exists {
			return nil, errors.New("duplicate object field")
		}
		if len(object) >= 1024 {
			return nil, errors.New("row exceeds 1024 fields")
		}
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		object[name] = value
	}
	_, err = decoder.Token()
	return object, err
}
