package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/ealink1/super-link/internal/domain"
)

func (d *tableDesigner) indexPanel() fyne.CanvasObject {
	selected := map[int]bool{}
	result := domain.Result{Columns: []domain.Column{{Name: "名称"}, {Name: "字段"}, {Name: "唯一"}, {Name: "类型"}, {Name: "顺序"}}}
	for _, index := range d.info.Indexes {
		result.Rows = append(result.Rows, []any{index.Name, index.ColumnName, index.NonUnique == 0, index.IndexType, index.SeqInIndex})
	}
	for _, change := range d.indexChanges {
		if change.Kind == "addIndex" {
			names := []string{}
			for _, c := range change.Index.Columns {
				names = append(names, c.Name)
			}
			result.Rows = append(result.Rows, []any{change.Index.Name, strings.Join(names, ", "), change.Index.Unique, change.Index.Method, "待新增"})
		}
	}
	model := gridModel{columns: result.Columns, length: func() int { return len(result.Rows) }, value: func(row, col int) any { return result.Rows[row][col] }, selected: selected, selectRow: func(row int, v bool) { selected[row] = v }, inspect: func(row, col int) { d.owner.showCell(result.Columns[col].Name, result.Rows[row][col]) }, boolColumns: map[int]bool{2: true}}
	model.changed = func(row int) string {
		if row >= len(d.info.Indexes) {
			return "insert"
		}
		for _, change := range d.indexChanges {
			if change.Kind == "dropIndex" && change.OriginalName == d.info.Indexes[row].Name {
				return "delete"
			}
		}
		return ""
	}
	tools := container.NewHBox(action("新建索引", "add-row", d.addIndex), action("删除选中索引", "trash", func() { d.removeIndexes(selected, false) }), action("撤销选中删除", "undo", func() { d.removeIndexes(selected, true) }), layout.NewSpacer())
	return container.NewBorder(tools, nil, nil, nil, newDataGrid(model))
}

type indexField struct {
	column     *widget.Select
	descending *widget.Check
}

func (d *tableDesigner) addIndex() {
	if !d.canEdit() || !d.stageEditors() {
		return
	}
	name := widget.NewEntry()
	name.SetPlaceHolder("索引名称")
	unique := widget.NewCheck("唯一索引", nil)
	methods := []string{"BTREE"}
	if d.profile.SQLDialect() == "postgres" {
		methods = append(methods, "HASH", "GIN", "GIST", "BRIN", "SPGIST")
	} else if d.profile.SQLDialect() == "mysql" {
		methods = append(methods, "HASH")
	}
	method := widget.NewSelect(methods, nil)
	method.SetSelected("BTREE")
	columns := []string{}
	for _, f := range d.fields {
		if !f.deleted && f.column.Name != "" {
			columns = append(columns, f.column.Name)
		}
	}
	rows := container.NewVBox()
	fields := []indexField{}
	var updatePreview func()
	add := func() {
		if len(fields) >= 32 {
			return
		}
		picker := widget.NewSelect(columns, nil)
		picker.PlaceHolder = "选择字段"
		if len(columns) > 0 {
			picker.SetSelected(columns[0])
		}
		order := widget.NewCheck("降序", nil)
		fields = append(fields, indexField{column: picker, descending: order})
		rows.Add(container.NewBorder(nil, nil, nil, order, picker))
		picker.OnChanged = func(string) {
			if updatePreview != nil {
				updatePreview()
			}
		}
		order.OnChanged = func(bool) {
			if updatePreview != nil {
				updatePreview()
			}
		}
		if updatePreview != nil {
			updatePreview()
		}
	}
	add()
	form := container.NewVBox(widget.NewForm(widget.NewFormItem("名称", name), widget.NewFormItem("类型", method)), unique, widget.NewLabel("字段顺序与列表顺序一致"), rows, action("添加字段", "add-row", add))
	indexValue := func() domain.NewIndex {
		index := domain.NewIndex{Name: name.Text, Unique: unique.Checked, Method: method.Selected}
		for _, field := range fields {
			index.Columns = append(index.Columns, domain.IndexColumn{Name: field.column.Selected, Descending: field.descending.Checked})
		}
		return index
	}
	preview, submit := widget.NewMultiLineEntry(), widget.NewButton("暂存", nil)
	preview.TextStyle = fyne.TextStyle{Monospace: true}
	preview.SetMinRowsVisible(6)
	preview.Disable()
	updatePreview = func() { d.previewNewIndex(indexValue(), preview, submit) }
	name.OnChanged = func(string) { updatePreview() }
	method.OnChanged = func(string) { updatePreview() }
	unique.OnChanged = func(bool) { updatePreview() }
	form.Add(widget.NewLabel("SQL 预览"))
	form.Add(preview)
	form.Add(submit)
	modal := dialog.NewCustom("新建索引", "取消", container.NewVScroll(form), d.owner.Window)
	submit.OnTapped = func() {
		if d.stageNewIndex(indexValue()) {
			modal.Hide()
		}
	}
	updatePreview()
	modal.Resize(fyne.NewSize(630, 620))
	modal.Show()
}
