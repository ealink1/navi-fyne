package db

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"testing"
)

// Oracle 11g 的 ALL_VIEWS.TEXT 是 LONG；12c 起为 CLOB。两者都会以 string 进入扫描层。
const scanRowsOracleLongViewTail = "-- END_OF_VIEW_TEXT"

func buildScanRowsOracleLongViewText() string {
	return `CREATE OR REPLACE VIEW "H2"."CV_SALES_DETAIL" AS SELECT '` +
		strings.Repeat("明细", 1500) +
		`' AS payload FROM DUAL ` + scanRowsOracleLongViewTail
}

func openScanRowsOracleTextRows(t *testing.T, query string) *sql.Rows {
	t.Helper()

	registerScanRowsDuplicateDriverOnce.Do(func() {
		sql.Register(scanRowsDuplicateDriverName, scanRowsDuplicateDriver{})
	})

	dbConn, err := sql.Open(scanRowsDuplicateDriverName, "")
	if err != nil {
		t.Fatalf("open oracle text scan rows db failed: %v", err)
	}
	t.Cleanup(func() {
		_ = dbConn.Close()
	})

	rows, err := dbConn.QueryContext(context.Background(), query)
	if err != nil {
		t.Fatalf("query oracle text scan rows db failed: %v", err)
	}
	t.Cleanup(func() {
		_ = rows.Close()
	})
	return rows
}

func desktopGridBudgetContext(t *testing.T, options RowBudgetOptions) context.Context {
	t.Helper()

	budget := NewRowBudgetWithOptions(options)
	if budget == nil {
		t.Fatal("desktop grid budget must not be nil")
	}
	return ContextWithRowBudget(context.Background(), budget)
}

// parseScanRowsPreviewMarker 解析 "[TEXT preview: 1023/9099 bytes] " 这类前缀，
// 返回保留字节数、原始字节数以及标记之后的载荷起点。
func parseScanRowsPreviewMarker(value, prefix string) (previewBytes, payloadBytes, markerEnd int, ok bool) {
	if !strings.HasPrefix(value, prefix) {
		return 0, 0, 0, false
	}
	inner, _, found := strings.Cut(value[len(prefix):], "] ")
	if !found {
		return 0, 0, 0, false
	}
	previewText, payloadText, found := strings.Cut(inner, "/")
	if !found {
		return 0, 0, 0, false
	}
	previewValue, err := strconv.Atoi(strings.TrimSpace(previewText))
	if err != nil {
		return 0, 0, 0, false
	}
	payloadValue, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(payloadText), " bytes"))
	if err != nil {
		return 0, 0, 0, false
	}
	return previewValue, payloadValue, len(prefix) + len(inner) + len("] "), true
}

func TestScanRowsKeepsCompleteOracleLongValueWithFieldBudget(t *testing.T) {
	t.Parallel()

	rows := openScanRowsOracleTextRows(t, "SELECT long_columns")
	fullText := buildScanRowsOracleLongViewText()
	if len(fullText) <= interactiveOracleLargeObjectPreviewBytes {
		t.Fatalf("fixture must exceed the 4KB interactive preview: length=%d", len(fullText))
	}

	// 桌面查询编辑器绑定的默认单字段预算（1MB）。
	ctx := desktopGridBudgetContext(t, RowBudgetOptions{
		MaxRowsPerResult: 50000,
		MaxTotalRows:     50000,
		MaxTotalBytes:    24 << 20,
		MaxFieldBytes:    1 << 20,
	})
	data, _, err := scanRowsForDialectContext(ctx, rows, "")
	if err != nil {
		t.Fatalf("scanRowsForDialectContext returned error: %v", err)
	}
	if len(data) != 1 {
		t.Fatalf("oracle LONG result rows = %d, want 1", len(data))
	}

	value, ok := data[0]["ddl"].(string)
	if !ok {
		t.Fatalf("oracle LONG cell type = %T, want string", data[0]["ddl"])
	}
	if strings.Contains(value, "[CLOB preview:") {
		t.Fatalf("oracle LONG cell keeps the interactive preview marker: %q", value[:min(len(value), 96)])
	}
	if !strings.Contains(value, scanRowsOracleLongViewTail) {
		t.Fatalf("oracle LONG cell was truncated at %d bytes, want complete %d bytes", len(value), len(fullText))
	}
	if value != fullText {
		t.Fatalf("oracle LONG cell differs from the stored view text: got length=%d want length=%d", len(value), len(fullText))
	}
}

