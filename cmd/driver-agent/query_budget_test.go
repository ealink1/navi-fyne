package main

import (
	"testing"

	"github.com/ealink1/super-link/internal/upstream/db"
)

func TestHandleRequestPassesRowBudgetToDriverContext(t *testing.T) {
	fake := &fakeAgentTimeoutDB{}
	runtimeState := &agentRuntime{inst: fake, sessions: make(map[string]db.StatementExecer)}
	options := db.RowBudgetOptions{
		MaxRowsPerResult: 12,
		MaxTotalRows:     18,
		MaxTotalBytes:    4096,
		MaxFieldBytes:    512,
	}

	response := handleRequest(runtimeState, agentRequest{
		ID:        41,
		Method:    agentMethodQuery,
		Query:     "SELECT payload FROM items",
		RowBudget: &options,
	})
	if !response.Success {
		t.Fatalf("budgeted agent query failed: %s", response.Error)
	}
	if !fake.queryContextCalled || fake.queryCalled {
		t.Fatalf("budgeted agent query path = QueryContext:%v Query:%v", fake.queryContextCalled, fake.queryCalled)
	}
	if fake.rowBudget == nil || fake.rowBudget.Options() != options {
		t.Fatalf("agent row budget = %#v", fake.rowBudget)
	}
}
