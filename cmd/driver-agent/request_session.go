package main

import (
	"context"
	"github.com/ealink1/navi-fyne/internal/upstream/db"
	"strings"
)

func handleSessionRequest(requestCtx context.Context, session db.StatementExecer, req agentRequest) agentResponse {
	resp := agentResponse{ID: req.ID, Success: true}
	method := strings.TrimSpace(req.Method)
	switch method {
	case agentMethodQuery:
		if len(req.Args) > 0 {
			ctx, cancel, budget := agentQueryRequestContext(requestCtx, req.TimeoutMs, req.RowBudget)
			defer cancel()
			data, fields, messages, err := queryStatementWithArgsOptionalTimeout(ctx, session, req.Query, req.Args, 0)
			if err != nil {
				return fail(resp, err.Error())
			}
			resp.Data = data
			resp.Fields = fields
			resp.Messages = messages
			applyAgentBudgetResponse(&resp, budget)
			break
		}
		data, fields, messages, budget, err := queryStatementWithMessagesRequest(requestCtx, session, req.Query, req.TimeoutMs, req.RowBudget)
		if err != nil {
			return fail(resp, err.Error())
		}
		resp.Data = data
		resp.Fields = fields
		resp.Messages = messages
		applyAgentBudgetResponse(&resp, budget)
	case agentMethodQueryMulti:
		data, messages, supported, budget, err := queryMultiStatementWithMessagesRequest(requestCtx, session, req.Query, req.TimeoutMs, req.RowBudget)
		if err != nil {
			return fail(resp, err.Error())
		}
		if !supported {
			return fail(resp, "当前事务会话不支持多结果集查询")
		}
		resp.Data = data
		resp.Messages = messages
		applyAgentBudgetResponse(&resp, budget)
	case agentMethodExec:
		if len(req.Args) > 0 {
			affected, err := execStatementWithArgsOptionalTimeout(requestCtx, session, req.Query, req.Args, req.TimeoutMs)
			if err != nil {
				return fail(resp, err.Error())
			}
			resp.RowsAffected = affected
			break
		}
		affected, err := execStatementWithOptionalTimeout(requestCtx, session, req.Query, req.TimeoutMs)
		if err != nil {
			return fail(resp, err.Error())
		}
		resp.RowsAffected = affected
	case agentMethodCommitTransaction, agentMethodRollbackTransaction:
		transaction, ok := session.(db.TransactionExecer)
		if !ok {
			return fail(resp, "当前会话不是托管事务")
		}
		var err error
		if method == agentMethodCommitTransaction {
			err = transaction.Commit()
		} else {
			err = transaction.Rollback()
		}
		if err != nil {
			return fail(resp, err.Error())
		}
	default:
		return fail(resp, "当前事务会话不支持该方法")
	}
	return resp
}
