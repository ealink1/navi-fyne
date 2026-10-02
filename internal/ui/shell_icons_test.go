package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

func TestShellOutlineIconsRenderAcrossAppearanceChanges(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	palette := &appearancePalette{}
	server := shellIcon("server", false)
	terminal := shellIcon("terminal", true)
	window := app.NewWindow("outline icon regression")
	defer window.Close()
	window.SetPadded(false)
	image := canvas.NewImageFromResource(server)
	image.FillMode = canvas.ImageFillContain
	image.SetMinSize(fyne.NewSquareSize(48))
	window.SetContent(container.NewStack(image))
	window.Resize(fyne.NewSquareSize(48))
	window.Show()
	previousName := ""
	for _, dark := range []bool{false, true, false} {
		palette.dark.Store(dark)
		app.Settings().SetTheme(Theme{Dark: dark, palette: palette})
		if server.Name() == previousName {
			t.Fatal("icon texture cache key did not change with the appearance")
		}
		previousName = server.Name()
		image.Resource = server
		image.Refresh()
		capture := window.Canvas().Capture()
		size := capture.Bounds().Size()
		background := resolveShellColor(shellBackground)
		center := color.NRGBAModel.Convert(capture.At(size.X/2, size.Y/4)).(color.NRGBA)
		if center != background {
			t.Fatalf("server outline filled its interior in dark=%v: %v, want %v", dark, center, background)
		}
		stroke := color.NRGBAModel.Convert(capture.At(size.X/2, size.Y/12)).(color.NRGBA)
		if stroke == background {
			t.Fatalf("server outline disappeared in dark=%v", dark)
		}
		image.Resource = terminal
		image.Refresh()
		capture = window.Canvas().Capture()
		stroke = color.NRGBAModel.Convert(capture.At(size.X*2/3, size.Y*19/24)).(color.NRGBA)
		if stroke != resolveShellColor(shellColor(theme.ColorNamePrimary)) {
			t.Fatalf("terminal stroke does not follow the shared accent in dark=%v: %v", dark, stroke)
		}
	}
}
