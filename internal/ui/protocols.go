package ui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/sqlworkbench"
	"github.com/ealink1/super-link/internal/upstream/db"
)

func (s *workspace) setJSON(value any) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		s.status.SetText(err.Error())
		return
	}
	s.editor.SetText(string(raw))
}
func (s *workspace) protocolControls() fyne.CanvasObject {
	switch s.descriptor.Family {
	case domain.Cache:
		return s.redisControls()
	case domain.Document:
		return s.mongoControls()
	case domain.Search:
		return widget.NewLabel("Elasticsearch REST Console · JSON / NDJSON · 当前范围作为默认索引")
	case domain.Vector:
		return s.vectorControls()
	case domain.Message:
		return s.messageControls()
	case domain.Configuration:
		return s.nacosControls()
	default:
		return widget.NewLabel("SQL 编辑器 · Shift+Enter 执行 · 选中内容优先 · 当前会话保持连接")
	}
}
func (s *workspace) redisControls() fyne.CanvasObject {
	s.scope.SetPlaceHolder("SCAN Key 模式，例如 user:*")
	key := widget.NewEntry()
	key.SetPlaceHolder("Key")
	kind := widget.NewSelect([]string{"GET", "HGETALL", "LRANGE", "SMEMBERS", "ZRANGE", "XRANGE", "TYPE", "TTL"}, nil)
	kind.SetSelected("GET")
	button := widget.NewButton("生成值读取命令", func() {
		name := key.Text
		if name == "" && s.selectedObject != nil {
			name = s.selectedObject.Name
		}
		command := kind.Selected + " " + strconv.Quote(name)
		switch kind.Selected {
		case "LRANGE", "ZRANGE":
			command += " 0 99"
		case "XRANGE":
			command += " - + COUNT 100"
		}
		s.editor.SetText(command)
	})
	return container.NewVBox(widget.NewLabel("Redis · 使用 SCAN 分批浏览；阻塞订阅和脚本执行已关闭"), container.NewBorder(nil, nil, kind, button, key))
}
func (s *workspace) mongoControls() fyne.CanvasObject {
	collection := widget.NewEntry()
	collection.SetPlaceHolder("Collection")
	filter := widget.NewEntry()
	filter.SetText("{}")
	filter.SetPlaceHolder("Filter JSON")
	build := widget.NewButton("生成文档查询", func() {
		name := collection.Text
		if name == "" && s.selectedObject != nil {
			name = s.selectedObject.Name
		}
		var value map[string]any
		if err := json.Unmarshal([]byte(filter.Text), &value); err != nil {
			s.status.SetText("Filter JSON 无效：" + err.Error())
			return
		}
		s.setJSON(map[string]any{"find": name, "filter": value, "limit": 100})
	})
	return container.NewVBox(widget.NewLabel("MongoDB · 原生文档筛选 · 查询 limit=100"), container.NewGridWithColumns(3, collection, filter, build))
}
func (s *workspace) vectorControls() fyne.CanvasObject {
	collection := widget.NewEntry()
	collection.SetPlaceHolder("Collection")
	vector := widget.NewEntry()
	vector.SetPlaceHolder("查询向量，例如 [0.1,0.2,0.3]")
	search := widget.NewButton("生成向量检索", func() {
		name := collection.Text
		if name == "" && s.selectedObject != nil {
			name = s.selectedObject.Name
		}
		var values []float64
		if err := json.Unmarshal([]byte(vector.Text), &values); err != nil || len(values) == 0 {
			s.status.SetText("请填写非空数值向量，并与集合维度保持一致")
			return
		}
		command := map[string]any{"search": name, "vector": values, "limit": 10}
		if s.descriptor.Key == "chroma" {
			command = map[string]any{"query": name, "query_embeddings": [][]float64{values}, "n_results": 10}
		}
		s.setJSON(command)
	})
	return container.NewVBox(widget.NewLabel(s.descriptor.Name+" · Collection 浏览 / 原生向量检索"), container.NewGridWithColumns(3, collection, vector, search), container.NewHBox(widget.NewButton("列出 Collection", func() { s.setJSON(map[string]any{"list_collections": true}) }), widget.NewButton("生成条目预览", s.previewObject)))
}
func (s *workspace) messageControls() fyne.CanvasObject {
	topic := widget.NewEntry()
	topic.SetPlaceHolder("Topic / Queue / Filter")
	payload := widget.NewEntry()
	payload.SetPlaceHolder("测试消息内容")
	limit := widget.NewSelect([]string{"10", "50", "100"}, nil)
	limit.SetSelected("10")
	read := widget.NewButton("生成限量读取", func() {
		name := topic.Text
		if name == "" && s.selectedObject != nil {
			name = s.selectedObject.Name
		}
		query := "SELECT * FROM " + strconv.Quote(name) + " LIMIT " + limit.Selected
		if s.descriptor.Key == "kafka" || s.descriptor.Key == "rocketmq" || s.descriptor.Key == "pulsar" {
			query = "SELECT * FROM " + strconv.Quote(name) + " LATEST LIMIT " + limit.Selected
		}
		s.editor.SetText(query)
	})
	publish := widget.NewButton("生成发布命令", func() {
		name := topic.Text
		if name == "" && s.selectedObject != nil {
			name = s.selectedObject.Name
		}
		command := map[string]any{"publish": name, "payload": payload.Text}
		if s.descriptor.Key == "kafka" || s.descriptor.Key == "pulsar" {
			command["value"] = payload.Text
		}
		if s.descriptor.Key == "mqtt" {
			command["qos"] = 0
			command["retain"] = false
		}
		s.setJSON(command)
	})
	note := "限量读取；停止会取消本次请求；发布必须通过写入按钮确认。"
	switch s.descriptor.Key {
	case "kafka":
		note += "预览使用独立读取，不提交业务 Consumer Group 位点。"
	case "rabbitmq":
		note += "Management API 读取后 requeue，可能改变顺序和 redelivered 状态，需要有副作用确认。"
	case "mqtt":
		note += "接收仅来自订阅期间；不会回放离线历史。"
	}
	label := widget.NewLabel(note)
	label.Wrapping = fyne.TextWrapWord
	return container.NewVBox(label, container.NewGridWithColumns(3, topic, payload, limit), container.NewHBox(read, publish))
}
func (s *workspace) nacosControls() fyne.CanvasObject {
	dataID := widget.NewEntry()
	dataID.SetPlaceHolder("Data ID / Service Name")
	group := widget.NewEntry()
	group.SetText("DEFAULT_GROUP")
	group.SetPlaceHolder("Group")
	command := func(op string) {
		s.setJSON(map[string]any{"op": op, "namespace": s.scope.Text, "group": group.Text, "dataId": dataID.Text, "service": dataID.Text, "page": 1})
	}
	return container.NewVBox(widget.NewLabel("Nacos · 配置 / 服务实例 · Namespace 填写在左侧范围"), container.NewGridWithColumns(2, dataID, group), container.NewHBox(widget.NewButton("配置列表", func() { command("configs") }), widget.NewButton("读取内容", func() { command("get") }), widget.NewButton("服务列表", func() { command("services") }), widget.NewButton("服务实例", func() { command("instances") }), widget.NewButton("生成发布", func() {
		s.setJSON(map[string]any{"op": "publish", "namespace": s.scope.Text, "group": group.Text, "dataId": dataID.Text, "content": "", "type": "text"})
	})))
}
func (s *workspace) previewObject() {
	if s.selectedObject == nil {
		s.status.SetText("请先选择对象")
		return
	}
	object := *s.selectedObject
	switch s.descriptor.Family {
	case domain.Cache:
		s.editor.SetText("GET " + strconv.Quote(object.Name))
	case domain.Document:
		s.setJSON(map[string]any{"find": object.Name, "filter": map[string]any{}, "limit": 100})
	case domain.Vector:
		command := map[string]any{"get": object.Name, "limit": 100}
		if s.descriptor.Key == "qdrant" {
			command = map[string]any{"scroll": object.Name, "limit": 100}
		}
		s.setJSON(command)
	case domain.Message:
		s.editor.SetText("SELECT * FROM " + strconv.Quote(object.Name) + " LIMIT 10")
	case domain.Configuration:
		s.setJSON(map[string]any{"op": "get", "namespace": object.Scope, "group": object.Kind, "dataId": object.Name})
	case domain.Search:
		s.editor.SetText("GET /" + object.Name + "/_search\n{\"size\":100,\"query\":{\"match_all\":{}}}")
	default:
		kind := s.profile.SQLDialect()
		if kind == "iotdb" {
			s.editor.SetText(previewSQL(kind, object.Name))
			return
		}
		text, err := sqlworkbench.PreviewRead(kind, object)
		if err != nil {
			s.status.SetText(err.Error())
			return
		}
		s.editor.SetText(text)
	}
}
func previewSQL(kind, name string) string {
	quote := func(value string) string {
		switch kind {
		case "mysql", "goldendb", "mariadb", "oceanbase", "diros", "starrocks", "sphinx", "clickhouse", "tdengine", "iotdb":
			return "`" + strings.ReplaceAll(value, "`", "``") + "`"
		case "sqlserver":
			return "[" + strings.ReplaceAll(value, "]", "]]") + "]"
		default:
			return "\"" + strings.ReplaceAll(value, "\"", "\"\"") + "\""
		}
	}
	target := quote(name)
	if kind != "sqlite" {
		segments := db.SplitSQLIdentifierPathForDialect(name, kind)
		parts := make([]string, 0, len(segments))
		for index, segment := range segments {
			if kind == "iotdb" && index == 0 && segment.Value == "root" {
				parts = append(parts, "root")
			} else {
				parts = append(parts, quote(segment.Value))
			}
		}
		target = strings.Join(parts, ".")
	}
	switch kind {
	case "sqlserver", "iris", "cache":
		return "SELECT TOP 100 * FROM " + target + ";"
	case "oracle", "dameng":
		return "SELECT * FROM " + target + " WHERE ROWNUM <= 100;"
	default:
		return fmt.Sprintf("SELECT * FROM %s LIMIT 100;", target)
	}
}
