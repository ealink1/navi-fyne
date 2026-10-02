package sqlparam

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Dialect 是占位符方言。
type Dialect int

const (
	// DialectQmark 使用 ? 占位符（MySQL 系、SQLite、SQL Server、ClickHouse 等）。
	DialectQmark Dialect = iota
	// DialectDollar 使用 $1、$2 位置占位符（PostgreSQL 系）。
	DialectDollar
	// DialectOracle 使用 :1、:2 位置占位符（Oracle OCI/godror）。
	DialectOracle
)

// DialectForDBType 返回数据源对应的占位符方言。不支持的调用方按 Qmark 处理，
// 由能力声明层决定是否暴露参数功能。
func DialectForDBType(dbType string) Dialect {
	switch normalizeDBType(dbType) {
	case "oracle":
		return DialectOracle
	case "postgres", "opengauss", "gaussdb", "kingbase", "highgo", "vastbase":
		return DialectDollar
	default:
		return DialectQmark
	}
}

// BindResult 是一次成功绑定后的产物：重写后的 SQL 与按占位符顺序排列的参数值。
type BindResult struct {
	SQL  string
	Args []any
}

// Bind 扫描 sql 中的命名参数，用 values 的声明类型转换值，并按 dbType 对应的
// 占位符方言重写 SQL。
//
// 语义约定：
//   - 参数按名字取值，同名出现多次只绑定一次值（填一次、处处生效）；
//   - 列表参数的一个占位符展开为与元素数量相同的逗号分隔占位符（供 IN 使用，
//     圆括号由调用方 SQL 自带）；
//   - 引号模板 '{name} 后缀'：整段（含引号）替换为一个占位符，绑定值为
//     模板渲染后的字符串（{name} 替换为参数原始输入，其余文本原样保留）；
//   - 未提供值的参数返回 ErrMissingParameter（包装具体参数名）；
//   - 不修改任何合法字符串字面量或注释；值永远通过 Args 绑定，绝不拼进 SQL。
func Bind(sql string, dbType string, values map[string]TypedValue) (BindResult, error) {
	opts := OptionsForDBType(dbType)
	spans := Scan(sql, opts)
	if len(spans) == 0 {
		return BindResult{SQL: sql}, nil
	}

	dialect := DialectForDBType(dbType)
	var out strings.Builder
	out.Grow(len(sql) + 8*len(spans))

	// convertedCache 避免同名参数重复转换。
	// Qmark 的 ? 是纯位置占位符：每次出现都要消耗一个参数值（同名重复出现时值重复追加）。
	// Dollar/Oracle 的占位符带序号：同名参数复用同一组槽位（填一次、处处生效）。
	convertedCache := make(map[string]any, len(values))
	nameSlots := make(map[string][]int, len(spans))
	nextSlot := 1
	slots := make(map[int]any)
	var qmarkArgs []any

	for spanIdx, span := range spans {
		out.WriteString(sql[between(spans, spanIdx):span.Start])

		// 引号模板 '{name} 后缀'：整段替换为一个占位符，绑定渲染后的字符串。
		if span.Template != "" {
			rendered, rErr := renderTemplate(span.Template, values)
			if rErr != nil {
				return BindResult{}, rErr
			}
			if dialect == DialectQmark {
				out.WriteByte('?')
				qmarkArgs = append(qmarkArgs, rendered)
			} else {
				marker := "$"
				if dialect == DialectOracle {
					marker = ":"
				}
				fmt.Fprintf(&out, "%s%d", marker, nextSlot)
				slots[nextSlot] = rendered
				nextSlot++
			}
			continue
		}

		converted, err := convertOnce(span.Name, values, convertedCache)
		if err != nil {
			return BindResult{}, err
		}
		items, isList := converted.([]any)

		if dialect == DialectQmark {
			for itemIdx := 0; itemIdx < len(items) || (itemIdx == 0 && !isList); itemIdx++ {
				if itemIdx > 0 {
					out.WriteByte(',')
				}
				out.WriteByte('?')
				if isList {
					qmarkArgs = append(qmarkArgs, items[itemIdx])
				} else {
					qmarkArgs = append(qmarkArgs, converted)
				}
			}
			continue
		}

		spanSlots, ok := nameSlots[span.Name]
		if !ok {
			if isList {
				spanSlots = make([]int, 0, len(items))
				for range items {
					spanSlots = append(spanSlots, nextSlot)
					nextSlot++
				}
			} else {
				spanSlots = []int{nextSlot}
				nextSlot++
			}
			nameSlots[span.Name] = spanSlots
		}

		marker := "$"
		if dialect == DialectOracle {
			marker = ":"
		}
		for slotIdx, slot := range spanSlots {
			if slotIdx > 0 {
				out.WriteByte(',')
			}
			fmt.Fprintf(&out, "%s%d", marker, slot)
			if _, assigned := slots[slot]; !assigned {
				if isList {
					slots[slot] = items[slotIdx]
				} else {
					slots[slot] = converted
				}
			}
		}
	}
	out.WriteString(sql[spans[len(spans)-1].End:])

	if dialect == DialectQmark {
		return BindResult{SQL: out.String(), Args: qmarkArgs}, nil
	}
	args := make([]any, nextSlot-1)
	for slot, value := range slots {
		args[slot-1] = value
	}
	return BindResult{SQL: out.String(), Args: args}, nil
}

