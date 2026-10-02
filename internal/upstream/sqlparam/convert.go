package sqlparam

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// 参数声明类型，与前端参数面板的类型选择一一对应。
const (
	TypeString   = "string"
	TypeNumber   = "number"
	TypeBoolean  = "boolean"
	TypeDatetime = "datetime"
	TypeNull     = "null"
	TypeList     = "list"
)

// TypedValue 是一个参数的声明类型与原始值。Value 来自前端 JSON 反序列化：
// 标量为 string/bool/float64/int64/nil，列表为 []any。
type TypedValue struct {
	Type  string
	Value any
}

// 转换失败与使用错误的哨兵错误。上层用 errors.Is 判定后给出可操作提示。
var (
	// ErrMissingParameter 表示某个扫描到的参数没有提供值。
	ErrMissingParameter = errors.New("绑定参数缺少值")
	// ErrEmptyList 表示列表参数没有提供任何元素。
	ErrEmptyList = errors.New("列表参数至少需要一个元素")
	// ErrInvalidValue 表示值与声明类型不匹配或无法解析。
	ErrInvalidValue = errors.New("参数值类型无效")
	// ErrNestedList 表示列表元素中又嵌套了列表，暂不支持。
	ErrNestedList = errors.New("列表参数不支持嵌套列表")
)

// datetime 解析格式按常见输入优先级排列；前端统一传 ISO 8601，
// 其余格式兜底手输或粘贴场景。
var datetimeLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
	"2006/01/02 15:04:05",
	"2006/01/02",
}

// ConvertTypedValue 将声明类型的原始值转换为可传给 database/sql 的绑定值。
// 返回值只能是 nil、string、bool、int64、float64、time.Time 或 []any（元素同为
// 这些标量）；database/sql 驱动负责最终到列类型的适配。
func ConvertTypedValue(declaredType string, value any) (any, error) {
	switch declaredType {
	case TypeNull:
		return nil, nil
	case TypeString:
		return convertString(value)
	case TypeNumber:
		return convertNumber(value)
	case TypeBoolean:
		return convertBoolean(value)
	case TypeDatetime:
		return convertDatetime(value)
	case TypeList:
		return convertList(value)
	default:
		return nil, fmt.Errorf("%w：未知参数类型 %q", ErrInvalidValue, declaredType)
	}
}

func convertString(value any) (any, error) {
	switch v := value.(type) {
	case nil:
		// 类型声明为 string 但未填值等价于显式 NULL，与面板 NULL 勾选一致。
		return nil, nil
	case string:
		return v, nil
	case bool:
		return strconv.FormatBool(v), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	default:
		return nil, fmt.Errorf("%w：字符串参数收到 %T", ErrInvalidValue, value)
	}
}

func convertNumber(value any) (any, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, ErrInvalidValue
		}
		return normalizeJSONNumber(v), nil
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case json.Number:
		return convertNumber(string(v))
	case string:
		text := trimSpaceBytes(v)
		if text == "" {
			return nil, fmt.Errorf("%w：数值参数收到空字符串", ErrInvalidValue)
		}
		if parsed, err := strconv.ParseInt(text, 10, 64); err == nil {
			return parsed, nil
		}
		// Preserve integer values outside int64 as exact decimal text. SQL
		// drivers can bind the string to NUMERIC/DECIMAL without float rounding.
		if !strings.ContainsAny(text, ".eE") && isIntegerText(text) && json.Valid([]byte(text)) {
			return text, nil
		}
		parsed, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return nil, fmt.Errorf("%w：应为数值", ErrInvalidValue)
		}
		return normalizeJSONNumber(parsed), nil
	default:
		return nil, fmt.Errorf("%w：数值参数收到 %T", ErrInvalidValue, value)
	}
}

// normalizeJSONNumber 把整数值的 JSON 数字收敛为 int64，避免 PG 等驱动对
// 整数列收到 float64 时报编码错误；非整数保持 float64。
func normalizeJSONNumber(v float64) any {
	if v == float64(int64(v)) {
		return int64(v)
	}
	return v
}

func convertBoolean(value any) (any, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case bool:
		return v, nil
	case string:
		parsed, err := strconv.ParseBool(trimSpaceBytes(v))
		if err != nil {
			return nil, fmt.Errorf("%w：应为布尔值", ErrInvalidValue)
		}
		return parsed, nil
	default:
		return nil, fmt.Errorf("%w：布尔参数收到 %T", ErrInvalidValue, value)
	}
}

func convertDatetime(value any) (any, error) {
	text, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("%w：日期时间参数收到 %T", ErrInvalidValue, value)
	}
	trimmed := trimSpaceBytes(text)
	if trimmed == "" {
		return nil, nil
	}
	for _, layout := range datetimeLayouts {
		if parsed, err := time.ParseInLocation(layout, trimmed, time.Local); err == nil {
			return parsed, nil
		}
	}
	return nil, fmt.Errorf("%w：应为 YYYY-MM-DD 或 ISO 8601 日期时间", ErrInvalidValue)
}

func convertList(value any) (any, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%w：列表参数收到 %T", ErrInvalidValue, value)
	}
	if len(items) == 0 {
		return nil, ErrEmptyList
	}
	converted := make([]any, 0, len(items))
	for _, item := range items {
		// 列表元素保持原始标量类型，由驱动绑定处理字面量与类型适配。
		switch v := item.(type) {
		case nil:
			converted = append(converted, nil)
		case string:
			converted = append(converted, v)
		case bool:
			converted = append(converted, v)
		case float64:
			value, err := convertNumber(v)
			if err != nil {
				return nil, err
			}
			converted = append(converted, value)
		case json.Number:
			value, err := convertNumber(v)
			if err != nil {
				return nil, err
			}
			converted = append(converted, value)
		case int64:
			converted = append(converted, v)
		case int:
			converted = append(converted, int64(v))
		case []any:
			return nil, ErrNestedList
		default:
			return nil, fmt.Errorf("%w：列表元素收到 %T", ErrInvalidValue, item)
		}
	}
	return converted, nil
}

func trimSpaceBytes(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func isIntegerText(text string) bool {
	for i, r := range text {
		if r == '-' && i == 0 {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return text != "" && text != "-"
}
