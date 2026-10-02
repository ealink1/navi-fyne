package db

import (
	"sort"
	"strings"

	"github.com/ealink1/navi-fyne/internal/upstream/connection"
)

// 反向语句（undo）生成。
//
// 与 change_preview.go 的分工：那里把 ChangeSet 渲染成"正向可读 SQL"，这里渲染成
// "把数据改回去的 SQL"。两者共用同一套标识符引用与方言字面量格式化钩子，避免出现
// 第二份转义实现（转义写错会直接产出破坏性语句）。
//
// 诚实性约束：本文件只负责"能精确还原的部分"，任何无法保证正确的行都会被跳过并给出
// 结构化原因，绝不用猜测补齐。调用方必须把这些原因如实呈现给用户 —— 生成过反向语句
// 不等于完成了回滚。

// SkippedReverseRow 描述一行为何无法生成反向语句。
// Reason 是 i18n key，不在此处翻译，保持本层为纯函数、便于测试。
type SkippedReverseRow struct {
	// Group 取 "insert" / "update" / "delete"，对应原变更类型。
	Group string `json:"group"`
	// Index 该行在其所属分组内的下标，便于前端定位到具体行。
	Index  int    `json:"index"`
	Reason string `json:"reason"`
}

// ChangeReverseResult 反向语句生成结果。
type ChangeReverseResult struct {
	// Deletes / Updates / Inserts 与 change_preview.go 的输出顺序一致，便于对照展示。
	// 回放顺序应为 Deletes → Updates → Inserts（先撤销新增，再还原修改，最后补回删除）。
	Deletes []string `json:"deletes"`
	Updates []string `json:"updates"`
	Inserts []string `json:"inserts"`
	// Skipped 无法精确还原的行。非空即表示"本次不能完整回滚"。
	Skipped []SkippedReverseRow `json:"skipped,omitempty"`
	// Complete 仅当没有任何行被跳过且至少生成了一条语句时为 true。
	Complete bool `json:"complete"`
}

// 跳过原因（i18n key）。
const (
	reverseReasonMissingPreviousValues = "data_grid.reverse.skip.missing_previous_values"
	reverseReasonMissingPreviousDelete = "data_grid.reverse.skip.missing_previous_delete"
	reverseReasonMissingLocator        = "data_grid.reverse.skip.missing_locator"
	reverseReasonNoLocatorColumns      = "data_grid.reverse.skip.no_locator_columns"
)

// GenerateChangeReverseWithDialect 依据 ChangeSet 中携带的 before-image 生成反向语句。
//
// 三类反向语义：
//   - INSERT → DELETE WHERE <定位器>   需要知道新增行的定位值，否则跳过
//   - UPDATE → UPDATE SET <旧值> WHERE <定位器>
//   - DELETE → INSERT (<全部列>) VALUES (<旧值>)
//
// 未携带 before-image 的行会被跳过而非猜测 —— 旧版前端不填这些字段时，结果是
// Skipped 而非错误 SQL。
func GenerateChangeReverseWithDialect(
	tableName string,
	changes connection.ChangeSet,
	dbType string,
	quoteIdent func(string) string,
	quoteTable func(string) string,
) ChangeReverseResult {
	if quoteTable == nil {
		quoteTable = quoteIdent
	}
	format := func(v interface{}) string { return formatLiteralForDialect(dbType, v) }

	result := ChangeReverseResult{}

	// Deletes 的反向：把删除前的整行插回去。定位器只服务正向语句，
	// 反向 INSERT 依赖完整列快照，因此这里不使用 changes.Deletes 的内容。
	for index := range changes.Deletes {
		previous := previousDeleteAt(changes.PreviousDeletes, index)
		if len(previous) == 0 {
			result.Skipped = append(result.Skipped, SkippedReverseRow{
				Group: "delete", Index: index, Reason: reverseReasonMissingPreviousDelete,
			})
			continue
		}
		statement, ok := buildReverseInsert(tableName, previous, quoteIdent, quoteTable, format)
		if !ok {
			result.Skipped = append(result.Skipped, SkippedReverseRow{
				Group: "delete", Index: index, Reason: reverseReasonMissingPreviousDelete,
			})
			continue
		}
		result.Inserts = append(result.Inserts, statement)
	}

	// Updates 的反向：用旧值覆盖新值，WHERE 沿用正向定位器。
	for index, row := range changes.Updates {
		if len(row.PreviousValues) == 0 {
			result.Skipped = append(result.Skipped, SkippedReverseRow{
				Group: "update", Index: index, Reason: reverseReasonMissingPreviousValues,
			})
			continue
		}
		if len(row.Keys) == 0 {
			result.Skipped = append(result.Skipped, SkippedReverseRow{
				Group: "update", Index: index, Reason: reverseReasonMissingLocator,
			})
			continue
		}
		// 只还原"本次真正改过"的列。PreviousValues 里多出来的列若被写回，
		// 会覆盖本次未触碰的并发修改。
		var sets []string
		for _, column := range sortedKeys(row.Values) {
			previousValue, present := row.PreviousValues[column]
			if !present {
				continue
			}
			sets = append(sets, quoteIdent(column)+" = "+format(previousValue))
		}
		if len(sets) == 0 {
			result.Skipped = append(result.Skipped, SkippedReverseRow{
				Group: "update", Index: index, Reason: reverseReasonMissingPreviousValues,
			})
			continue
		}
		var conditions []string
		for _, column := range sortedKeys(row.Keys) {
			conditions = append(conditions, quoteIdent(column)+" = "+format(row.Keys[column]))
		}
		result.Updates = append(result.Updates, "UPDATE "+quoteTable(tableName)+
			" SET "+strings.Join(sets, ", ")+
			" WHERE "+strings.Join(conditions, " AND ")+";")
	}

	// Inserts 的反向：按定位值删掉新增行。定位值必须来自行数据本身，
	// 数据库生成的（自增主键 / 默认值 / rowid）无法在客户端得知，只能跳过。
	for index, row := range changes.Inserts {
		conditions, ok := buildReverseDeleteConditions(
			row, changes.LocatorColumns, changes.LocatorStrategy, quoteIdent, format)
		if !ok {
			reason := reverseReasonMissingLocator
			if len(changes.LocatorColumns) == 0 {
				reason = reverseReasonNoLocatorColumns
			}
			result.Skipped = append(result.Skipped, SkippedReverseRow{
				Group: "insert", Index: index, Reason: reason,
			})
			continue
		}
		result.Deletes = append(result.Deletes, "DELETE FROM "+quoteTable(tableName)+
			" WHERE "+strings.Join(conditions, " AND ")+";")
	}

	result.Complete = len(result.Skipped) == 0 &&
		len(result.Deletes)+len(result.Updates)+len(result.Inserts) > 0
	return result
}

