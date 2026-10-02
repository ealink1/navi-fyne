package ui

import (
	"runtime"
	"testing"
	"weak"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

func detachedEditorTheme() (codeTheme, weak.Pointer[codeEntry]) {
	entry := newCodeEntry()
	return codeTheme{state: entry.colors}, weak.Make(entry)
}

func detachedResultTheme() (resultButtonTheme, weak.Pointer[resultTabs]) {
	tabs := newResultTabs(container.NewTabItem("result", nil))
	state := &resultButtonState{selected: true}
	tabs.buttons[tabs.Items[0]] = resultTabButton{state: state}
	return resultButtonTheme{state: state}, weak.Make(tabs)
}

func TestCachedThemesDoNotRetainEditorsOrResultPages(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(Theme{})
	code, entry := detachedEditorTheme()
	result, tabs := detachedResultTheme()
	for range 3 {
		runtime.GC()
	}
	if entry.Value() != nil || tabs.Value() != nil {
		t.Fatal("cached display theme keeps a discarded editor or result model alive")
	}
	_, _, _, alpha := code.Color(theme.ColorNameForeground, theme.VariantLight).RGBA()
	if alpha != 0 || result.Color(theme.ColorNameForeground, theme.VariantLight) != theme.PrimaryColor() {
		t.Fatal("theme state was lost after its owner was collected")
	}
	runtime.KeepAlive(code)
	runtime.KeepAlive(result)
}

func TestEditorWrapPreservesNativeTextAndChangesDisplayLayers(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(Theme{})
	entry := newCodeEntry()
	container.NewThemeOverride(entry, codeTheme{state: entry.colors})
	entry.SetText("SELECT '中文' AS name;\n-- comment")
	for _, wrap := range []fyne.TextWrap{fyne.TextWrapOff, fyne.TextWrapWord, fyne.TextWrapOff} {
		entry.Wrapping = wrap
		entry.Refresh()
		_, _, _, alpha := (codeTheme{state: entry.colors}).Color(theme.ColorNameForeground, theme.VariantLight).RGBA()
		if entry.layer.Visible() != (wrap == fyne.TextWrapOff) || (alpha == 0) != (wrap == fyne.TextWrapOff) {
			t.Fatal("native text and syntax layer visibility disagree")
		}
		if entry.Text != "SELECT '中文' AS name;\n-- comment" {
			t.Fatal("wrapping changed source text")
		}
	}
}
