package ui

import (
	"image/color"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	transport "github.com/ealink1/navi-fyne/internal/infra/shell"
)

var (
	shellConnectBlue  = shellTone{day: color.NRGBA{R: 75, G: 112, B: 207, A: 255}, night: color.NRGBA{R: 125, G: 157, B: 239, A: 255}}
	shellConnectGreen = shellTone{day: color.NRGBA{R: 76, G: 129, B: 39, A: 255}, night: color.NRGBA{R: 158, G: 203, B: 102, A: 255}}
	shellConnectPink  = shellTone{day: color.NRGBA{R: 217, G: 72, B: 106, A: 255}, night: color.NRGBA{R: 245, G: 112, B: 148, A: 255}}
)

const shellStepWaiting = 3

type shellConnectStep struct {
	widget.BaseWidget
	index int
	state int
}

func newShellConnectStep(index int) *shellConnectStep {
	step := &shellConnectStep{index: index, state: shellStepWaiting}
	step.ExtendBaseWidget(step)
	return step
}

func (s *shellConnectStep) shade() color.Color {
	switch s.state {
	case int(transport.ConnectStarted):
		return shellConnectBlue
	case int(transport.ConnectCompleted):
		return shellConnectGreen
	case int(transport.ConnectFailed):
		return shellConnectPink
	default:
		return shellMutedColor
	}
}

func (s *shellConnectStep) CreateRenderer() fyne.WidgetRenderer {
	r := &shellConnectStepRenderer{step: s, circle: canvas.NewCircle(color.Transparent), number: canvas.NewText("", color.White), icon: canvas.NewImageFromResource(nil), label: canvas.NewText(shellConnectTitles[s.index], color.White)}
	r.number.TextSize, r.label.TextSize = 13, 11
	r.number.Alignment, r.label.Alignment = fyne.TextAlignCenter, fyne.TextAlignCenter
	r.label.TextStyle.Bold = true
	r.icon.FillMode = canvas.ImageFillContain
	r.Refresh()
	return r
}

type shellConnectStepRenderer struct {
	step   *shellConnectStep
	circle *canvas.Circle
	number *canvas.Text
	icon   *canvas.Image
	label  *canvas.Text
}

func (*shellConnectStepRenderer) MinSize() fyne.Size { return fyne.NewSize(68, 58) }
func (r *shellConnectStepRenderer) Layout(size fyne.Size) {
	r.circle.Move(fyne.NewPos((size.Width-30)/2, 0))
	r.circle.Resize(fyne.NewSquareSize(30))
	r.number.Move(fyne.NewPos((size.Width-30)/2, 5))
	r.number.Resize(fyne.NewSize(30, 20))
	r.icon.Move(fyne.NewPos((size.Width-16)/2, 7))
	r.icon.Resize(fyne.NewSquareSize(16))
	r.label.Move(fyne.NewPos(0, 40))
	r.label.Resize(fyne.NewSize(size.Width, 18))
}
func (r *shellConnectStepRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.circle, r.number, r.icon, r.label}
}
func (*shellConnectStepRenderer) Destroy() {}
func (r *shellConnectStepRenderer) Refresh() {
	s := r.step
	r.circle.FillColor = resolveShellColor(s.shade())
	r.label.Color = resolveShellColor(s.shade())
	r.number.Text = strconv.Itoa(s.index + 1)
	r.number.Color = color.White
	r.number.Show()
	r.icon.Hide()
	if s.state == shellStepWaiting {
		r.circle.FillColor = resolveShellColor(shellRailColor)
		r.number.Color = resolveShellColor(shellMutedColor)
	} else if s.state != int(transport.ConnectFailed) {
		name := "refresh-cw"
		if s.state == int(transport.ConnectCompleted) {
			name = "check"
		}
		resource := shellIcon(name, false)
		resource.(*shellOutlineIcon).shade = theme.ColorNameForegroundOnPrimary
		r.icon.Resource = resource
		r.number.Hide()
		r.icon.Show()
	}
	r.circle.Refresh()
	r.number.Refresh()
	r.icon.Refresh()
	r.label.Refresh()
}

type shellConnectStepsLayout struct{}

func (*shellConnectStepsLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(304, 58) }
func (*shellConnectStepsLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	const stepWidth float32 = 68
	stride := max(0, size.Width-16-stepWidth) / 3
	for i := range 3 {
		objects[i].Move(fyne.NewPos(8+stepWidth/2+stride*float32(i)+22, 14))
		objects[i].Resize(fyne.NewSize(max(0, stride-44), 2))
	}
	for i := range 4 {
		objects[3+i].Move(fyne.NewPos(8+float32(i)*stride, 0))
		objects[3+i].Resize(fyne.NewSize(stepWidth, 58))
	}
}

func newShellConnectSteps() (*fyne.Container, [4]*shellConnectStep, [3]*shellPrimitive) {
	var steps [4]*shellConnectStep
	var lines [3]*shellPrimitive
	objects := make([]fyne.CanvasObject, 0, 7)
	for i := range lines {
		lines[i] = shellRectangle(shellBorderColor, 1, nil)
		objects = append(objects, lines[i])
	}
	for i := range steps {
		steps[i] = newShellConnectStep(i)
		objects = append(objects, steps[i])
	}
	return container.New(&shellConnectStepsLayout{}, objects...), steps, lines
}