func TestScanRowsKeepsCompleteOracleClobValueWithFieldBudget(t *testing.T) {
	t.Parallel()

	rows := openScanRowsOracleTextRows(t, "SELECT clob_columns")
	fullText := strings.Repeat("数", 6*1024)

	ctx := desktopGridBudgetContext(t, RowBudgetOptions{
		MaxRowsPerResult: 50000,
		MaxTotalRows:     50000,
		MaxTotalBytes:    24 << 20,
		MaxFieldBytes:    1 << 20,
	})
	data, _, err := scanRowsForDialectContext(ctx, rows, "")
	if err != nil {
		t.Fatalf("scanRowsForDialectContext returned error: %v", err)
	}
	value, ok := data[0]["content"].(string)
	if !ok {
		t.Fatalf("oracle CLOB cell type = %T, want string", data[0]["content"])
	}
	if value != fullText {
		t.Fatalf("oracle CLOB cell was truncated: got length=%d want length=%d", len(value), len(fullText))
	}
	if strings.Contains(value, "[CLOB preview:") {
		t.Fatalf("oracle CLOB cell keeps the interactive preview marker: %q", value[:min(len(value), 96)])
	}
}

func TestScanRowsMatchesUnboundedStreamForOracleLongViewText(t *testing.T) {
	t.Parallel()

	// 「查看视图定义」走无界流式查询；网格走带预算的扫描。同一 LONG 文本
	// 在两条路径上必须得到相同结果，避免同一视图时好时坏。
	streamRows := openScanRowsOracleTextRows(t, "SELECT long_columns")
	consumer := &scanRowsValueConsumer{}
	if err := streamRowsForDialect(streamRows, "", consumer); err != nil {
		t.Fatalf("streamRowsForDialect returned error: %v", err)
	}
	if len(consumer.rows) != 1 || len(consumer.rows[0]) != 1 {
		t.Fatalf("unexpected streamed oracle LONG rows: %#v", consumer.rows)
	}
	streamedText, ok := consumer.rows[0][0].(string)
	if !ok {
		t.Fatalf("streamed oracle LONG cell type = %T, want string", consumer.rows[0][0])
	}

	gridRows := openScanRowsOracleTextRows(t, "SELECT long_columns")
	ctx := desktopGridBudgetContext(t, RowBudgetOptions{
		MaxRowsPerResult: 50000,
		MaxTotalRows:     50000,
		MaxTotalBytes:    24 << 20,
		MaxFieldBytes:    1 << 20,
	})
	data, _, err := scanRowsForDialectContext(ctx, gridRows, "")
	if err != nil {
		t.Fatalf("scanRowsForDialectContext returned error: %v", err)
	}

	if got := data[0]["ddl"]; got != streamedText {
		t.Fatalf("grid and view-definition paths disagree: grid length=%d stream length=%d", len(got.(string)), len(streamedText))
	}
	if !strings.Contains(streamedText, scanRowsOracleLongViewTail) {
		t.Fatalf("streamed oracle LONG text lost its tail: length=%d", len(streamedText))
	}
}

func TestScanRowsStillBoundsOracleTextPreviewWithoutFieldBudget(t *testing.T) {
	t.Parallel()

	// 未绑定单字段预算的调用（如 DBQuery 取定义、浏览表数据）继续使用 4KB 兜底预览，
	// 保持跨 Wails 桥的体积保护与既有的 [CLOB preview:] 标记契约。
	rows := openScanRowsOracleTextRows(t, "SELECT long_columns")

	data, _, err := scanRowsForDialect(rows, "")
	if err != nil {
		t.Fatalf("scanRowsForDialect returned error: %v", err)
	}
	value, ok := data[0]["ddl"].(string)
	if !ok {
		t.Fatalf("oracle LONG cell type = %T, want string", data[0]["ddl"])
	}
	// truncateUTF8Prefix 会回退到 UTF-8 字符边界，实际保留字节数不超过上限。
	previewBytes, payloadBytes, markerEnd, ok := parseScanRowsPreviewMarker(value, "[CLOB preview:")
	if !ok {
		t.Fatalf("unbudgeted oracle LONG cell lost its bounded preview: %q", value[:min(len(value), 96)])
	}
	if previewBytes > interactiveOracleLargeObjectPreviewBytes {
		t.Fatalf("unbudgeted oracle LONG preview = %d bytes, want at most %d", previewBytes, interactiveOracleLargeObjectPreviewBytes)
	}
	if payloadBytes <= interactiveOracleLargeObjectPreviewBytes {
		t.Fatalf("fixture no longer exercises the 4KB guard: payload=%d bytes", payloadBytes)
	}
	if got := len(value) - markerEnd; got != previewBytes {
		t.Fatalf("unbudgeted oracle LONG payload = %d bytes, want exactly the %d previewed bytes", got, previewBytes)
	}
	if strings.Contains(value, scanRowsOracleLongViewTail) {
		t.Fatalf("unbudgeted oracle LONG cell exceeds the bridge guard: length=%d", len(value))
	}
}

