package db

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestOptionalDriverAgentQueryPropagatesAndAccountsForBudget(t *testing.T) {
	var stdin optionalAgentTestWriteCloser
	client := &optionalDriverAgentClient{
		stdin:  &stdin,
		reader: bufio.NewReader(strings.NewReader(`{"id":1,"success":true,"data":[{"payload":"preview"}],"fields":["payload"],"truncated":true}` + "\n")),
		driver: "duckdb",
	}
	database := &OptionalDriverAgentDB{driverType: "duckdb", client: client}
	budget := NewRowBudgetWithOptions(RowBudgetOptions{
		MaxRowsPerResult: 2,
		MaxTotalRows:     2,
		MaxTotalBytes:    4096,
		MaxFieldBytes:    16,
	})
	ctx := ContextWithRowBudget(context.Background(), budget)

	rows, fields, err := database.QueryContext(ctx, "SELECT payload FROM items")
	if err != nil {
		t.Fatalf("budgeted agent query: %v", err)
	}
	if len(rows) != 1 || len(fields) != 1 || fields[0] != "payload" {
		t.Fatalf("agent query result = rows=%#v fields=%#v", rows, fields)
	}
	var request optionalAgentRequest
	if err := json.Unmarshal(bytes.TrimSpace(stdin.Bytes()), &request); err != nil {
		t.Fatalf("decode budgeted agent request: %v", err)
	}
	if request.RowBudget == nil || *request.RowBudget != budget.Options() {
		t.Fatalf("agent request budget = %#v", request.RowBudget)
	}
	if remaining := budget.RemainingOptions(); remaining.MaxTotalRows != 1 {
		t.Fatalf("remaining total rows = %d, want 1", remaining.MaxTotalRows)
	}
	if !budget.TakeResultTruncated() {
		t.Fatal("agent truncation flag was not propagated to the main-process budget")
	}
}
