package state

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ealink1/super-link/internal/domain"
)

// NotebookRef describes only the encrypted snapshot, never document metadata.
type NotebookRef struct {
	Ref      string
	Revision int64
}

// NotebookReference reads the single atomic snapshot pointer.
func (s *Store) NotebookReference(ctx context.Context) (NotebookRef, error) {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS notebook(id INTEGER PRIMARY KEY CHECK(id=1), secret_ref TEXT NOT NULL, revision INTEGER NOT NULL)`); err != nil {
		return NotebookRef{}, err
	}
	var r NotebookRef
	err := s.db.QueryRowContext(ctx, "SELECT secret_ref,revision FROM notebook WHERE id=1").Scan(&r.Ref, &r.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return r, nil
	}
	return r, err
}

// PublishNotebook rejects stale snapshots and makes all note changes atomic.
func (s *Store) PublishNotebook(ctx context.Context, expected int64, ref string) (int64, error) {
	var result sql.Result
	var err error
	if expected == 0 {
		result, err = s.db.ExecContext(ctx, "INSERT INTO notebook(id,secret_ref,revision) VALUES(1,?,1) ON CONFLICT(id) DO NOTHING", ref)
	} else {
		result, err = s.db.ExecContext(ctx, "UPDATE notebook SET secret_ref=?,revision=revision+1 WHERE id=1 AND revision=?", ref, expected)
	}
	if err != nil {
		return expected, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return expected, err
	}
	if count != 1 {
		return expected, domain.ErrConflict
	}
	return expected + 1, nil
}
