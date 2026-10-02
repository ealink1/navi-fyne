package domain

// RowChange contains a complete original row for optimistic concurrency checks.
type RowChange struct {
	Kind     string // insert, update or delete
	Original map[string]any
	Values   map[string]any
}

// TableChanges binds staged rows to the profile revision that produced them.
type TableChanges struct {
	Import       bool
	Object       Object
	Revision     int64
	Rows         []RowChange
	Confirmation string
}

// CommittedWarning reports a successful server write with failed local auditing.
// Callers must clear the submitted batch and must not retry it as a failed write.
type CommittedWarning struct{ Cause error }

func (w *CommittedWarning) Error() string {
	return "changes committed, but local audit could not be saved: " + w.Cause.Error()
}
func (w *CommittedWarning) Unwrap() error { return w.Cause }

// Statement separates SQL structure from values sent through the driver binder.
type Statement struct {
	SQL        string
	Args       []any
	RequireOne bool
}

// RolledBackError certifies that the driver confirmed rollback. The session can
// remain open, which is essential for an in-memory database's lifetime.
type RolledBackError struct{ Cause error }

func (e *RolledBackError) Error() string { return "transaction rolled back: " + e.Cause.Error() }
func (e *RolledBackError) Unwrap() error { return e.Cause }
