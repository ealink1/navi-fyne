package main

import (
	"context"
	"github.com/ealink1/navi-fyne/internal/upstream/connection"
	"github.com/ealink1/navi-fyne/internal/upstream/db"
	"strings"
)

func handleChangesRequest(requestCtx context.Context, runtimeState *agentRuntime, req agentRequest) agentResponse {
	resp := agentResponse{ID: req.ID, Success: true}
	method := strings.TrimSpace(req.Method)
	switch method {
	case agentMethodApplyChanges:
		if req.Changes == nil {
			return fail(resp, "变更集为空")
		}
		applier, ok := runtimeState.inst.(interface {
			ApplyChanges(tableName string, changes connection.ChangeSet) error
		})
		if !ok {
			return fail(resp, "当前驱动不支持 ApplyChanges")
		}
		if err := applier.ApplyChanges(req.TableName, *req.Changes); err != nil {
			resp = fail(resp, err.Error())
			resp.OutcomeUnknown = db.IsWriteOutcomeUnknown(err)
			return resp
		}
	default:
		return fail(resp, "不支持的方法")
	}
	return resp
}
