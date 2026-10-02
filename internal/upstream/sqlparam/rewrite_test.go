package sqlparam

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBindQmarkDialectReusesValuesForRepeatedNames(t *testing.T) {
	sql := "SELECT :a + :b + :a"
	result, err := Bind(sql, "mysql", map[string]TypedValue{
		"a": {Type: TypeNumber, Value: float64(1)},
		"b": {Type: TypeNumber, Value: float64(2)},
	})
	if err != nil {
		t.Fatalf("Bind 返回错误: %v", err)
	}
	if result.SQL != "SELECT ? + ? + ?" {
		t.Fatalf("重写结果异常: %q", result.SQL)
	}
	if !reflect.DeepEqual(result.Args, []any{int64(1), int64(2), int64(1)}) {
		t.Fatalf("绑定值异常: %#v", result.Args)
	}
}

func TestBindDollarDialectSharesSlotsForRepeatedNames(t *testing.T) {
	sql := "SELECT :a + :b + :a"
	result, err := Bind(sql, "postgres", map[string]TypedValue{
		"a": {Type: TypeNumber, Value: float64(1)},
		"b": {Type: TypeNumber, Value: float64(2)},
	})
	if err != nil {
		t.Fatalf("Bind 返回错误: %v", err)
	}
	if result.SQL != "SELECT $1 + $2 + $1" {
		t.Fatalf("重写结果异常: %q", result.SQL)
	}
	if !reflect.DeepEqual(result.Args, []any{int64(1), int64(2)}) {
		t.Fatalf("绑定值应按占位符序号去重: %#v", result.Args)
	}
}

func TestBindOracleDialectUsesPositionalColons(t *testing.T) {
	result, err := Bind("WHERE a = :x AND b = :y", "oracle", map[string]TypedValue{
		"x": {Type: TypeString, Value: "v1"},
		"y": {Type: TypeString, Value: "v2"},
	})
	if err != nil {
		t.Fatalf("Bind 返回错误: %v", err)
	}
	if result.SQL != "WHERE a = :1 AND b = :2" {
		t.Fatalf("重写结果异常: %q", result.SQL)
	}
	if !reflect.DeepEqual(result.Args, []any{"v1", "v2"}) {
		t.Fatalf("绑定值异常: %#v", result.Args)
	}
}

func TestBindExpandsListForINClause(t *testing.T) {
	cases := []struct {
		name     string
		dbType   string
		wantSQL  string
		wantArgs []any
	}{
		{name: "Qmark", dbType: "mysql", wantSQL: "SELECT * FROM t WHERE id IN (?,?,?)", wantArgs: []any{"a", "b", "c"}},
		{name: "Dollar", dbType: "postgres", wantSQL: "SELECT * FROM t WHERE id IN ($1,$2,$3)", wantArgs: []any{"a", "b", "c"}},
		{name: "Oracle", dbType: "oracle", wantSQL: "SELECT * FROM t WHERE id IN (:1,:2,:3)", wantArgs: []any{"a", "b", "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Bind("SELECT * FROM t WHERE id IN (:ids)", tc.dbType, map[string]TypedValue{
				"ids": {Type: TypeList, Value: []any{"a", "b", "c"}},
			})
			if err != nil {
				t.Fatalf("Bind 返回错误: %v", err)
			}
			if result.SQL != tc.wantSQL {
				t.Fatalf("重写结果异常: %q", result.SQL)
			}
			if !reflect.DeepEqual(result.Args, tc.wantArgs) {
				t.Fatalf("绑定值异常: %#v", result.Args)
			}
		})
	}
}

func TestBindListExpansionKeepsSubsequentPlaceholdersAligned(t *testing.T) {
	result, err := Bind("WHERE a IN (:ids) AND b = :b", "postgres", map[string]TypedValue{
		"ids": {Type: TypeList, Value: []any{float64(1), float64(2)}},
		"b":   {Type: TypeNumber, Value: float64(9)},
	})
	if err != nil {
		t.Fatalf("Bind 返回错误: %v", err)
	}
	if result.SQL != "WHERE a IN ($1,$2) AND b = $3" {
		t.Fatalf("列表展开后占位符序号应继续递增: %q", result.SQL)
	}
	if !reflect.DeepEqual(result.Args, []any{int64(1), int64(2), int64(9)}) {
		t.Fatalf("绑定值异常: %#v", result.Args)
	}
}

