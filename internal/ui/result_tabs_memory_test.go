package ui

import (
	"testing"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func TestResultTabsReuseButtonsAcrossCountsSelectionAndReordering(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(Theme{})
	first := container.NewTabItem("结果", widget.NewLabel("one"))
	second := container.NewTabItem("日志", widget.NewLabel("two"))
	tabs := newResultTabs(first, second)
	renderer := test.WidgetRenderer(tabs)
	renderer.MinSize()
	initialFirst, initialSecond := tabs.buttons[first], tabs.buttons[second]
	for i := range 100 {
		tabs.setCount(first, i)
		tabs.SelectIndex(i % 2)
	}
	if tabs.buttons[first] != initialFirst || tabs.buttons[second] != initialSecond {
		t.Fatal("refresh replaced keyboard-focusable tab buttons")
	}
	if initialFirst.button.Text != "结果  99" {
		t.Fatal("count did not update", initialFirst.button.Text)
	}
	tabs.SetItems([]*container.TabItem{second, first})
	test.Tap(initialFirst.button)
	if tabs.Selected() != first {
		t.Fatal("reordered button selected its previous numeric index")
	}
	style := resultButtonTheme{state: initialFirst.state}
	if style.Color(theme.ColorNameForeground, theme.VariantLight) != theme.PrimaryColor() {
		t.Fatal("selection highlight is stale")
	}
	tabs.SetItems([]*container.TabItem{second})
	if _, ok := tabs.buttons[first]; ok {
		t.Fatal("removed tab button retained")
	}
	if initialFirst.button.OnTapped != nil {
		t.Fatal("removed button still retains its tab selection callback")
	}
}
