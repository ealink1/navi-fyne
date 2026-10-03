package main

import (
	"context"
	"fmt"
	"github.com/ealink1/super-link/internal/upstream/connection"
	"github.com/ealink1/super-link/internal/upstream/db"
	"strings"
	"time"
)

func handleLifecycleRequest(requestCtx context.Context, runtimeState *agentRuntime, req agentRequest, progressReporter connection.SSHProgressReporter) agentResponse {
	resp := agentResponse{ID: req.ID, Success: true}
	method := strings.TrimSpace(req.Method)
	switch method {
	case agentMethodConnect:
		if req.Config == nil {
			return fail(resp, "连接配置为空")
		}
		config := *req.Config
		if config.UseSSH {
			config.SSH = config.SSH.WithRuntimeSnapshot(req.SSHRuntime)
			if progressReporter != nil {
				config.SSH = config.SSH.WithProgressReporter(progressReporter)
			}
		}
		runtimeState.close()
		next := agentDatabaseFactory()
		if next == nil {
			return fail(resp, "驱动代理初始化失败")
		}
		if err := next.Connect(config); err != nil {
			return failWithSSHHostKeyTrust(resp, err)
		}
		runtimeState.inst = next
		connectionInfo := agentConnectionInfo{ProtocolSchema: agentProtocolSchemaV2, InFlightCancel: true, TypedArguments: true}
		if versionProvider, ok := next.(db.ElasticsearchServerVersionProvider); ok {
			connectionInfo.ElasticsearchServerMajor = versionProvider.ElasticsearchServerMajor()
		}
		resp.Data = connectionInfo
		return resp
	case agentMethodClose:
		if runtimeState.inst != nil {
			if err := runtimeState.close(); err != nil {
				return fail(resp, err.Error())
			}
		}
		return resp
	case agentMethodMetadata:
		resp.Data = map[string]string{
			"driverType":     strings.TrimSpace(agentDriverType),
			"agentRevision":  db.OptionalDriverAgentRevision(agentDriverType),
			"protocolSchema": agentProtocolSchemaV2,
		}
		return resp
	case agentMethodOpenSession:
		if runtimeState.inst == nil {
			return fail(resp, "connection not open")
		}
		provider, ok := runtimeState.inst.(db.SessionExecerProvider)
		if !ok {
			return fail(resp, fmt.Sprintf("当前数据源（%s）不支持 SQL 编辑器托管事务", strings.TrimSpace(agentDriverType)))
		}
		openCtx := context.Background()
		var cancel context.CancelFunc
		if req.TimeoutMs > 0 {
			openCtx, cancel = context.WithTimeout(context.Background(), time.Duration(req.TimeoutMs)*time.Millisecond)
			defer cancel()
		}
		session, err := provider.OpenSessionExecer(openCtx)
		if err != nil {
			return fail(resp, err.Error())
		}
		sessionID := runtimeState.nextID()
		runtimeState.sessions[sessionID] = session
		resp.Data = sessionID
		return resp
	case agentMethodOpenTransaction:
		if runtimeState.inst == nil {
			return fail(resp, "connection not open")
		}
		provider, ok := runtimeState.inst.(db.TransactionExecerProvider)
		if !ok {
			return fail(resp, fmt.Sprintf("当前数据源（%s）不支持 SQL 编辑器托管事务", strings.TrimSpace(agentDriverType)))
		}
		// The transaction must outlive this request and be finished by a later RPC.
		transaction, err := provider.OpenTransactionExecer(context.Background())
		if err != nil {
			return fail(resp, err.Error())
		}
		sessionID := runtimeState.nextID()
		runtimeState.sessions[sessionID] = transaction
		resp.Data = sessionID
		return resp
	case agentMethodCloseSession:
		if err := runtimeState.closeSession(req.SessionID); err != nil {
			return fail(resp, err.Error())
		}
		return resp
	}

	return resp
}
