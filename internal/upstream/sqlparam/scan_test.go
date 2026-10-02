package sqlparam

import (
	"reflect"
	"testing"
)

func namesOf(spans []Span) []string {
	names := make([]string, 0, len(spans))
	for _, span := range spans {
		names = append(names, span.Name)
	}
	return names
}

func TestScanRecognizesNamedParameters(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		opts ScanOptions
		want []string
	}{
		{name: "基础命名参数", sql: "SELECT * FROM orders WHERE status = :status", opts: OptionsForDBType("mysql"), want: []string{"status"}},
		{name: "同参数多次出现不去重", sql: "WHERE a > :d AND b < :d AND c = :e", opts: OptionsForDBType("postgres"), want: []string{"d", "d", "e"}},
		{name: "紧邻标点", sql: "SELECT :a, (:b), (:c), f=:d;", opts: OptionsForDBType(""), want: []string{"a", "b", "c", "d"}},
		{name: "标识符内含美元井号", sql: "WHERE x = :a$b", opts: OptionsForDBType("mysql"), want: []string{"a$b"}},
		{name: "字符串字面量内不算", sql: "WHERE url = 'http://a:b' AND t = ':x' AND y = :y", opts: OptionsForDBType("mysql"), want: []string{"y"}},
		{name: "字符串内单引号转义后仍算", sql: "WHERE note = 'it''s :fake' AND x = :real", opts: OptionsForDBType("postgres"), want: []string{"real"}},
		{name: "MySQL 反斜杠转义保护冒号", sql: "WHERE note = 'it\\'s :fake' AND x = :real", opts: OptionsForDBType("mysql"), want: []string{"real"}},
		{name: "双引号标识符内不算", sql: `SELECT "col:name" FROM t WHERE x = :x`, opts: OptionsForDBType("postgres"), want: []string{"x"}},
		{name: "反引号标识符内不算", sql: "SELECT `col:name` FROM t WHERE x = :x", opts: OptionsForDBType("mysql"), want: []string{"x"}},
		{name: "方括号标识符内不算", sql: "SELECT [col:name] FROM t WHERE x = :x", opts: OptionsForDBType("sqlserver"), want: []string{"x"}},
		{name: "SQL Server 方括号 ]] 转义", sql: "SELECT [a]]b:name] FROM t WHERE x = :x", opts: OptionsForDBType("sqlserver"), want: []string{"x"}},
		{name: "PG 类型转换不算", sql: "SELECT created_at::date, :x::text FROM t", opts: OptionsForDBType("postgres"), want: []string{"x"}},
		{name: "PG dollar-quote 块内不算", sql: "SELECT $$ :fake $$, $tag$ :also-fake $tag$, :real", opts: OptionsForDBType("postgres"), want: []string{"real"}},
		{name: "dollar 标签前是标识符不算块", sql: "SELECT a$tag$ :real $tag$", opts: OptionsForDBType("postgres"), want: []string{"real"}},
		{name: "行注释内不算", sql: "SELECT 1 -- 注释 :fake\n, :real", opts: OptionsForDBType("postgres"), want: []string{"real"}},
		{name: "MySQL 双横线需空格才成注释", sql: "SELECT 1 --:real", opts: OptionsForDBType("mysql"), want: []string{"real"}},
		{name: "PG 双横线无需空格成注释", sql: "SELECT 1 --:fake", opts: OptionsForDBType("postgres"), want: nil},
		{name: "MySQL 井号注释内不算", sql: "SELECT 1 # :fake\n, :real", opts: OptionsForDBType("mysql"), want: []string{"real"}},
		{name: "PG 不认井号注释", sql: "SELECT 1 # :real", opts: OptionsForDBType("postgres"), want: []string{"real"}},
		{name: "块注释内不算", sql: "SELECT 1 /* :fake */ , :real", opts: OptionsForDBType("mysql"), want: []string{"real"}},
		{name: "块注释未闭合之后全部跳过", sql: "SELECT 1 /* :fake, :real", opts: OptionsForDBType("mysql"), want: nil},
		{name: "冒号后非标识符不算", sql: "SELECT '09:30:00', :x", opts: OptionsForDBType("postgres"), want: []string{"x"}},
		{name: "冒号后数字不算", sql: "SELECT 1, :9, :x", opts: OptionsForDBType("mysql"), want: []string{"x"}},
		{name: "标识符后的冒号不算", sql: "SELECT a:b, :x", opts: OptionsForDBType("mysql"), want: []string{"x"}},
		{name: "无参数原样", sql: "SELECT 1; SELECT 'a:b'", opts: OptionsForDBType(""), want: nil},
		{name: "多语句各自扫描", sql: "SELECT :a; SELECT :b", opts: OptionsForDBType("mysql"), want: []string{"a", "b"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := namesOf(Scan(tc.sql, tc.opts))
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Scan(%q) = %v, want %v", tc.sql, got, tc.want)
			}
		})
	}
}