// previousDeleteAt 容忍 PreviousDeletes 比 Deletes 短（旧前端只填其一）。
func previousDeleteAt(previousDeletes []map[string]interface{}, index int) map[string]interface{} {
	if index < 0 || index >= len(previousDeletes) {
		return nil
	}
	return previousDeletes[index]
}

// buildReverseInsert 用整行旧值拼 INSERT。
// 跳过值为 nil 的列（可能是驱动未回传或被截断的大字段），由数据库默认值兜底；
// 若整行都不可用则返回 false，交由调用方记为 skipped。
func buildReverseInsert(
	tableName string,
	row map[string]interface{},
	quoteIdent func(string) string,
	quoteTable func(string) string,
	format func(interface{}) string,
) (string, bool) {
	columns := make([]string, 0, len(row))
	values := make([]string, 0, len(row))
	for _, column := range sortedKeys(row) {
		value := row[column]
		if value == nil {
			continue
		}
		columns = append(columns, quoteIdent(column))
		values = append(values, format(value))
	}
	if len(columns) == 0 || len(values) != len(columns) {
		return "", false
	}
	return "INSERT INTO " + quoteTable(tableName) +
		" (" + strings.Join(columns, ", ") + ")" +
		" VALUES (" + strings.Join(values, ", ") + ");", true
}

// buildReverseDeleteConditions 依据定位列从行数据里取定位值。
//
// 定位列名与值的承载列分开表达：LocatorColumn.ValueColumn 为空时值就取自 Key 同名列，
// 否则取自 ValueColumn（Oracle / DuckDB 的 rowid 会把真实定位值投影到别名列）。
//
// 任一必填定位列缺失或为空即失败 —— 用不完整的 WHERE 生成 DELETE 会误删其他行。
func buildReverseDeleteConditions(
	row map[string]interface{},
	locatorColumns []connection.LocatorColumn,
	locatorStrategy string,
	quoteIdent func(string) string,
	format func(interface{}) string,
) ([]string, bool) {
	if len(locatorColumns) == 0 {
		return nil, false
	}
	columns := make([]string, 0, len(locatorColumns))
	valueColumns := make(map[string]string, len(locatorColumns))
	for _, locator := range locatorColumns {
		column := strings.TrimSpace(locator.Key)
		if column == "" {
			return nil, false
		}
		valueColumn := strings.TrimSpace(locator.ValueColumn)
		if valueColumn == "" {
			valueColumn = column
		}
		columns = append(columns, column)
		valueColumns[column] = valueColumn
	}
	// 列名排序保证输出确定性；valueColumns 以列名为键，排序不影响取值。
	sort.Strings(columns)

	conditions := make([]string, 0, len(columns))
	for _, column := range columns {
		value, present := row[valueColumns[column]]
		if !present || value == nil {
			return nil, false
		}
		// 空串无法作为可靠定位值：它既可能真是空串，也可能是驱动未回传。
		if text, isString := value.(string); isString && text == "" {
			return nil, false
		}
		conditions = append(conditions, quoteReverseLocatorColumn(column, locatorStrategy, quoteIdent)+
			" = "+format(value))
	}
	return conditions, true
}

// quoteReverseLocatorColumn 引用定位列名，伪列必须保持不引用。
//
// Oracle 的 ROWID 与 DuckDB 的 rowid 是伪列而非真实列名，正向语句
// （oracle_impl.go / duckdb_impl.go）一律以裸名输出，反向语句必须与其一致，
// 否则会生成 `WHERE \`rowid\` = ?` 这类无效 SQL。
//
// 尚存的方言缺口：MySQL 的 `_rowid` 别名会被引成反引号，虽仍能解析但与正向写法不一致；
// 该策略当前不产出 LocatorColumns，故暂不处理。见 docs/需求追踪 的 P1 记录。
func quoteReverseLocatorColumn(column string, locatorStrategy string, quoteIdent func(string) string) string {
	name := strings.TrimSpace(column)
	strategy := strings.ToLower(strings.TrimSpace(locatorStrategy))
	if (strategy == "oracle-rowid" || strategy == "duckdb-rowid") &&
		strings.EqualFold(name, "rowid") {
		return name
	}
	return quoteIdent(name)
}
