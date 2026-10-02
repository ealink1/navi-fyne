package domain

// RowConsumer consumes one ordered result set without retaining all rows.
type RowConsumer interface {
	SetColumns([]string) error
	ConsumeRowValues([]any) error
}