func TestBindPreservesLiteralsAndComments(t *testing.T) {
	sql := "SELECT 'a:b' /* :c */ -- :d\nFROM t WHERE x = :x"
	result, err := Bind(sql, "mysql", map[string]TypedValue{
		"x": {Type: TypeString, Value: "v"},
	})
	if err != nil {
		t.Fatalf("Bind 返回错误: %v", err)
	}
	if result.SQL != "SELECT 'a:b' /* :c */ -- :d\nFROM t WHERE x = ?" {
		t.Fatalf("字面量与注释应原样保留: %q", result.SQL)
	}
}

func TestBindWithoutParametersReturnsOriginalSQL(t *testing.T) {
	sql := "SELECT 'a:b', 1, ::date"
	result, err := Bind(sql, "postgres", nil)
	if err != nil {
		t.Fatalf("Bind 返回错误: %v", err)
	}
	if result.SQL != sql || result.Args != nil {
		t.Fatalf("无参数应原样返回: %q %#v", result.SQL, result.Args)
	}
}

func TestBindReportsMissingParameters(t *testing.T) {
	_, err := Bind("WHERE a = :alpha AND b = :beta", "mysql", map[string]TypedValue{
		"alpha": {Type: TypeString, Value: "v"},
	})
	if !errors.Is(err, ErrMissingParameter) {
		t.Fatalf("缺值应返回 ErrMissingParameter: %v", err)
	}
	if !strings.Contains(err.Error(), "beta") {
		t.Fatalf("错误信息应包含缺失参数名: %v", err)
	}
}

func TestMissingParameterNamesListsUnfilled(t *testing.T) {
	got := MissingParameterNames("WHERE a = :b2 AND b = :a1 AND c = :b2", "mysql", map[string]TypedValue{
		"a1": {Type: TypeNull},
	})
	want := []string{"b2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MissingParameterNames = %v, want %v", got, want)
	}
}

func TestBindPropagatesConversionErrors(t *testing.T) {
	_, err := Bind("WHERE a = :d", "mysql", map[string]TypedValue{
		"d": {Type: TypeDatetime, Value: "not-a-date"},
	})
	if !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("非法日期应返回 ErrInvalidValue: %v", err)
	}
}

func TestConvertTypedValueScalars(t *testing.T) {
	parsed, _ := time.ParseInLocation("2006-01-02", "2026-08-01", time.Local)
	cases := []struct {
		name    string
		typ     string
		value   any
		want    any
		wantErr bool
	}{
		{name: "NULL 显式", typ: TypeNull, value: "ignored", want: nil},
		{name: "字符串", typ: TypeString, value: "v", want: "v"},
		{name: "字符串收数字转为文本", typ: TypeString, value: float64(12), want: "12"},
		{name: "整数收敛 int64", typ: TypeNumber, value: float64(12), want: int64(12)},
		{name: "小数保持 float64", typ: TypeNumber, value: 1.5, want: 1.5},
		{name: "数字字符串", typ: TypeNumber, value: "42", want: int64(42)},
		{name: "布尔", typ: TypeBoolean, value: true, want: true},
		{name: "布尔字符串", typ: TypeBoolean, value: "true", want: true},
		{name: "日期 ISO", typ: TypeDatetime, value: "2026-08-01", want: parsed},
		{name: "空字符串日期按 NULL", typ: TypeDatetime, value: "  ", want: nil},
		{name: "字符串类型空值按 NULL", typ: TypeString, value: nil, want: nil},
		{name: "非法布尔", typ: TypeBoolean, value: "yes", wantErr: true},
		{name: "非法日期", typ: TypeDatetime, value: "08/01/2026", wantErr: true},
		{name: "数值收对象", typ: TypeNumber, value: map[string]any{}, wantErr: true},
		{name: "未知类型", typ: "blob", value: "v", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ConvertTypedValue(tc.typ, tc.value)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("应返回错误，got %#v", got)
				}
				if !errors.Is(err, ErrInvalidValue) {
					t.Fatalf("应返回 ErrInvalidValue: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ConvertTypedValue 返回错误: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v (%T), want %#v (%T)", got, got, tc.want, tc.want)
			}
		})
	}
}

