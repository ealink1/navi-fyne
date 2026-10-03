package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// Note is a local Markdown document; Deleted keeps it recoverable in Trash.
type Note struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	GroupID   string    `json:"groupId"`
	Tags      string    `json:"tags"`
	UpdatedAt time.Time `json:"updatedAt"`
	Deleted   bool      `json:"deleted"`
}

// NoteGroup is an explicitly named collection; the default group has no ID.
type NoteGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Notebook is an encrypted atomic snapshot, including recoverable notes.
type Notebook struct {
	Notes    []Note      `json:"notes"`
	Groups   []NoteGroup `json:"groups"`
	Revision int64       `json:"-"`
}

// Notebook storage budgets bound decryption, searching and rendering memory.
const (
	MaxNoteBody      = 256 << 10
	MaxNotebookBytes = 6 << 20
	MaxNotes         = 300
	MaxNoteGroups    = 50
)

// Validate checks references and budgets without disclosing document contents.
func (b Notebook) Validate() error {
	if len(b.Notes) > MaxNotes || len(b.Groups) > MaxNoteGroups {
		return errors.New("笔记或分组数量超过限制")
	}
	groups := map[string]bool{"": true}
	names := map[string]bool{"默认分组": true, "全部笔记": true}
	for _, g := range b.Groups {
		name := strings.ToLower(strings.TrimSpace(g.Name))
		if g.ID == "" || groups[g.ID] || name == "" || names[name] || utf8.RuneCountInString(g.Name) > 80 || !utf8.ValidString(g.Name) {
			return errors.New("笔记分组无效或重名")
		}
		groups[g.ID] = true
		names[name] = true
	}
	ids := map[string]bool{}
	for _, n := range b.Notes {
		if n.ID == "" || ids[n.ID] || !groups[n.GroupID] {
			return errors.New("笔记标识或分组无效")
		}
		if len(n.Body) > MaxNoteBody || utf8.RuneCountInString(n.Title) > 200 || len(n.Tags) > 512 || !utf8.ValidString(n.Title+n.Body+n.Tags) {
			return errors.New("笔记格式无效，标题最多 200 字，正文最多 256 KiB")
		}
		ids[n.ID] = true
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return err
	}
	if len(raw) > MaxNotebookBytes {
		return errors.New("笔记本超过 6 MiB，请先导出并整理")
	}
	return nil
}

// Clone owns slice storage; immutable strings can be shared between snapshots.
func (b Notebook) Clone() Notebook {
	b.Notes = append([]Note(nil), b.Notes...)
	b.Groups = append([]NoteGroup(nil), b.Groups...)
	return b
}
