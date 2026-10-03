package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ealink1/super-link/internal/domain"
)

// Shell hosts use their own table and do not change SQL connection records.
func (s *Store) ensureShell(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS shell_hosts(id TEXT PRIMARY KEY, metadata TEXT NOT NULL, secret_ref TEXT NOT NULL, revision INTEGER NOT NULL)`)
	return err
}

// ShellHosts lists public host metadata without reading credentials.
func (s *Store) ShellHosts(ctx context.Context) ([]domain.ShellHost, error) {
	if err := s.ensureShell(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT metadata,secret_ref,revision FROM shell_hosts ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hosts := []domain.ShellHost{}
	for rows.Next() {
		var h domain.ShellHost
		var raw string
		if err := rows.Scan(&raw, &h.SecretRef, &h.Revision); err != nil {
			return nil, err
		}
		ref, revision := h.SecretRef, h.Revision
		if err := json.Unmarshal([]byte(raw), &h); err != nil {
			return nil, err
		}
		h.SecretRef, h.Revision = ref, revision
		hosts = append(hosts, h)
	}
	return hosts, rows.Err()
}

// ShellHost loads one host, including its opaque credential reference.
func (s *Store) ShellHost(ctx context.Context, id string) (domain.ShellHost, error) {
	if err := s.ensureShell(ctx); err != nil {
		return domain.ShellHost{}, err
	}
	var h domain.ShellHost
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT metadata,secret_ref,revision FROM shell_hosts WHERE id=?", id).Scan(&raw, &h.SecretRef, &h.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return h, domain.ErrNotFound
	}
	if err != nil {
		return h, err
	}
	ref, revision := h.SecretRef, h.Revision
	err = json.Unmarshal([]byte(raw), &h)
	h.SecretRef, h.Revision = ref, revision
	return h, err
}

// SaveShellHost prevents stale editors from overwriting a newer host.
func (s *Store) SaveShellHost(ctx context.Context, h domain.ShellHost) (domain.ShellHost, error) {
	if err := s.ensureShell(ctx); err != nil {
		return h, err
	}
	raw, err := json.Marshal(h)
	if err != nil {
		return h, err
	}
	var result sql.Result
	if h.Revision == 0 {
		result, err = s.db.ExecContext(ctx, "INSERT INTO shell_hosts(id,metadata,secret_ref,revision) VALUES(?,?,?,1) ON CONFLICT(id) DO NOTHING", h.ID, string(raw), h.SecretRef)
	} else {
		result, err = s.db.ExecContext(ctx, "UPDATE shell_hosts SET metadata=?,secret_ref=?,revision=revision+1 WHERE id=? AND revision=?", string(raw), h.SecretRef, h.ID, h.Revision)
	}
	if err != nil {
		return h, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return h, err
	}
	if count != 1 {
		return h, domain.ErrConflict
	}
	h.Revision++
	return h, nil
}

// DeleteShellHost removes a host only if its revision still matches.
func (s *Store) DeleteShellHost(ctx context.Context, id string, revision int64) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM shell_hosts WHERE id=? AND revision=?", id, revision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return domain.ErrConflict
	}
	return nil
}