func TestConvertTypedListValues(t *testing.T) {
	got, err := ConvertTypedValue(TypeList, []any{"a", nil, float64(3), true})
	if err != nil {
		t.Fatalf("ConvertTypedValue 返回错误: %v", err)
	}
	want := []any{"a", nil, int64(3), true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}

	if _, err := ConvertTypedValue(TypeList, []any{}); !errors.Is(err, ErrEmptyList) {
		t.Fatalf("空列表应返回 ErrEmptyList: %v", err)
	}
	if _, err := ConvertTypedValue(TypeList, []any{[]any{}}); !errors.Is(err, ErrNestedList) {
		t.Fatalf("嵌套列表应返回 ErrNestedList: %v", err)
	}
	if _, err := ConvertTypedValue(TypeList, "not-a-list"); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("非列表应返回 ErrInvalidValue: %v", err)
	}
}

func TestDialectForDBType(t *testing.T) {
	cases := []struct {
		dbType string
		want   Dialect
	}{
		{"oracle", DialectOracle},
		{"postgres", DialectDollar},
		{"postgresql", DialectDollar},
		{"kingbase8", DialectDollar},
		{"mysql", DialectQmark},
		{"sqlserver", DialectQmark},
		{"", DialectQmark},
	}
	for _, tc := range cases {
		if got := DialectForDBType(tc.dbType); got != tc.want {
			t.Fatalf("DialectForDBType(%q) = %d, want %d", tc.dbType, got, tc.want)
		}
	}
}

func TestBindCurlyBraceParameters(t *testing.T) {
	cases := []struct {
		name     string
		dbType   string
		sql      string
		wantSQL  string
		wantArgs []any
	}{
		{name: "Qmark", dbType: "mysql", sql: "WHERE a = ${x}", wantSQL: "WHERE a = ?", wantArgs: []any{"v"}},
		{name: "Dollar", dbType: "postgres", sql: "WHERE a = ${x}", wantSQL: "WHERE a = $1", wantArgs: []any{"v"}},
		{name: "Oracle", dbType: "oracle", sql: "WHERE a = ${x}", wantSQL: "WHERE a = :1", wantArgs: []any{"v"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Bind(tc.sql, tc.dbType, map[string]TypedValue{
				"x": {Type: TypeString, Value: "v"},
			})
			if err != nil {
				t.Fatalf("Bind 返回错误: %v", err)
			}
			if result.SQL != tc.wantSQL {
				t.Fatalf("重写结果异常: %q", result.SQL)
			}
			if !reflect.DeepEqual(result.Args, tc.wantArgs) {
				t.Fatalf("绑定值异常: %#v", result.Args)
			}
		})
	}
}

func TestBindNormalizesColonAndCurlySameName(t *testing.T) {
	// :x 与 ${x} 同名归一：跨语法填一次生效。
	result, err := Bind("SELECT :x AS a, ${x} AS b", "postgres", map[string]TypedValue{
		"x": {Type: TypeString, Value: "same"},
	})
	if err != nil {
		t.Fatalf("Bind 返回错误: %v", err)
	}
	if result.SQL != "SELECT $1 AS a, $1 AS b" {
		t.Fatalf("跨语法同名应复用槽位: %q", result.SQL)
	}
	if !reflect.DeepEqual(result.Args, []any{"same"}) {
		t.Fatalf("绑定值异常: %#v", result.Args)
	}
}

