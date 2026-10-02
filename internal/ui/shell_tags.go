package ui

import (
	"image/color"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
)

func shellProtocolBadge() fyne.CanvasObject {
	shade := shellTone{day: color.NRGBA{R: 14, G: 116, B: 144, A: 255}, night: color.NRGBA{R: 103, G: 232, B: 249, A: 255}}
	fill := shellTone{day: color.NRGBA{R: 207, G: 250, B: 254, A: 160}, night: color.NRGBA{R: 22, G: 78, B: 99, A: 80}}
	badge := shellPanel(shellText("SSH", 10, false, shade), fill, 4, 3)
	badge.Objects[0].(*shellPrimitive).stroke = color.NRGBA{R: 8, G: 145, B: 178, A: 160}
	return badge
}

type shellTagTheme struct {
	shellTheme
	shade color.Color
}

func (t shellTagTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNameForeground {
		return resolveShellColor(t.shade)
	}
	return t.shellTheme.Color(name, variant)
}
func (t shellTagTheme) Size(name fyne.ThemeSizeName) float32 {
	if name == theme.SizeNameText {
		return 11
	}
	return t.shellTheme.Size(name)
}

func (t shellTagTheme) Font(style fyne.TextStyle) fyne.Resource {
	style.Bold = false
	return t.shellTheme.Font(style)
}

func (s *shellWorkspace) buildTagFilters() fyne.CanvasObject {
	unique := make(map[string]bool)
	for _, host := range s.hosts {
		for _, tag := range strings.FieldsFunc(host.Tags, func(r rune) bool { return r == ',' || r == '，' }) {
			if name := strings.TrimSpace(tag); name != "" {
				unique[name] = true
			}
		}
	}
	names := make([]string, 0, len(unique))
	for name := range unique {
		names = append(names, name)
	}
	sort.Strings(names)
	day := []color.NRGBA{{R: 79, G: 70, B: 229, A: 255}, {R: 4, G: 120, B: 87, A: 255}, {R: 161, G: 98, B: 7, A: 255}, {R: 190, G: 24, B: 93, A: 255}, {R: 14, G: 116, B: 144, A: 255}}
	night := []color.NRGBA{{R: 129, G: 140, B: 248, A: 255}, {R: 52, G: 211, B: 153, A: 255}, {R: 251, G: 191, B: 36, A: 255}, {R: 244, G: 114, B: 182, A: 255}, {R: 34, G: 211, B: 238, A: 255}}
	chips := make([]fyne.CanvasObject, 0, len(names))
	for i, tag := range names {
		name := tag
		shade := shellTone{day: day[i%len(day)], night: night[i%len(night)]}
		button := shellButton(tag, "", false, func() {
			if s.tag == name {
				s.tag = ""
			} else {
				s.tag = name
			}
			s.filterHosts()
			s.showHosts()
		})
		fill := shade
		fill.day.A, fill.night.A = 28, 28
		if s.tag == tag {
			fill.day.A, fill.night.A = 80, 80
		}
		chip := shellPanel(container.NewThemeOverride(button, shellTagTheme{shellTheme: newShellTheme(), shade: shade}), fill, 10, 0)
		border := shade
		border.day.A, border.night.A = 90, 90
		chip.Objects[0].(*shellPrimitive).stroke = border
		chips = append(chips, shellFixed(chip, 0, 22))
	}
	var filters fyne.CanvasObject = shellText("尚无标签", 12, false, shellMutedColor)
	if len(chips) > 0 {
		filters = container.New(&shellTagFlowLayout{}, chips...)
	}
	return shellVBox(shellText("标签筛选", 11, true, shellMutedColor), shellFixed(layout.NewSpacer(), 0, 10), filters, shellFixed(layout.NewSpacer(), 0, 12))
}

type shellTagFlowLayout struct{}

func (l *shellTagFlowLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(200, l.height(objects, 200))
}
func (*shellTagFlowLayout) height(objects []fyne.CanvasObject, width float32) float32 {
	x, y := float32(0), float32(0)
	for _, object := range objects {
		w := min(width, object.MinSize().Width+8)
		if x > 0 && x+w > width {
			x, y = 0, y+28
		}
		x += w + 6
	}
	return y + 22
}
func (*shellTagFlowLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	x, y := float32(0), float32(0)
	for _, object := range objects {
		width := min(size.Width, object.MinSize().Width+8)
		if x > 0 && x+width > size.Width {
			x, y = 0, y+28
		}
		object.Move(fyne.NewPos(x, y))
		object.Resize(fyne.NewSize(width, 22))
		x += width + 6
	}
}
