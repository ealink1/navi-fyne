package domain

import (
	"fmt"
	"strings"
	"testing"
)

func TestNotebookValidatesGroupsAndBudgets(t *testing.T) {
	tests := []struct {
		name string
		book Notebook
	}{
		{"duplicate groups", Notebook{Groups: []NoteGroup{{ID: "a", Name: "Operations"}, {ID: "b", Name: " operations "}}}},
		{"reserved default", Notebook{Groups: []NoteGroup{{ID: "a", Name: "默认分组"}}}},
		{"reserved all", Notebook{Groups: []NoteGroup{{ID: "a", Name: "全部笔记"}}}},
		{"unknown group", Notebook{Notes: []Note{{ID: "one", GroupID: "missing"}}}},
		{"duplicate note", Notebook{Notes: []Note{{ID: "one"}, {ID: "one"}}}},
		{"large body", Notebook{Notes: []Note{{ID: "one", Body: strings.Repeat("x", MaxNoteBody+1)}}}},
		{"invalid utf8", Notebook{Notes: []Note{{ID: "one", Body: string([]byte{0xff})}}}},
	}
	total := Notebook{}
	for i := range 25 {
		total.Notes = append(total.Notes, Note{ID: fmt.Sprint(i), Body: strings.Repeat("x", MaxNoteBody)})
	}
	tests = append(tests, struct {
		name string
		book Notebook
	}{"whole library", total})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.book.Validate(); err == nil {
				t.Fatal("invalid notebook accepted")
			}
		})
	}
	original := Notebook{Groups: []NoteGroup{{ID: "a", Name: "Operations"}}, Notes: []Note{{ID: "one", GroupID: "a", Body: "kept"}}}
	clone := original.Clone()
	clone.Notes[0].Body = "changed"
	clone.Groups[0].Name = "renamed"
	if original.Notes[0].Body != "kept" || original.Groups[0].Name != "Operations" {
		t.Fatal("snapshot aliases original slices")
	}
	if err := original.Validate(); err != nil {
		t.Fatal(err)
	}
}
