package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/ealink1/navi-fyne/internal/domain"
)

type filterRow struct {
	enabled                *widget.Check
	column, operator, join *widget.Select
	value                  *widget.Entry
	content                fyne.CanvasObject
}
type sortRow struct {
	column, direction *widget.Select
	content           fyne.CanvasObject
}

func (t *tableWorkspace) buildFilter() *fyne.Container {
	t.filterRows = container.NewVBox()
	t.sortRows = container.NewVBox()
	clear := action("清空", "trash", func() {
		if !t.canChangePage() {
			return
		}
		t.condition.SetText("")
		t.filters = nil
		t.sorts = nil
		t.filterRows.Objects = nil
		t.sortRows.Objects = nil
		t.filterRows.Refresh()
		t.sortRows.Refresh()
		t.applyFilters()
	})
	apply := widget.NewButton("应用条件", t.applyFilters)
	apply.Importance = widget.HighImportance
	condition := container.NewBorder(nil, nil, widget.NewLabel("手动查询条件"), nil, t.condition)
	tools := container.NewHBox(action("添加条件", "add-row", t.addFilter), action("添加排序", "columns", t.addSort), layout.NewSpacer(), clear, apply)
	return container.NewPadded(container.NewVBox(condition, t.filterRows, t.sortRows, tools))
}
func fixedControl(width float32, object fyne.CanvasObject) fyne.CanvasObject {
	return container.NewGridWrap(fyne.NewSize(width, 30), object)
}
func (t *tableWorkspace) addFilter() {
	if len(t.filters) >= 64 {
		t.status.SetText("最多添加 64 个条件")
		return
	}
	r := &filterRow{enabled: widget.NewCheck("", nil), column: widget.NewSelect(t.columnNames(), nil), operator: widget.NewSelect([]string{"=", "<>", ">", ">=", "<", "<=", "LIKE", "NOT LIKE", "IS NULL", "IS NOT NULL"}, nil), join: widget.NewSelect([]string{"AND", "OR"}, nil), value: widget.NewEntry()}
	r.enabled.SetChecked(true)
	r.column.PlaceHolder = "选择字段"
	r.operator.SetSelected("=")
	r.join.SetSelected("AND")
	r.value.SetPlaceHolder("值")
	r.operator.OnChanged = func(value string) {
		if value == "IS NULL" || value == "IS NOT NULL" {
			r.value.Disable()
		} else {
			r.value.Enable()
		}
	}
	remove := action("", "trash", func() {
		for i, c := range t.filters {
			if c == r {
				t.filters = append(t.filters[:i], t.filters[i+1:]...)
				break
			}
		}
		t.filterRows.Remove(r.content)
	})
	r.content = container.NewBorder(nil, nil, container.NewHBox(r.enabled, fixedControl(74, r.join), fixedControl(160, r.column), fixedControl(120, r.operator)), remove, r.value)
	t.filters = append(t.filters, r)
	t.filterRows.Add(r.content)
}
func (t *tableWorkspace) addSort() {
	if len(t.sorts) >= 32 {
		t.status.SetText("最多添加 32 个排序字段")
		return
	}
	r := &sortRow{column: widget.NewSelect(t.columnNames(), nil), direction: widget.NewSelect([]string{"ASC", "DESC"}, nil)}
	r.column.PlaceHolder = "排序字段"
	r.direction.SetSelected("ASC")
	remove := action("", "trash", func() {
		for i, c := range t.sorts {
			if c == r {
				t.sorts = append(t.sorts[:i], t.sorts[i+1:]...)
				break
			}
		}
		t.sortRows.Remove(r.content)
	})
	r.content = container.NewHBox(widget.NewLabel("排序"), fixedControl(200, r.column), fixedControl(100, r.direction), remove)
	t.sorts = append(t.sorts, r)
	t.sortRows.Add(r.content)
}
func (t *tableWorkspace) canChangePage() bool {
	if t.busy {
		return false
	}
	if t.dirty() {
		t.status.SetText("请先提交或丢弃当前修改，再应用条件。 ")
		return false
	}
	return true
}
func (t *tableWorkspace) applyFilters() {
	if !t.canChangePage() {
		return
	}
	request := t.request
	request.Page = 1
	request.Condition = t.condition.Text
	request.Filters = nil
	request.Sorts = nil
	for _, r := range t.filters {
		if !r.enabled.Checked {
			continue
		}
		if r.column.Selected == "" {
			t.status.SetText("请选择条件字段")
			return
		}
		request.Filters = append(request.Filters, domain.Filter{Column: r.column.Selected, Operator: r.operator.Selected, Value: r.value.Text, Join: r.join.Selected})
	}
	for _, r := range t.sorts {
		if r.column.Selected == "" {
			t.status.SetText("请选择排序字段")
			return
		}
		request.Sorts = append(request.Sorts, domain.Sort{Column: r.column.Selected, Descending: r.direction.Selected == "DESC"})
	}
	t.request = request
	t.refresh()
}
