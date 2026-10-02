package ui

import (
	"testing"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestOutputTabsKeepValidSelectionAndRefreshActualSelectedContent(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	window := app.NewWindow("tabs")
	first, second := container.NewTabItem("日志", widget.NewLabel("one")), container.NewTabItem("结果", widget.NewLabel("two"))
	tabs := newResultTabs(first, second)
	window.SetContent(tabs)
	window.Show()
	tabs.Select(second)
	tabs.SelectIndex(200)
	if tabs.Selected() != second || tabs.body.Objects[0] != second.Content {
		t.Fatal("invalid index or refresh detached selected page")
	}
	tabs.SetItems([]*container.TabItem{second, first})
	if tabs.Selected() != second || tabs.SelectedIndex() != 0 {
		t.Fatal("reordering changed selected result identity")
	}
	tabs.SetItems(nil)
	if tabs.Selected() != nil || tabs.SelectedIndex() != -1 || len(tabs.body.Objects) != 0 {
		t.Fatal("empty output left stale content")
	}
	window.Close()
}
