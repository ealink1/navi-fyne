package domain

import (
	"errors"
	"time"
)

var ErrQueryConflict = errors.New("saved query changed; reload before saving")

// SavedQuery is an explicitly named SQL document, separate from autosaved drafts.
// It contains editor text and context, never parameter values or result rows.
type SavedQuery struct {
	ID        string
	ProfileID string
	Title     string
	Scope     string
	Schema    string
	Text      string
	Revision  int64
	UpdatedAt time.Time
}