func TestScanSpanOffsetsAreCorrect(t *testing.T) {
	sql := "SELECT a::int, :dup + :dup, 'x:y' FROM t"
	opts := OptionsForDBType("postgres")
	spans := Scan(sql, opts)
	if len(spans) != 2 {
		t.Fatalf("应扫出 2 个出现位置，got %d: %#v", len(spans), spans)
	}
	for _, span := range spans {
		if span.Name != "dup" {
			t.Fatalf("参数名异常: %#v", span)
		}
		if sql[span.Start:span.End] != ":dup" {
			t.Fatalf("跨度切片异常: %q", sql[span.Start:span.End])
		}
	}
}

func TestNamesDeduplicatesPreservingFirstSeenOrder(t *testing.T) {
	sql := "WHERE b = :beta AND a = :alpha AND c = :beta"
	got := Names(sql, OptionsForDBType("mysql"))
	want := []string{"beta", "alpha"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Names = %v, want %v", got, want)
	}
}

func TestScanRecognizesCurlyBraceParameters(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		opts ScanOptions
		want []string
	}{
		{name: "基础花括号参数", sql: "SELECT * FROM t WHERE a = ${x}", opts: OptionsForDBType("mysql"), want: []string{"x"}},
		{name: "花括号允许连字符与点号", sql: "WHERE ${col-name} = 1 AND ${t.col} = 2", opts: OptionsForDBType("postgres"), want: []string{"col-name", "t.col"}},
		{name: "空名不识别", sql: "SELECT ${} FROM t", opts: OptionsForDBType("mysql"), want: nil},
		{name: "未闭合不识别", sql: "SELECT ${x FROM t", opts: OptionsForDBType("mysql"), want: nil},
		{name: "名字内空白截断不识别", sql: "SELECT ${x y} FROM t", opts: OptionsForDBType("mysql"), want: nil},
		{name: "字符串内花括号模板参数", sql: "WHERE note = '${fake}' AND x = ${real}", opts: OptionsForDBType("mysql"), want: []string{"fake", "real"}},
		{name: "注释内不识别", sql: "SELECT 1 /* ${fake} */ , ${real}", opts: OptionsForDBType("mysql"), want: []string{"real"}},
		{name: "PG dollar-quote 不受花括号影响", sql: "SELECT $$ ${fake} $$, ${real}", opts: OptionsForDBType("postgres"), want: []string{"real"}},
		{name: "非 PG 方言同样支持花括号", sql: "WHERE a = ${x}", opts: OptionsForDBType("oracle"), want: []string{"x"}},
		{name: "与冒号参数混合", sql: "WHERE a = :a AND b = ${b} AND c = :a", opts: OptionsForDBType("mysql"), want: []string{"a", "b", "a"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := namesOf(Scan(tc.sql, tc.opts))
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Scan(%q) = %v, want %v", tc.sql, got, tc.want)
			}
		})
	}
}

func TestScanRecognizesQuotedCurlyParameters(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		opts ScanOptions
		want []string
	}{
		{name: "Navicat 风格引号包裹", sql: "WHERE f_date >= '{startDate}'", opts: OptionsForDBType("mysql"), want: []string{"startDate"}},
		{name: "含下划线数字", sql: "WHERE d <= '{u_end_2}', x = '{a1}'", opts: OptionsForDBType("mysql"), want: []string{"u_end_2", "a1"}},
		{name: "带前后缀文本为模板参数", sql: "WHERE note = 'abc{name}def'", opts: OptionsForDBType("mysql"), want: []string{"name"}},
		{name: "多段花括号归为模板", sql: "WHERE x = '{a}{b}'", opts: OptionsForDBType("mysql"), want: []string{"a"}},
		{name: "JSON 字面量不误伤", sql: "WHERE payload = '{\"a\":1}'", opts: OptionsForDBType("postgres"), want: nil},
		{name: "数字开头不识别", sql: "WHERE x = '{123}'", opts: OptionsForDBType("mysql"), want: nil},
		{name: "空白名不识别", sql: "WHERE x = '{ }'", opts: OptionsForDBType("mysql"), want: nil},
		{name: "标准转义引号后的模板参数", sql: "WHERE note = 'it''s {fake}' AND x = '{real}'", opts: OptionsForDBType("postgres"), want: []string{"fake", "real"}},
		{name: "MySQL 反斜杠转义后的模板参数", sql: "WHERE note = 'a\\'b {fake}' AND x = '{real}'", opts: OptionsForDBType("mysql"), want: []string{"fake", "real"}},
		{name: "与裸花括号同名归一", sql: "SELECT '{x}' AS a, ${x} AS b", opts: OptionsForDBType("mysql"), want: []string{"x", "x"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := namesOf(Scan(tc.sql, tc.opts))
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Scan(%q) = %v, want %v", tc.sql, got, tc.want)
			}
		})
	}
}
