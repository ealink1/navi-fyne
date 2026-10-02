package domain

type ImportRequest struct {
	Object       Object
	Revision     int64
	Columns      []string
	Rows         [][]any
	Mapping      map[int]string
	BatchSize    int
	Confirmation string
}
type ImportResult struct {
	Attempted bool
	Committed int64
	Batches   int
	Total     int
}
