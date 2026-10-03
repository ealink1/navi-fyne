package ui

import (
	"testing"

	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/upstream/connection"
)

func TestDesignerPendingIndexCanBeRemovedAndDropCanBeUndone(t *testing.T) {
	w, p := parityWindow(t)
	info, _ := (parityFixture{}).TableInfo(t.Context(), "main", "items")
	info.Indexes = []connection.IndexDefinition{{Name: "existing", ColumnName: "id"}, {Name: "existing", ColumnName: "name"}}
	w.designTable(p, domain.Object{Name: "items", Kind: "table"}, info)
	d := w.designers[w.tabs.Selected()]
	captureParity(t, w, "parity-structure.png")
	index := domain.NewIndex{Name: "idx_name", Method: "BTREE", Columns: []domain.IndexColumn{{Name: "name"}}}
	if !d.stageNewIndex(index) || d.stageNewIndex(index) {
		t.Fatal("index validation did not reject duplicates")
	}
	d.removeIndexes(map[int]bool{2: true}, false)
	if len(d.indexChanges) != 0 {
		t.Fatal("deleting new index left a CREATE or nonexistent DROP")
	}
	d.removeIndexes(map[int]bool{0: true, 1: true}, false)
	if len(d.indexChanges) != 1 || d.indexChanges[0].OriginalName != "existing" {
		t.Fatal("composite index created duplicate drops")
	}
	d.removeIndexes(map[int]bool{1: true}, true)
	if len(d.indexChanges) != 0 || d.dirty() {
		t.Fatal("undo left a pending index change")
	}
}
