package db

import (
	"database/sql"
	"strings"
	"testing"
)

func budgetOptionsContext(options RowBudgetOptions) *RowBudget {
	return NewRowBudgetWithOptions(options)
}

func TestScanMultiRowsSharesTotalRowBudget(t *testing.T) {
	counter := &rowBudgetFakeCounter{}
	rows := openRowBudgetFakeRows(t, openRowBudgetFakeDB(t, counter, 3, 3), "SELECT n; SELECT n")
	budget := budgetOptionsContext(RowBudgetOptions{
		MaxRowsPerResult: 10,
		MaxTotalRows:     4,
	})

	results, err := scanMultiRowsWithBudget(rows, "", budget)
	if err != nil {
		t.Fatalf("scanMultiRowsWithBudget: %v", err)
	}
	if len(results) != 2 || len(results[0].Rows) != 3 || len(results[1].Rows) != 1 {
		t.Fatalf("shared total-row budget returned %#v", results)
	}
	if results[0].Truncated || !results[1].Truncated || !budget.Exhausted() {
		t.Fatalf("shared total-row budget truncation state = results=%#v budget=%#v", results, budget)
	}
}

func TestScanMultiRowsMarksPreviousResultWhenExactTotalBudgetHidesLaterSet(t *testing.T) {
	counter := &rowBudgetFakeCounter{}
	rows := openRowBudgetFakeRows(t, openRowBudgetFakeDB(t, counter, 4, 2), "SELECT n; SELECT n")
	budget := NewRowBudgetWithOptions(RowBudgetOptions{MaxRowsPerResult: 10, MaxTotalRows: 4})

	results, err := scanMultiRowsWithBudget(rows, "", budget)
	if err != nil {
		t.Fatalf("scanMultiRowsWithBudget: %v", err)
	}
	if len(results) != 1 || len(results[0].Rows) != 4 || !results[0].Truncated {
		t.Fatalf("exact shared budget with later set returned %#v", results)
	}
}

type rowBudgetPayloadScanner struct {
	payload string
}

func (s rowBudgetPayloadScanner) scanCurrentPreviewRow(rows *sql.Rows) (map[string]interface{}, error) {
	var value int64
	if err := rows.Scan(&value); err != nil {
		return nil, err
	}
	return map[string]interface{}{"payload": s.payload}, nil
}

func (s rowBudgetPayloadScanner) scanCurrentRow(rows *sql.Rows) (map[string]interface{}, error) {
	return s.scanCurrentPreviewRow(rows)
}

func (s rowBudgetPayloadScanner) scanCurrentRowValues(rows *sql.Rows) ([]interface{}, error) {
	row, err := s.scanCurrentPreviewRow(rows)
	if err != nil {
		return nil, err
	}
	return []interface{}{row["payload"]}, nil
}

func TestScanRowsBoundsWideFieldsAndTotalBytes(t *testing.T) {
	t.Run("field preview", func(t *testing.T) {
		counter := &rowBudgetFakeCounter{}
		rows := openRowBudgetFakeRows(t, openRowBudgetFakeDB(t, counter, 1), "SELECT payload")
		budget := NewRowBudgetWithOptions(RowBudgetOptions{
			MaxRowsPerResult: 10,
			MaxTotalRows:     10,
			MaxTotalBytes:    1 << 20,
			MaxFieldBytes:    16,
		})

		data, _, truncated, err := scanRowsWithScanner(
			rows,
			[]string{"payload"},
			rowBudgetPayloadScanner{payload: strings.Repeat("x", 100)},
			true,
			budget,
		)
		if err != nil {
			t.Fatalf("scanRowsWithScanner: %v", err)
		}
		preview, ok := data[0]["payload"].(string)
		if !ok || !strings.HasPrefix(preview, "[TEXT preview: 16/100 bytes] "+strings.Repeat("x", 16)) {
			t.Fatalf("wide field preview = %#v", data[0]["payload"])
		}
		if !truncated || budget.Exhausted() {
			t.Fatalf("field preview truncation state = truncated=%v exhausted=%v", truncated, budget.Exhausted())
		}
	})

	t.Run("total bytes", func(t *testing.T) {
		payload := strings.Repeat("y", 64)
		rowBytes := estimateQueryRowBytes(map[string]interface{}{"payload": payload})
		counter := &rowBudgetFakeCounter{}
		rows := openRowBudgetFakeRows(t, openRowBudgetFakeDB(t, counter, 2), "SELECT payload")
		budget := NewRowBudgetWithOptions(RowBudgetOptions{
			MaxRowsPerResult: 10,
			MaxTotalRows:     10,
			MaxTotalBytes:    rowBytes,
			MaxFieldBytes:    1024,
		})

		data, _, truncated, err := scanRowsWithScanner(
			rows,
			[]string{"payload"},
			rowBudgetPayloadScanner{payload: payload},
			true,
			budget,
		)
		if err != nil {
			t.Fatalf("scanRowsWithScanner: %v", err)
		}
		if len(data) != 1 || !truncated || !budget.Exhausted() {
			t.Fatalf("total-byte budget returned rows=%d truncated=%v exhausted=%v", len(data), truncated, budget.Exhausted())
		}
	})
}

func TestBuildQueryFieldPreviewBoundsBinaryAndStructuredValues(t *testing.T) {
	tests := []struct {
		name       string
		value      interface{}
		wantPrefix string
	}{
		{name: "binary", value: []byte(strings.Repeat("b", 32)), wantPrefix: "[BINARY preview: 8/32 bytes] 0x"},
		{name: "json", value: map[string]interface{}{"payload": strings.Repeat("j", 32)}, wantPrefix: "[JSON preview: 8/"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			preview, truncated := buildQueryFieldPreview(test.value, 8)
			previewText, ok := preview.(string)
			if !truncated || !ok || !strings.HasPrefix(previewText, test.wantPrefix) {
				t.Fatalf("preview = %#v truncated=%v", preview, truncated)
			}
		})
	}
}
