package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/ealink1/navi-fyne/internal/domain"
	"github.com/ealink1/navi-fyne/internal/upstream/sqlparam"
)

type queryParameterField struct {
	typePicker *widget.Select
	value      *widget.Entry
	provided   bool
}

var parameterTypes = []string{"字符串", "数值", "布尔", "日期时间", "NULL", "列表（JSON）"}
var parameterKinds = map[string]string{"字符串": "string", "数值": "number", "布尔": "boolean", "日期时间": "datetime", "NULL": "null", "列表（JSON）": "list"}

func (s *workspace) parameterPanel() fyne.CanvasObject {
	s.parameters = make(map[string]*queryParameterField)
	s.parameterHost = container.NewVBox()
	note := widget.NewLabel("识别 :name、${name} 参数。参数值仅在当前查询标签中保留。")
	note.Wrapping = fyne.TextWrapWord
	s.syncParameters()
	return container.NewBorder(note, nil, nil, nil, container.NewVScroll(s.parameterHost))
}

func (s *workspace) syncParameters() {
	if s.parameterHost == nil {
		return
	}
	names := sqlparam.MissingParameterNames(s.editor.Text, s.profile.SQLDialect(), nil)
	if len(names) > 1024 {
		s.status.SetText("参数数量超过 1,024 个")
		return
	}
	active := make(map[string]*queryParameterField, len(names))
	rows := []fyne.CanvasObject{}
	for _, name := range names {
		field := s.parameters[name]
		if field == nil {
			field = &queryParameterField{typePicker: widget.NewSelect(parameterTypes, nil), value: widget.NewEntry()}
			field.typePicker.SetSelected("字符串")
			field.value.SetPlaceHolder("输入参数值")
			field.value.OnChanged = func(string) { field.provided = true }
			field.typePicker.OnChanged = func(kind string) {
				if kind == "NULL" {
					field.value.Disable()
				} else {
					field.value.Enable()
				}
			}
		}
		active[name] = field
		label := widget.NewLabel(name)
		label.TextStyle = fyne.TextStyle{Monospace: true}
		left := container.NewHBox(container.NewGridWrap(fyne.NewSize(160, 32), label), container.NewGridWrap(fyne.NewSize(150, 32), field.typePicker))
		rows = append(rows, container.NewBorder(nil, nil, left, nil, field.value))
	}
	if len(rows) == 0 {
		rows = append(rows, widget.NewLabel("当前 SQL 没有命名参数"))
	}
	s.parameters = active
	s.parameterHost.Objects = rows
	s.parameterHost.Refresh()
}

func (s *workspace) executionRequest(text string) (domain.Execution, error) {
	r := domain.Execution{Text: text, Scope: s.scope.Text, Schema: s.schemaName, MaxRows: s.rowLimit, Revision: s.profile.Revision}
	if s.descriptor.Family != domain.SQL {
		return r, nil
	}
	names := sqlparam.MissingParameterNames(text, s.profile.SQLDialect(), nil)
	r.Parameters = make(map[string]domain.Parameter, len(names))
	for _, name := range names {
		field := s.parameters[name]
		if field == nil || !field.provided && field.typePicker.Selected != "NULL" {
			if s.outputTabs != nil {
				s.outputTabs.Select(s.parameterTab)
			}
			return r, fmt.Errorf("请在参数面板填写 %s", name)
		}
		value, err := field.parameter()
		if err != nil {
			return r, fmt.Errorf("参数 %s：%w", name, err)
		}
		r.Parameters[name] = value
	}
	return r, nil
}

func (f *queryParameterField) parameter() (domain.Parameter, error) {
	p := domain.Parameter{Type: parameterKinds[f.typePicker.Selected], Value: f.value.Text}
	if p.Type == "null" {
		p.Value = nil
	}
	if p.Type == "list" {
		if len(f.value.Text) > 1<<20 {
			return p, errors.New("列表超过 1 MiB")
		}
		decoder := json.NewDecoder(strings.NewReader(f.value.Text))
		decoder.UseNumber()
		var list []any
		if err := decoder.Decode(&list); err != nil || !json.Valid([]byte(f.value.Text)) || len(list) > 10000 {
			return p, errors.New("请输入最多 10,000 项的 JSON 数组")
		}
		p.Value = list
	}
	return p, nil
}
