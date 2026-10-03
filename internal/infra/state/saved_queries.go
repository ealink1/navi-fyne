package state

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ealink1/super-link/internal/domain"
)

func validateSavedQuery(q domain.SavedQuery) error {
	if q.ID == "" || q.ProfileID == "" || len(q.ID) > 128 || len(q.ProfileID) > 128 || q.Revision < 0 {
		return errors.New("invalid saved query identity")
	}
	for _, value := range []string{q.Title, q.Text, q.Scope, q.Schema} {
		if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return errors.New("saved query requires valid UTF-8 without NUL characters")
		}
	}
	if strings.TrimSpace(q.Title) == "" || len(q.Title) > 256 || strings.TrimSpace(q.Text) == "" || len(q.Text) > 1<<20 || len(q.Scope) > 1024 || len(q.Schema) > 256 {
		return errors.New("saved query requires a title (256 bytes maximum) and SQL (1 MiB maximum)")
	}
	return nil
}

// SaveQuery compares revisions so two open tabs cannot silently overwrite SQL.
func (s *Store) SaveQuery(ctx context.Context, q domain.SavedQuery) (domain.SavedQuery, error) {
	q.Title = strings.TrimSpace(q.Title)
	if err := validateSavedQuery(q); err != nil {
		return q, err
	}
	q.UpdatedAt = time.Now().UTC()
	var result sql.Result
	var err error
	if q.Revision == 0 {
		result, err = s.db.ExecContext(ctx, `INSERT INTO saved_queries(id,profile_id,title,scope,schema_name,text,revision,updated_at) SELECT ?,?,?,?,?,?,1,? WHERE (SELECT COUNT(*) FROM saved_queries)<500 ON CONFLICT(id) DO NOTHING`, q.ID, q.ProfileID, q.Title, q.Scope, q.Schema, q.Text, q.UpdatedAt.UnixMilli())
	} else {
		result, err = s.db.ExecContext(ctx, `UPDATE saved_queries SET title=?,scope=?,schema_name=?,text=?,revision=revision+1,updated_at=? WHERE id=? AND profile_id=? AND revision=?`, q.Title, q.Scope, q.Schema, q.Text, q.UpdatedAt.UnixMilli(), q.ID, q.ProfileID, q.Revision)
	}
	if err != nil {
		return q, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return q, err
	}
	if count != 1 {
		if q.Revision == 0 {
			return q, errors.New("saved query already exists or the 500-document limit has been reached")
		}
		return q, domain.ErrQueryConflict
	}
	q.Revision++
	return q, nil
}

// SavedQueries lists metadata without loading every SQL document into memory.
func (s *Store) SavedQueries(ctx context.Context, profileID string) ([]domain.SavedQuery, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,profile_id,title,scope,schema_name,revision,updated_at FROM saved_queries WHERE ?='' OR profile_id=? ORDER BY title COLLATE NOCASE,id`, profileID, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	queries := []domain.SavedQuery{}
	for rows.Next() {
		var query domain.SavedQuery
		var milliseconds int64
		if err = rows.Scan(&query.ID, &query.ProfileID, &query.Title, &query.Scope, &query.Schema, &query.Revision, &milliseconds); err != nil {
			return nil, err
		}
		query.UpdatedAt = time.UnixMilli(milliseconds).UTC()
		queries = append(queries, query)
	}
	return queries, rows.Err()
}

func (s *Store) SavedQuery(ctx context.Context, id string) (domain.SavedQuery, error) {
	var query domain.SavedQuery
	var milliseconds int64
	err := s.db.QueryRowContext(ctx, `SELECT id,profile_id,title,scope,schema_name,text,revision,updated_at FROM saved_queries WHERE id=?`, id).Scan(&query.ID, &query.ProfileID, &query.Title, &query.Scope, &query.Schema, &query.Text, &query.Revision, &milliseconds)
	if errors.Is(err, sql.ErrNoRows) {
		return query, domain.ErrNotFound
	}
	query.UpdatedAt = time.UnixMilli(milliseconds).UTC()
	return query, err
}

func (s *Store) DeleteQuery(ctx context.Context, id string, revision int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM saved_queries WHERE id=? AND revision=?`, id, revision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return domain.ErrQueryConflict
	}
	return nil
}
