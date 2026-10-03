package main

import (
	"context"
	"fmt"
	"github.com/ealink1/super-link/internal/upstream/db"
	"strings"
	"time"
)

func handleExternalRequest(requestCtx context.Context, runtimeState *agentRuntime, req agentRequest) agentResponse {
	resp := agentResponse{ID: req.ID, Success: true}
	method := strings.TrimSpace(req.Method)
	switch method {
	case agentMethodElasticsearchConsole:
		if req.ElasticsearchRequest == nil {
			return fail(resp, "Elasticsearch Console 请求为空")
		}
		executor, ok := runtimeState.inst.(db.ElasticsearchConsoleExecutor)
		if !ok {
			return fail(resp, "当前驱动不支持 Elasticsearch Console")
		}
		executeCtx := context.Background()
		var cancel context.CancelFunc
		if req.TimeoutMs > 0 {
			executeCtx, cancel = context.WithTimeout(executeCtx, time.Duration(req.TimeoutMs)*time.Millisecond)
			defer cancel()
		}
		data, err := executor.ExecuteElasticsearchConsoleRequest(executeCtx, *req.ElasticsearchRequest)
		if err != nil {
			return fail(resp, err.Error())
		}
		resp.Data = data
	case agentMethodAttachExternalDatabase:
		if runtimeState.inst == nil {
			return fail(resp, "connection not open")
		}
		if req.AttachSpec == nil {
			return fail(resp, "attach spec is empty")
		}
		attacher, ok := runtimeState.inst.(db.ExternalDatabaseAttacher)
		if !ok {
			return fail(resp, fmt.Sprintf("当前数据源（%s）不支持附加外部数据源", strings.TrimSpace(agentDriverType)))
		}
		attachCtx := context.Background()
		var attachCancel context.CancelFunc
		if req.TimeoutMs > 0 {
			attachCtx, attachCancel = context.WithTimeout(attachCtx, time.Duration(req.TimeoutMs)*time.Millisecond)
			defer attachCancel()
		}
		if err := attacher.AttachExternalDatabase(attachCtx, *req.AttachSpec); err != nil {
			return failWithExternalAttachNotAttached(resp, err)
		}
		return resp
	case agentMethodDetachExternalDatabase:
		if runtimeState.inst == nil {
			return fail(resp, "connection not open")
		}
		attacher, ok := runtimeState.inst.(db.ExternalDatabaseAttacher)
		if !ok {
			return fail(resp, fmt.Sprintf("当前数据源（%s）不支持附加外部数据源", strings.TrimSpace(agentDriverType)))
		}
		detachCtx := context.Background()
		var detachCancel context.CancelFunc
		if req.TimeoutMs > 0 {
			detachCtx, detachCancel = context.WithTimeout(detachCtx, time.Duration(req.TimeoutMs)*time.Millisecond)
			defer detachCancel()
		}
		if err := attacher.DetachExternalDatabase(detachCtx, req.Alias); err != nil {
			return failWithExternalAttachNotAttached(resp, err)
		}
		return resp

	case agentMethodListExternalAttachments:
		if runtimeState.inst == nil {
			return fail(resp, "connection not open")
		}
		lister, ok := runtimeState.inst.(db.ExternalAttachmentLister)
		if !ok {
			return fail(resp, fmt.Sprintf("当前数据源（%s）不支持附加外部数据源", strings.TrimSpace(agentDriverType)))
		}
		listCtx := context.Background()
		var listCancel context.CancelFunc
		if req.TimeoutMs > 0 {
			listCtx, listCancel = context.WithTimeout(listCtx, time.Duration(req.TimeoutMs)*time.Millisecond)
			defer listCancel()
		}
		attachments, listErr := lister.ListExternalAttachments(listCtx)
		if listErr != nil {
			return fail(resp, listErr.Error())
		}
		resp.Data = attachments
		return resp
	default:
		return fail(resp, "不支持的方法")
	}
	return resp
}
