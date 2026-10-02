package main

import (
	"context"
	"strings"
)

func handleSQLRequest(requestCtx context.Context, runtimeState *agentRuntime, req agentRequest) agentResponse {
	resp := agentResponse{ID: req.ID, Success: true}
	method := strings.TrimSpace(req.Method)
	switch method {
	case agentMethodPing:
		if err := runtimeState.inst.Ping(); err != nil {
			return fail(resp, err.Error())
		}
	case agentMethodQuery:
		if len(req.Args) > 0 {
			ctx, cancel, budget := agentQueryRequestContext(requestCtx, req.TimeoutMs, req.RowBudget)
			defer cancel()
			data, fields, messages, err := queryWithArgsOptionalTimeout(ctx, runtimeState.inst, req.Query, req.Args, 0)
			if err != nil {
				return fail(resp, err.Error())
			}
			resp.Data = data
			resp.Fields = fields
			resp.Messages = messages
			applyAgentBudgetResponse(&resp, budget)
			break
		}
		data, fields, messages, budget, err := queryWithMessagesRequest(requestCtx, runtimeState.inst, req.Query, req.TimeoutMs, req.RowBudget)
		if err != nil {
			return fail(resp, err.Error())
		}
		resp.Data = data
		resp.Fields = fields
		resp.Messages = messages
		applyAgentBudgetResponse(&resp, budget)
	case agentMethodQueryMulti:
		data, messages, supported, budget, err := queryMultiWithMessagesRequest(requestCtx, runtimeState.inst, req.Query, req.TimeoutMs, req.RowBudget)
		if err != nil {
			return fail(resp, err.Error())
		}
		if !supported {
			return fail(resp, "当前驱动不支持原生多结果集查询")
		}
		resp.Data = data
		resp.Messages = messages
		applyAgentBudgetResponse(&resp, budget)
	case agentMethodExec:
		if len(req.Args) > 0 {
			affected, err := execWithArgsOptionalTimeout(requestCtx, runtimeState.inst, req.Query, req.Args, req.TimeoutMs)
			if err != nil {
				return fail(resp, err.Error())
			}
			resp.RowsAffected = affected
			break
		}
		affected, err := execWithOptionalTimeout(requestCtx, runtimeState.inst, req.Query, req.TimeoutMs)
		if err != nil {
			return fail(resp, err.Error())
		}
		resp.RowsAffected = affected
	default:
		return fail(resp, "不支持的方法")
	}
	return resp
}
