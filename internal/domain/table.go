package domain

import "github.com/ealink1/navi-fyne/internal/upstream/connection"

// TableInfo carries driver metadata independently of the UI toolkit.
type TableInfo struct {
	Columns     []connection.ColumnDefinition
	Indexes     []connection.IndexDefinition
	ForeignKeys []connection.ForeignKeyDefinition
	Triggers    []connection.TriggerDefinition
	DDL         string
	Warnings    []string
}

// Filter defines one typed condition in the table browser.
type Filter struct{ Column, Operator, Value, Join string }

// Sort defines a validated column ordering.
type Sort struct {
	Column     string
	Descending bool
}

// TableRequest defines a bounded server-side page of a canonical object.
type TableRequest struct {
	Revision   int64
	Object     Object
	Page, Size int
	Filters    []Filter
	Sorts      []Sort
	Condition  string
}

// TablePage preserves total-count and metadata alongside the bounded result.
type TablePage struct {
	Revision   int64
	Info       TableInfo
	Result     Result
	Total      int64
	Page, Size int
}
