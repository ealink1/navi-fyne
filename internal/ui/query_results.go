package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/ealink1/navi-fyne/internal/domain"
)

func (s *workspace) queryResultView(result domain.Result, index int) fyne.CanvasObject {
	if len(result.Columns) == 0 {
		return s.owner.resultView(result)
	}
	selected := map[int]bool{}
	model := gridModel{columns: result.Columns, length: func() int { return len(result.Rows) }, value: func(row, col int) any { return result.Rows[row][col] }, selected: selected, selectRow: func(row int, checked bool) { selected[row] = checked }, inspect: func(row, col int) { s.owner.showCell(result.Columns[col].Name, result.Rows[row][col]) }, copyValue: s.owner.Window.Clipboard().SetContent}
	grid := newDataGrid(model)
	body := container.NewStack(grid)
	toolbar := container.NewHBox(widget.NewIcon(icon("table")), widget.NewLabel("查询结果 · "+s.lastRequest.Scope), toolbarAction("", "refresh", func() { s.runAdaptive() }), toolbarAction("", "copy", func() { s.copyResult(result) }), toolbarAction("", "export", func() { s.exportResult(index) }))
	mode := func(value string) {
		switch value {
		case "table":
			body.Objects = []fyne.CanvasObject{grid}
		case "json":
			text := widget.NewMultiLineEntry()
			text.TextStyle = fyne.TextStyle{Monospace: true}
			raw, err := json.MarshalIndent(resultObjects(result), "", "  ")
			if err != nil {
				text.SetText("无法表示为 JSON：" + err.Error())
			} else {
				text.SetText(string(raw))
			}
			text.Disable()
			body.Objects = []fyne.CanvasObject{text}
		case "text":
			text := widget.NewMultiLineEntry()
			text.TextStyle = fyne.TextStyle{Monospace: true}
			text.SetText(resultText(result))
			text.Disable()
			body.Objects = []fyne.CanvasObject{text}
		}
		body.Refresh()
	}
	note := widget.NewLabel(fmt.Sprintf("%d 行 · %d 列 · %s", len(result.Rows), len(result.Columns), result.Duration.Round(1000000)))
	if result.Truncated {
		note.SetText(note.Text + " · 已截断")
	}
	footer := container.NewHBox(action("", "table", func() { mode("table") }), action("", "json", func() { mode("json") }), action("", "text-view", func() { mode("text") }), note)
	return container.NewBorder(toolbar, footer, nil, nil, body)
}
func resultText(result domain.Result) string {
	var text strings.Builder
	for i, c := range result.Columns {
		if i > 0 {
			text.WriteByte('\t')
		}
		text.WriteString(c.Name)
	}
	text.WriteByte('\n')
	for _, row := range result.Rows {
		for i, value := range row {
			if i > 0 {
				text.WriteByte('\t')
			}
			text.WriteString(displayValue(value))
		}
		text.WriteByte('\n')
	}
	return text.String()
}
func (s *workspace) copyResult(result domain.Result) {
	s.owner.Window.Clipboard().SetContent(resultText(result))
}

func resultObjects(result domain.Result) any {
	seen := map[string]bool{}
	for _, c := range result.Columns {
		if seen[c.Name] {
			return map[string]any{"columns": result.Columns, "rows": result.Rows}
		}
		seen[c.Name] = true
	}
	rows := make([]map[string]any, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = make(map[string]any, len(result.Columns))
		for j, c := range result.Columns {
			if j < len(row) {
				rows[i][c.Name] = row[j]
			}
		}
	}
	return rows
}