// renderTemplate 把字符串模板中的 {name} 替换为参数原始输入的字符串形式，
// 其余文本原样保留。模板内参数缺失时返回 ErrMissingParameter。
// 使用原始输入而非转换后的值：用户输入 "2025-01-01" + 模板后缀 " 00:00:00"
// 应渲染为 "2025-01-01 00:00:00"，二次格式化会造成错位。
func renderTemplate(content string, values map[string]TypedValue) (string, error) {
	var b strings.Builder
	b.Grow(len(content) + 16)
	for i := 0; i < len(content); i++ {
		// DBeaver / PL/SQL Developer 的 '${name}' 会把整个 ${name} 换成值。
		// 这里的词法只认 {name}，若不吞掉紧挨着的 $，渲染结果会多出一个美元符号。
		if content[i] == '$' {
			if name, end, ok := templateParamAt(content, i+1); ok {
				rendered, err := templateParamValue(name, values)
				if err != nil {
					return "", err
				}
				b.WriteString(rendered)
				i = end
				continue
			}
		}
		if content[i] != '{' {
			b.WriteByte(content[i])
			continue
		}
		name, end, ok := templateParamAt(content, i)
		if !ok {
			b.WriteByte(content[i])
			continue
		}
		rendered, err := templateParamValue(name, values)
		if err != nil {
			return "", err
		}
		b.WriteString(rendered)
		i = end
	}
	return b.String(), nil
}

func templateParamAt(content string, start int) (string, int, bool) {
	if start < 0 || start >= len(content) || content[start] != '{' {
		return "", 0, false
	}
	end := strings.IndexByte(content[start:], '}')
	if end <= 1 {
		return "", 0, false
	}
	name := content[start+1 : start+end]
	if !isQuotedParamName(name) {
		return "", 0, false
	}
	return name, start + end, true
}

func templateParamValue(name string, values map[string]TypedValue) (string, error) {
	typed, ok := values[name]
	if !ok {
		return "", fmt.Errorf("%w：%s", ErrMissingParameter, name)
	}
	return renderRawValue(typed.Value), nil
}

// renderRawValue 把参数原始输入渲染为字符串（JSON 反序列化形态 → 文本）。
func renderRawValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	default:
		return fmt.Sprintf("%v", value)
	}
}

func between(spans []Span, idx int) int {
	if idx == 0 {
		return 0
	}
	return spans[idx-1].End
}

// convertOnce 转换并缓存单个参数的值；缺值返回包装了参数名的 ErrMissingParameter。
func convertOnce(name string, values map[string]TypedValue, cache map[string]any) (any, error) {
	if cached, ok := cache[name]; ok {
		return cached, nil
	}
	typed, provided := values[name]
	if !provided {
		return nil, fmt.Errorf("%w：%s", ErrMissingParameter, name)
	}
	converted, err := ConvertTypedValue(typed.Type, typed.Value)
	if err != nil {
		return nil, fmt.Errorf("参数 %s：%w", name, err)
	}
	cache[name] = converted
	return converted, nil
}

// MissingParameterNames 返回扫描到但未提供值的参数名（排序去重），供上层生成
// 可操作的错误提示。
func MissingParameterNames(sql string, dbType string, values map[string]TypedValue) []string {
	opts := OptionsForDBType(dbType)
	var missing []string
	seen := make(map[string]struct{})
	for _, span := range Scan(sql, opts) {
		names := []string{span.Name}
		if span.Template != "" {
			names = extractTemplateNames(span.Template)
		}
		for _, name := range names {
			if _, ok := values[name]; ok {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}
