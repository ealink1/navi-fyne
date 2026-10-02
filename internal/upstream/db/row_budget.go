package db

import (
	"context"
	"sync"
)

// RowBudgetOptions 定义单次交互式查询的物化上限。
type RowBudgetOptions struct {
	PreserveBinary   bool  `json:"preserveBinary,omitempty"`
	MaxRowsPerResult int   `json:"maxRowsPerResult,omitempty"`
	MaxTotalRows     int   `json:"maxTotalRows,omitempty"`
	MaxTotalBytes    int64 `json:"maxTotalBytes,omitempty"`
	MaxFieldBytes    int   `json:"maxFieldBytes,omitempty"`
}

// RowBudget 限制单次查询物化的行数与估算字节数。
type RowBudget struct {
	mu              sync.Mutex
	options         RowBudgetOptions
	totalRows       int
	totalBytes      int64
	truncated       bool
	exhausted       bool
	resultTruncated bool
}

// NewRowBudget 创建行预算；maxRowsPerResult 非正时返回 nil（不限制），
// 使预算检查点无需区分 nil 与零值。
func NewRowBudget(maxRowsPerResult int) *RowBudget {
	return NewRowBudgetWithOptions(RowBudgetOptions{MaxRowsPerResult: maxRowsPerResult})
}

// NewRowBudgetWithOptions 创建复合结果预算；所有上限均非正时返回 nil。
func NewRowBudgetWithOptions(options RowBudgetOptions) *RowBudget {
	if options.MaxRowsPerResult <= 0 && options.MaxTotalRows <= 0 && options.MaxTotalBytes <= 0 && options.MaxFieldBytes <= 0 {
		return nil
	}
	return &RowBudget{options: options}
}

// MaxRowsPerResult 返回每个结果集的行数上限，0 表示不限制。
func (b *RowBudget) MaxRowsPerResult() int {
	if b == nil {
		return 0
	}
	return b.options.MaxRowsPerResult
}

// MaxTotalRows 返回本次查询所有结果集共享的行数上限。
func (b *RowBudget) MaxTotalRows() int {
	if b == nil {
		return 0
	}
	return b.options.MaxTotalRows
}

// MaxTotalBytes 返回本次查询所有结果集共享的估算字节上限。
func (b *RowBudget) MaxTotalBytes() int64 {
	if b == nil {
		return 0
	}
	return b.options.MaxTotalBytes
}

// MaxFieldBytes 返回单字段交互式预览上限。
func (b *RowBudget) MaxFieldBytes() int {
	if b == nil {
		return 0
	}
	return b.options.MaxFieldBytes
}

// Options 返回预算配置的副本。
func (b *RowBudget) Options() RowBudgetOptions {
	if b == nil {
		return RowBudgetOptions{}
	}
	return b.options
}

// RemainingOptions 返回适合下传到子进程或后续语句的剩余预算。
func (b *RowBudget) RemainingOptions() RowBudgetOptions {
	if b == nil {
		return RowBudgetOptions{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	options := b.options
	if options.MaxTotalRows > 0 {
		options.MaxTotalRows -= b.totalRows
		if options.MaxTotalRows < 0 {
			options.MaxTotalRows = 0
		}
	}
	if options.MaxTotalBytes > 0 {
		options.MaxTotalBytes -= b.totalBytes
		if options.MaxTotalBytes < 0 {
			options.MaxTotalBytes = 0
		}
	}
	return options
}

// CanMaterializeRow 在扫描当前行前检查行数预算。
func (b *RowBudget) CanMaterializeRow(rowsInResult int) bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if (b.options.MaxRowsPerResult > 0 && rowsInResult >= b.options.MaxRowsPerResult) ||
		(b.options.MaxTotalRows > 0 && b.totalRows >= b.options.MaxTotalRows) ||
		(b.options.MaxTotalBytes > 0 && b.totalBytes >= b.options.MaxTotalBytes) {
		b.markExhaustedLocked()
		return false
	}
	return true
}

// ConsumeRow 在追加当前行前登记估算字节；超限时拒绝该行。
func (b *RowBudget) ConsumeRow(estimatedBytes int64) bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if estimatedBytes < 0 {
		estimatedBytes = 0
	}
	if b.options.MaxTotalBytes > 0 && b.totalBytes+estimatedBytes > b.options.MaxTotalBytes {
		b.markExhaustedLocked()
		return false
	}
	b.totalRows++
	b.totalBytes += estimatedBytes
	return true
}

// MarkTruncated 记录“达到预算后停止读取”。
func (b *RowBudget) MarkTruncated() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.markExhaustedLocked()
}

// MarkFieldTruncated 记录当前结果集发生字段预览，但不停止后续行扫描。
func (b *RowBudget) MarkFieldTruncated() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.truncated = true
	b.resultTruncated = true
}

func (b *RowBudget) markExhaustedLocked() {
	b.truncated = true
	b.exhausted = true
	b.resultTruncated = true
}

// Truncated 报告扫描是否停读或字段是否被替换为有界预览。
func (b *RowBudget) Truncated() bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

// Exhausted 报告是否必须停止读取剩余行和结果集。
func (b *RowBudget) Exhausted() bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.exhausted
}

// TakeResultTruncated 返回并清除当前结果集的截断标记。
func (b *RowBudget) TakeResultTruncated() bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	truncated := b.resultTruncated
	b.resultTruncated = false
	return truncated
}

type rowBudgetContextKey struct{}

// ContextWithRowBudget 将行预算绑定到查询 context。db 层的扫描函数据此在
// 达到上限后停止 rows.Next 并让调用方的 rows.Close 释放连接。
func ContextWithRowBudget(ctx context.Context, budget *RowBudget) context.Context {
	if ctx == nil || budget == nil {
		return ctx
	}
	return context.WithValue(ctx, rowBudgetContextKey{}, budget)
}

// RowBudgetFromContext 返回 context 中绑定的行预算；未绑定时返回 nil。
func RowBudgetFromContext(ctx context.Context) *RowBudget {
	if ctx == nil {
		return nil
	}
	budget, _ := ctx.Value(rowBudgetContextKey{}).(*RowBudget)
	return budget
}
