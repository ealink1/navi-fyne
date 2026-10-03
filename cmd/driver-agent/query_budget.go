package main

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/ealink1/super-link/internal/upstream/db"
	sshbridge "github.com/ealink1/super-link/internal/upstream/ssh"
)

func fail(response agentResponse, errorText string) agentResponse {
	response.Success = false
	response.Error = strings.TrimSpace(errorText)
	return response
}

func failWithSSHHostKeyTrust(response agentResponse, err error) agentResponse {
	response = fail(response, err.Error())
	if status, ok := sshbridge.HostKeyTrustStatusFromError(err); ok {
		response.SSHHostKeyTrust = &status
	}
	return response
}

func normalizeAgentResponseData(value interface{}) interface{} {
	if value == nil {
		return nil
	}

	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Pointer, reflect.Interface:
		if reflected.IsNil() {
			return nil
		}
		return normalizeAgentResponseData(reflected.Elem().Interface())
	case reflect.Map:
		if reflected.IsNil() {
			return nil
		}
		out := make(map[string]interface{}, reflected.Len())
		iter := reflected.MapRange()
		for iter.Next() {
			out[fmt.Sprint(iter.Key().Interface())] = normalizeAgentResponseData(iter.Value().Interface())
		}
		return out
	case reflect.Slice:
		if reflected.IsNil() {
			return nil
		}
		if reflected.Type().Elem().Kind() == reflect.Uint8 {
			return value
		}
		size := reflected.Len()
		items := make([]interface{}, size)
		for index := 0; index < size; index++ {
			items[index] = normalizeAgentResponseData(reflected.Index(index).Interface())
		}
		return items
	case reflect.Array:
		size := reflected.Len()
		items := make([]interface{}, size)
		for index := 0; index < size; index++ {
			items[index] = normalizeAgentResponseData(reflected.Index(index).Interface())
		}
		return items
	default:
		return value
	}
}

// agentQueryRequestContext 以请求生命周期上下文为父构建查询上下文。
//
// parent 会由取消通知取消，因此即使没有配置超时，查询上下文也是可取消的：这是「停止」
// 能传递到驱动层（如 SQL Server 的 TDS attention）的前提，而不是等查询自然结束。
func agentQueryRequestContext(parent context.Context, timeoutMs int64, options *db.RowBudgetOptions) (context.Context, context.CancelFunc, *db.RowBudget) {
	if parent == nil {
		parent = context.Background()
	}
	ctx := parent
	cancel := func() {}
	if timeoutMs > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	}
	var budget *db.RowBudget
	if options != nil {
		budget = db.NewRowBudgetWithOptions(*options)
		ctx = db.ContextWithRowBudget(ctx, budget)
	}
	return ctx, cancel, budget
}

// agentStreamRequestContext 与 agentQueryRequestContext 同样以请求上下文为父，
// 供流式查询与写入复用。
func agentStreamRequestContext(parent context.Context, timeoutMs int64) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if timeoutMs > 0 {
		return context.WithTimeout(parent, time.Duration(timeoutMs)*time.Millisecond)
	}
	return parent, func() {}
}

func applyAgentBudgetResponse(response *agentResponse, budget *db.RowBudget) {
	if response == nil || budget == nil {
		return
	}
	response.Truncated = budget.Truncated()
	response.BudgetExhausted = budget.Exhausted()
}
