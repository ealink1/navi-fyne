package ui

import (
	"embed"
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

//go:embed assets/shell/*.svg
var shellIconFiles embed.FS

func shellIcon(name string, active bool) fyne.Resource {
	raw, err := shellIconFiles.ReadFile("assets/shell/" + name + ".svg")
	if err != nil {
		return icon(name)
	}
	shade := theme.ColorNamePlaceHolder
	if active {
		shade = theme.ColorNamePrimary
	}
	return &shellOutlineIcon{name: name, source: raw, shade: shade}
}

// Fyne's fill recoloring drops inherited fill="none" and does not recolor
// strokes. Resolve only currentColor, preserving every outline and SVG element.
type shellOutlineIcon struct {
	name   string
	source []byte
	shade  fyne.ThemeColorName
}

func (r *shellOutlineIcon) color() string {
	value := color.NRGBAModel.Convert(resolveShellColor(shellColor(r.shade))).(color.NRGBA)
	return fmt.Sprintf("#%02x%02x%02x", value.R, value.G, value.B)
}

func (r *shellOutlineIcon) Name() string { return "shell-" + r.name + "-" + r.color() + ".svg" }
func (r *shellOutlineIcon) Content() []byte {
	return []byte(strings.ReplaceAll(string(r.source), "currentColor", r.color()))
}

type shellTheme struct{ palette *appearancePalette }

func newShellTheme() shellTheme {
	if app := fyne.CurrentApp(); app != nil {
		if source, ok := app.Settings().Theme().(interface{ appearancePalette() *appearancePalette }); ok {
			return shellTheme{palette: source.appearancePalette()}
		}
	}
	return shellTheme{palette: &appearancePalette{}}
}

func (t shellTheme) appearancePalette() *appearancePalette { return t.palette }

func (t shellTheme) shared() Theme {
	return Theme{Dark: t.palette != nil && t.palette.dark.Load()}
}

func (t shellTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNameShadow {
		return color.Transparent
	}
	return t.shared().Color(name, variant)
}

func (t shellTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameText:
		return 13
	case theme.SizeNameHeadingText:
		return 18
	case theme.SizeNameSubHeadingText:
		return 15
	case theme.SizeNamePadding:
		return 0
	case theme.SizeNameInnerPadding:
		return 5
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius:
		return 4
	case theme.SizeNamePopupRadius:
		return 12
	case theme.SizeNameInputBorder, theme.SizeNameSeparatorThickness:
		return 1
	case theme.SizeNameModalBlurRadius:
		return 0 // Avoid a full-window blur allocation; the modal has a dim backdrop.
	}
	return Theme{}.Size(name)
}

func (t shellTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return t.shared().Icon(name)
}

// Keep the shared CJK fonts and the Shell's compact layout.
func (t shellTheme) Font(style fyne.TextStyle) fyne.Resource { return Theme{}.Font(style) }
