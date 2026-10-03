package ui

import (
	"context"
	"fmt"
	"sync/atomic"

	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const appearanceSettingKey = "ui.appearance"

// UI state stays on the Fyne goroutine; only immutable snapshots reach storage.
type appearanceState struct {
	palette  *appearancePalette
	revision uint64
	saving   bool
	check    *widget.Check
}

// Theme overrides hold only this small display state, never the Window.
type appearancePalette struct{ dark atomic.Bool }

func (w *Window) load() {
	w.jobs.run(func(ctx context.Context) (any, error) {
		return w.Store.Setting(ctx, appearanceSettingKey)
	}, func(value any, err error) {
		if err != nil {
			w.status.SetText("无法读取主题设置：" + err.Error())
		} else if w.appearance.revision == 0 {
			w.applyAppearance(value.(string) == "dark")
		}
		w.loadProfiles()
	})
}

func (w *Window) toggleAppearance() { w.setDark(!w.dark) }

func (w *Window) setDark(dark bool) {
	if w.shuttingDown || w.dark == dark {
		return
	}
	w.appearance.revision++
	w.applyAppearance(dark)
	w.persistAppearance()
}

func (w *Window) applyAppearance(dark bool) {
	w.dark = dark
	w.appearance.palette.dark.Store(dark)
	w.App.Settings().SetTheme(Theme{Dark: dark, palette: w.appearance.palette})
	if check := w.appearance.check; check != nil {
		changed := check.OnChanged
		check.OnChanged = nil
		check.SetChecked(dark)
		check.OnChanged = changed
	}
	if w.switcher != nil {
		w.switcher.refreshAppearance()
	}
	// Concrete canvas colors must change too, so cached glyph textures use a
	// new color key rather than keeping a dynamic color with the old texture.
	w.Window.Content().Refresh()
	for _, overlay := range w.Window.Canvas().Overlays().List() {
		overlay.Refresh()
	}
}

func (w *Window) appearanceCheck() *widget.Check {
	if w.appearance.check == nil {
		check := widget.NewCheck("夜间模式（SQL / Shell / Note 同步）", nil)
		check.SetChecked(w.dark)
		check.OnChanged = w.setDark
		w.appearance.check = check
	}
	return w.appearance.check
}

func (w *Window) persistAppearance() {
	if w.appearance.saving {
		return
	}
	w.appearance.saving = true
	revision, dark := w.appearance.revision, w.dark
	w.jobs.run(func(ctx context.Context) (any, error) {
		return nil, w.saveAppearance(ctx, dark)
	}, func(_ any, err error) {
		w.appearance.saving = false
		if err != nil {
			w.status.SetText(err.Error())
		}
		if revision != w.appearance.revision {
			w.persistAppearance()
		}
	})
}

func (w *Window) saveAppearance(ctx context.Context, dark bool) error {
	mode := "light"
	if dark {
		mode = "dark"
	}
	if err := w.Store.SetSetting(ctx, appearanceSettingKey, mode); err != nil {
		return fmt.Errorf("保存日间 / 夜间设置：%w", err)
	}
	return nil
}

func (s *workspaceSwitcher) refreshAppearance() {
	label := "日间"
	if s.owner.dark {
		label = "夜间"
	}
	s.themeButton.SetText(label)
	s.themeButton.SetIcon(theme.ColorPaletteIcon())
	if s.nativeSelect != nil {
		s.nativeSelect(s.mode, s.owner.dark)
	}
	if s.note != nil {
		s.note.content.Refresh()
	}
	if s.shell != nil {
		s.shell.content.Refresh()
		for _, pane := range s.shell.panes {
			pane.item.Content.Refresh()
		}
	}
}
