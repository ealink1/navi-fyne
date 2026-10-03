package main

import (
	"context"
	"github.com/ealink1/super-link/internal/upstream/db"
	"strings"
)

func handleCatalogRequest(requestCtx context.Context, runtimeState *agentRuntime, req agentRequest) agentResponse {
	resp := agentResponse{ID: req.ID, Success: true}
	method := strings.TrimSpace(req.Method)
	switch method {
	case agentMethodGetDatabases:
		data, err := runtimeState.inst.GetDatabases()
		if err != nil {
			return fail(resp, err.Error())
		}
		resp.Data = data
	case agentMethodGetTables:
		data, err := runtimeState.inst.GetTables(req.DBName)
		if err != nil {
			return fail(resp, err.Error())
		}
		resp.Data = data
	case agentMethodTableExists:
		if checker, ok := runtimeState.inst.(db.TableExistsChecker); ok {
			exists, err := checker.TableExists(req.DBName, req.TableName)
			if err != nil {
				return fail(resp, err.Error())
			}
			resp.Data = exists
			break
		}
		tables, err := runtimeState.inst.GetTables(req.DBName)
		if err != nil {
			return fail(resp, err.Error())
		}
		target := strings.TrimSpace(req.TableName)
		exists := false
		for _, table := range tables {
			if strings.TrimSpace(table) == target {
				exists = true
				break
			}
		}
		resp.Data = exists
	case agentMethodGetCreateStmt:
		data, err := runtimeState.inst.GetCreateStatement(req.DBName, req.TableName)
		if err != nil {
			return fail(resp, err.Error())
		}
		resp.Data = data
	case agentMethodGetColumns:
		data, err := runtimeState.inst.GetColumns(req.DBName, req.TableName)
		if err != nil {
			return fail(resp, err.Error())
		}
		resp.Data = data
	case agentMethodGetAllColumns:
		data, err := runtimeState.inst.GetAllColumns(req.DBName)
		if err != nil {
			return fail(resp, err.Error())
		}
		resp.Data = data
	case agentMethodGetIndexes:
		data, err := runtimeState.inst.GetIndexes(req.DBName, req.TableName)
		if err != nil {
			return fail(resp, err.Error())
		}
		resp.Data = data
	case agentMethodGetForeignKey:
		data, err := runtimeState.inst.GetForeignKeys(req.DBName, req.TableName)
		if err != nil {
			return fail(resp, err.Error())
		}
		resp.Data = data
	case agentMethodGetTriggers:
		data, err := runtimeState.inst.GetTriggers(req.DBName, req.TableName)
		if err != nil {
			return fail(resp, err.Error())
		}
		resp.Data = data
	default:
		return fail(resp, "不支持的方法")
	}
	return resp
}
