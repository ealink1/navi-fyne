package main

import (
	"context"
	"errors"
	"time"

	"github.com/ealink1/navi-fyne/internal/upstream/db"
)

// agentProtocolSchemaV2 是当前 agent 支持的 IPC 协议版本：connect 响应与
// metadata 都回显该值，主进程据此决定是否开放参数绑定。
const agentProtocolSchemaV2 = "json-lines-v2"

// 参数绑定的执行入口按 req.Args 是否非空分流：空参数走既有无参路径（老
// 行为完全不变）；带参数时断言目标实现的参数化契约，断言失败返回可操作
// 的失败响应，绝不把含占位符的 SQL 原样下发到数据库。

func queryWithArgsOptionalTimeout(requestCtx context.Context, inst db.Database, query string, args []any, timeoutMs int64) ([]map[string]interface{}, []string, []string, error) {
	target, ok := inst.(db.QueryArgsContexter)
	if !ok {
		return nil, nil, nil, errors.New("当前驱动不支持参数绑定")
	}
	ctx, cancel := agentArgsContext(requestCtx, timeoutMs)
	if cancel != nil {
		defer cancel()
	}
	data, fields, err := target.QueryContextWithArgs(ctx, query, args)
	return data, fields, nil, err
}

func execWithArgsOptionalTimeout(requestCtx context.Context, inst db.Database, query string, args []any, timeoutMs int64) (int64, error) {
	target, ok := inst.(db.ExecArgsContexter)
	if !ok {
		return 0, errors.New("当前驱动不支持参数绑定")
	}
	ctx, cancel := agentArgsContext(requestCtx, timeoutMs)
	if cancel != nil {
		defer cancel()
	}
	return target.ExecContextWithArgs(ctx, query, args)
}

func queryStatementWithArgsOptionalTimeout(requestCtx context.Context, inst db.StatementExecer, query string, args []any, timeoutMs int64) ([]map[string]interface{}, []string, []string, error) {
	target, ok := inst.(db.StatementQueryArgsExecer)
	if !ok {
		return nil, nil, nil, errors.New("当前事务会话不支持参数绑定")
	}
	ctx, cancel := agentArgsContext(requestCtx, timeoutMs)
	if cancel != nil {
		defer cancel()
	}
	data, fields, err := target.QueryContextWithArgs(ctx, query, args)
	return data, fields, nil, err
}

func execStatementWithArgsOptionalTimeout(requestCtx context.Context, inst db.StatementExecer, query string, args []any, timeoutMs int64) (int64, error) {
	target, ok := inst.(db.StatementExecArgsExecer)
	if !ok {
		return 0, errors.New("当前事务会话不支持参数绑定")
	}
	ctx, cancel := agentArgsContext(requestCtx, timeoutMs)
	if cancel != nil {
		defer cancel()
	}
	return target.ExecContextWithArgs(ctx, query, args)
}

// agentArgsContext 与既有可选超时语义一致：timeoutMs <= 0 时由数据库/驱动
// 自身控制时长；parent 始终为请求生命周期上下文，取消通知据此中止参数化语句。
func agentArgsContext(parent context.Context, timeoutMs int64) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if timeoutMs <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, time.Duration(timeoutMs)*time.Millisecond)
}