func TestScanRowsStillBoundsOracleBlobPreviewWithFieldBudget(t *testing.T) {
	t.Parallel()

	// BLOB 预览不受单字段预算影响：十六进制展开会放大体积，桥接保护必须保留。
	registerScanRowsDuplicateDriverOnce.Do(func() {
		sql.Register(scanRowsDuplicateDriverName, scanRowsDuplicateDriver{})
	})
	dbConn, err := sql.Open(scanRowsDuplicateDriverName, "")
	if err != nil {
		t.Fatalf("open blob scan rows db failed: %v", err)
	}
	defer dbConn.Close()

	rows, err := dbConn.QueryContext(context.Background(), "SELECT blob_columns")
	if err != nil {
		t.Fatalf("query blob scan rows db failed: %v", err)
	}
	defer rows.Close()

	ctx := desktopGridBudgetContext(t, RowBudgetOptions{
		MaxRowsPerResult: 50000,
		MaxTotalRows:     50000,
		MaxTotalBytes:    24 << 20,
		MaxFieldBytes:    1 << 20,
	})
	data, _, err := scanRowsForDialectContext(ctx, rows, "")
	if err != nil {
		t.Fatalf("scanRowsForDialectContext returned error: %v", err)
	}
	value, ok := data[0]["payload"].(string)
	if !ok {
		t.Fatalf("oracle BLOB preview type = %T, want string", data[0]["payload"])
	}
	wantPrefix := "[BLOB preview: 4096/"
	if !strings.HasPrefix(value, wantPrefix) {
		t.Fatalf("budgeted oracle BLOB cell lost its bounded preview: %q", value[:min(len(value), 96)])
	}
}

func TestScanRowsBoundsOversizedOracleLongUnderFieldBudget(t *testing.T) {
	t.Parallel()

	// 超过单字段预算的文本仍被通用预算截断，并带上网格的截断指示。
	rows := openScanRowsOracleTextRows(t, "SELECT long_columns")

	ctx := desktopGridBudgetContext(t, RowBudgetOptions{
		MaxRowsPerResult: 50000,
		MaxTotalRows:     50000,
		MaxTotalBytes:    24 << 20,
		MaxFieldBytes:    1024,
	})
	data, _, truncated, err := scanRowsForDialectWithPreview(rows, "", true, RowBudgetFromContext(ctx))
	if err != nil {
		t.Fatalf("scanRowsForDialectWithPreview returned error: %v", err)
	}
	value, ok := data[0]["ddl"].(string)
	if !ok {
		t.Fatalf("budgeted oracle LONG cell type = %T, want string", data[0]["ddl"])
	}
	previewBytes, _, markerEnd, hasMarker := parseScanRowsPreviewMarker(value, "[TEXT preview:")
	if !hasMarker {
		t.Fatalf("oversized oracle LONG cell is not bounded by the field budget: %q", value[:min(len(value), 96)])
	}
	if previewBytes > 1024 {
		t.Fatalf("oversized oracle LONG preview = %d bytes, want at most the 1024-byte field budget", previewBytes)
	}
	if got := len(value) - markerEnd; got != previewBytes {
		t.Fatalf("oversized oracle LONG payload = %d bytes, want exactly the %d previewed bytes", got, previewBytes)
	}
	if !truncated {
		t.Fatal("oversized oracle LONG cell must be reported as truncated")
	}
}

// TestOracleQueryContextKeepsCompleteLongViewTextWithFieldBudget 走真实的网格入口：
// OracleDB.QueryContext → scanRowsForDialectContext，绑定桌面查询编辑器的默认预算。
func TestOracleQueryContextKeepsCompleteLongViewTextWithFieldBudget(t *testing.T) {
	t.Parallel()

	registerScanRowsDuplicateDriverOnce.Do(func() {
		sql.Register(scanRowsDuplicateDriverName, scanRowsDuplicateDriver{})
	})
	dbConn, err := sql.Open(scanRowsDuplicateDriverName, "")
	if err != nil {
		t.Fatalf("open oracle grid db failed: %v", err)
	}
	t.Cleanup(func() {
		_ = dbConn.Close()
	})

	oracleDB := &OracleDB{conn: dbConn}
	ctx := desktopGridBudgetContext(t, RowBudgetOptions{
		MaxRowsPerResult: 50000,
		MaxTotalRows:     50000,
		MaxTotalBytes:    24 << 20,
		MaxFieldBytes:    1 << 20,
	})

	data, columns, err := oracleDB.QueryContext(ctx, "SELECT long_columns")
	if err != nil {
		t.Fatalf("OracleDB.QueryContext returned error: %v", err)
	}
	if len(columns) != 1 || columns[0] != "ddl" {
		t.Fatalf("oracle grid columns = %v, want [ddl]", columns)
	}
	if len(data) != 1 {
		t.Fatalf("oracle grid rows = %d, want 1", len(data))
	}

	value, ok := data[0]["ddl"].(string)
	if !ok {
		t.Fatalf("oracle grid LONG cell type = %T, want string", data[0]["ddl"])
	}
	fullText := buildScanRowsOracleLongViewText()
	if value != fullText {
		t.Fatalf("oracle grid LONG cell truncated: got length=%d want length=%d", len(value), len(fullText))
	}
	if strings.Contains(value, "[CLOB preview:") || strings.Contains(value, "[TEXT preview:") {
		t.Fatalf("oracle grid LONG cell carries a preview marker: %q", value[:min(len(value), 96)])
	}
}
