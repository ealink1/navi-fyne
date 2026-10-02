package db

import "context"

// QueryArgsContexter 是支持按位置绑定参数执行查询的可选契约。
//
// 参数值来自 sqlparam.Bind 的产物，调用方保证 query 中占位符与 args 一一对应。
// 能力探测方式与 QueryContexter 相同：执行前对 db.Database 做类型断言，
// 未实现该契约的驱动由调用方给出可操作的限制说明。
type QueryArgsContexter interface {
	QueryContextWithArgs(ctx context.Context, query string, args []any) ([]map[string]interface{}, []string, error)
}

// ExecArgsContexter 是支持按位置绑定参数执行写操作的可选契约。
type ExecArgsContexter interface {
	ExecContextWithArgs(ctx context.Context, query string, args []any) (int64, error)
}

// StatementExecer 的事务/会话路径同样支持参数绑定；会话持有方在托管事务中
// 执行参数化语句时走这两个契约。
type StatementQueryArgsExecer interface {
	QueryContextWithArgs(ctx context.Context, query string, args []any) ([]map[string]interface{}, []string, error)
}

type StatementExecArgsExecer interface {
	ExecContextWithArgs(ctx context.Context, query string, args []any) (int64, error)
}
