package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/ealink1/super-link/internal/domain"
)

func tableCell(t *testing.T, table *dataGrid, row, col int) *gridCell {
	t.Helper()
	c := table.CreateCell().(*gridCell)
	table.UpdateCell(widget.TableCellID{Row: row, Col: col}, c)
	c.Resize(fyne.NewSize(180, 26))
	return c
}
func TestGridBlurStagesTextAndPageActionsCannotLoseActiveEditor(t *testing.T) {
	w, p := parityWindow(t)
	page := w.openTable(p, domain.Object{Name: "items", Schema: "public", Scope: "main", Kind: "table"})
	waitUI(t, w)
	c := tableCell(t, page.grid, 0, 3)
	test.DoubleTap(c)
	c.entry.SetText("changed without Enter")
	if !page.dirty() || page.commitButton.Disabled() {
		t.Fatal("active input invisible to submission")
	}
	page.gotoPage(2)
	if page.request.Page != 1 || page.cellValue(0, 1) != "changed without Enter" || c.editing {
		t.Fatal("paging lost editor input")
	}
	c = tableCell(t, page.grid, 1, 3)
	test.DoubleTap(c)
	c.entry.FocusLost()
	if _, exists := page.edits[1]; exists {
		t.Fatal("untouched NULL changed into empty text")
	}
	c = tableCell(t, page.grid, 0, 2)
	test.DoubleTap(c)
	c.entry.SetText("invalid-number")
	page.previewChanges()
	if !c.editing || !page.grid.hasPendingEditor() {
		t.Fatal("invalid numeric draft discarded")
	}
	c.entry.SetText("3")
	c.entry.FocusLost()
	if page.cellValue(0, 0) != int64(3) {
		t.Fatal("blur did not stage valid integer")
	}
}

func TestDesignerBlurStagesFieldsAndClosedGridCannotChangeState(t *testing.T) {
	w, p := parityWindow(t)
	info, _ := (parityFixture{}).TableInfo(t.Context(), "main", "items")
	w.designTable(p, domain.Object{Name: "items", Kind: "table"}, info)
	d := w.designers[w.tabs.Selected()]
	c := tableCell(t, d.grid, 1, 2)
	test.DoubleTap(c)
	c.entry.SetText("renamed")
	c.entry.FocusLost()
	if d.fields[1].column.Name != "renamed" || !d.dirty() {
		t.Fatal("designer blur lost change")
	}
	c = tableCell(t, d.grid, 1, 2)
	test.DoubleTap(c)
	c.entry.SetText("stale")
	d.closed = true
	if c.commitEditor() == nil || d.fields[1].column.Name != "renamed" {
		t.Fatal("closed designer accepted stale input")
	}
}

func TestVirtualizedInvalidEditorStaysBoundToOriginalCell(t *testing.T) {
	w, p := parityWindow(t)
	page := w.openTable(p, domain.Object{Name: "items", Schema: "public", Scope: "main", Kind: "table"})
	waitUI(t, w)
	c := tableCell(t, page.grid, 0, 2)
	test.DoubleTap(c)
	c.entry.SetText("invalid-number")
	page.grid.UpdateCell(widget.TableCellID{Row: 1, Col: 2}, c)
	if !page.dirty() || page.stageEditors() || len(page.edits) != 0 {
		t.Fatal("recycled editor lost validation or wrote to another row")
	}
	page.grid.UpdateCell(widget.TableCellID{Row: 0, Col: 2}, c)
	test.DoubleTap(c)
	if c.entry.Text != "invalid-number" {
		t.Fatal("virtualized draft text disappeared")
	}
	c.entry.SetText("5")
	c.entry.FocusLost()
	if page.cellValue(0, 0) != int64(5) || page.cellValue(1, 0) != int64(2) {
		t.Fatal("recycled correction targeted the wrong row")
	}
	page.clearEdits()
	if page.dirty() {
		t.Fatal("discard left an invisible editor draft")
	}
}
