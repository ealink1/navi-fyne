package container

import (
	"runtime"
	"testing"
	"weak"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/internal/cache"
	intTheme "fyne.io/fyne/v2/internal/theme"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func cachedChildTheme() (fyne.Theme, weak.Pointer[ThemeOverride]) {
	child := canvas.NewRectangle(nil)
	owner := NewThemeOverride(child, test.Theme())
	return cache.WidgetTheme(child), weak.Make(owner)
}

func TestCachedChildThemeDoesNotRetainItsContainer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	cached, owner := cachedChildTheme()
	for range 3 {
		runtime.GC()
	}
	if owner.Value() != nil {
		t.Fatal("child theme keeps the removed ThemeOverride and its content alive")
	}
	if cached.(*featureTheme).Feature(intTheme.FeatureNameDeviceIsMobile) != false {
		t.Fatal("cached feature state changed after container collection")
	}
	runtime.KeepAlive(cached)
}

func TestCachedFeatureStateTracksDeviceChanges(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	owner := NewThemeOverride(canvas.NewRectangle(nil), test.Theme())
	cached := cache.WidgetTheme(owner.Content).(*featureTheme)
	for _, mobile := range []bool{true, false, true} {
		owner.SetDeviceIsMobile(mobile)
		if cached.Feature(intTheme.FeatureNameDeviceIsMobile) != mobile {
			t.Fatal("existing child scope has stale device features")
		}
	}
}

func TestDestroyedOverrideReleasesRetiredChildrenAndPreservesNestedThemes(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	retired := canvas.NewRectangle(nil)
	nestedChild := widget.NewLabel("nested")
	nested := NewThemeOverride(nestedChild, test.Theme())
	content := NewHBox(retired, nested)
	owner := NewThemeOverride(content, test.Theme())
	cache.Renderer(owner)
	nestedTheme := cache.WidgetTheme(nestedChild)
	content.Objects = []fyne.CanvasObject{nested}
	owner.Refresh()
	if cache.WidgetTheme(retired) == nil {
		t.Fatal("test did not retain a retired child scope")
	}
	cache.DestroyRenderer(owner)
	if cache.WidgetTheme(retired) != nil {
		t.Fatal("expired theme renderer retained its removed canvas objects")
	}
	if cache.WidgetTheme(nestedChild) != nestedTheme {
		t.Fatal("parent cleanup removed an independently owned nested theme")
	}
	// Expired hidden pages can be shown again; rebuilding reapplies their theme.
	content.Objects = []fyne.CanvasObject{retired, nested}
	cache.Renderer(owner)
	if cache.WidgetTheme(retired) == nil {
		t.Fatal("reopening the page did not restore its theme")
	}
}