func TestBindListCurlyBraceExpansion(t *testing.T) {
	result, err := Bind("SELECT name FROM t WHERE id IN (${ids})", "sqlite", map[string]TypedValue{
		"ids": {Type: TypeList, Value: []any{"a", "b"}},
	})
	if err != nil {
		t.Fatalf("Bind 返回错误: %v", err)
	}
	if result.SQL != "SELECT name FROM t WHERE id IN (?,?)" {
		t.Fatalf("花括号列表展开异常: %q", result.SQL)
	}
	if !reflect.DeepEqual(result.Args, []any{"a", "b"}) {
		t.Fatalf("绑定值异常: %#v", result.Args)
	}
}

func TestBindQuotedCurlyParameterRewritesWholeLiteral(t *testing.T) {
	// 引号连同内容整体替换为占位符，值走绑定——杜绝字符串拼接注入面。
	result, err := Bind("WHERE f_trade_date >= '{startDate}' AND f_trade_date < '{endDate}'", "mysql", map[string]TypedValue{
		"startDate": {Type: TypeDatetime, Value: "2025-01-01 00:00:00"},
		"endDate":   {Type: TypeDatetime, Value: "2025-01-31 23:59:59"},
	})
	if err != nil {
		t.Fatalf("Bind 返回错误: %v", err)
	}
	if result.SQL != "WHERE f_trade_date >= ? AND f_trade_date < ?" {
		t.Fatalf("引号应连同内容整体替换: %q", result.SQL)
	}
	if len(result.Args) != 2 {
		t.Fatalf("应绑定两个时间值: %#v", result.Args)
	}
}

func TestBindQuotedDollarTemplateDropsDollarPrefix(t *testing.T) {
	result, err := Bind("SELECT '${year}' || '-' || LPAD(LEVEL, 2, '0') AS yearMonth FROM dual", "oracle", map[string]TypedValue{
		"year": {Type: TypeString, Value: "2026"},
	})
	if err != nil {
		t.Fatalf("Bind 返回错误: %v", err)
	}
	if result.SQL != "SELECT :1 || '-' || LPAD(LEVEL, 2, '0') AS yearMonth FROM dual" {
		t.Fatalf("重写结果异常: %q", result.SQL)
	}
	if !reflect.DeepEqual(result.Args, []any{"2026"}) {
		t.Fatalf("渲染值应去掉 ${} 的美元符号: %#v", result.Args)
	}
}

func TestBindQuotedTemplateRendersSuffixText(t *testing.T) {
	// 用户场景：'{u_startDate} 00:00:00' → 占位符绑定 "2025-01-01 00:00:00"
	result, err := Bind("WHERE f_trade_date >= '{u_startDate} 00:00:00'", "mysql", map[string]TypedValue{
		"u_startDate": {Type: TypeDatetime, Value: "2025-01-01"},
	})
	if err != nil {
		t.Fatalf("Bind 返回错误: %v", err)
	}
	if result.SQL != "WHERE f_trade_date >= ?" {
		t.Fatalf("重写结果异常: %q", result.SQL)
	}
	if !reflect.DeepEqual(result.Args, []any{"2025-01-01 00:00:00"}) {
		t.Fatalf("渲染值异常: %#v", result.Args)
	}
}

func TestBindQuotedTemplateMultipleNames(t *testing.T) {
	result, err := Bind("WHERE d BETWEEN '{start} 00:00:00' AND '{end} 23:59:59'", "mysql", map[string]TypedValue{
		"start": {Type: TypeDatetime, Value: "2025-01-01"},
		"end":   {Type: TypeDatetime, Value: "2025-01-31"},
	})
	if err != nil {
		t.Fatalf("Bind 返回错误: %v", err)
	}
	if result.SQL != "WHERE d BETWEEN ? AND ?" {
		t.Fatalf("重写结果异常: %q", result.SQL)
	}
	if !reflect.DeepEqual(result.Args, []any{"2025-01-01 00:00:00", "2025-01-31 23:59:59"}) {
		t.Fatalf("绑定值异常: %#v", result.Args)
	}
}
