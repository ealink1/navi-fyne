package db

import (
	"context"

	"github.com/ealink1/super-link/internal/upstream/connection"
)

func recordOptionalAgentBudgetResponse(request optionalAgentRequest, output interface{}, response optionalAgentResponse) {
	budget := request.rowBudget
	if budget == nil {
		return
	}
	switch result := output.(type) {
	case *[]map[string]interface{}:
		for _, row := range *result {
			budget.ConsumeRow(estimateQueryRowBytes(row))
		}
		if response.BudgetExhausted {
			budget.MarkTruncated()
		} else if response.Truncated {
			budget.MarkFieldTruncated()
		}
	case *[]connection.ResultSetData:
		for _, resultSet := range *result {
			for _, row := range resultSet.Rows {
				budget.ConsumeRow(estimateQueryRowBytes(row))
			}
		}
		if response.BudgetExhausted {
			budget.MarkTruncated()
		}
	}
}

func applyOptionalAgentRequestBudget(ctx context.Context, request *optionalAgentRequest) {
	if request == nil {
		return
	}
	budget := RowBudgetFromContext(ctx)
	if budget == nil {
		return
	}
	options := budget.RemainingOptions()
	request.RowBudget = &options
	request.rowBudget = budget
}

func (d *OptionalDriverAgentDB) QueryContextWithMessages(ctx context.Context, query string) ([]map[string]interface{}, []string, []string, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}
	client, err := d.requireClient()
	if err != nil {
		return nil, nil, nil, err
	}
	var data []map[string]interface{}
	var fields []string
	var messages []string
	request := optionalAgentRequest{Method: optionalAgentMethodQuery, Query: query, TimeoutMs: timeoutMsFromContext(ctx)}
	applyOptionalAgentRequestBudget(ctx, &request)
	if err := client.callContext(ctx, request, &data, &fields, &messages, nil); err != nil {
		return nil, nil, nil, err
	}
	return data, fields, messages, nil
}

func (d *OptionalDriverAgentDB) QueryMultiContext(ctx context.Context, query string) ([]connection.ResultSetData, error) {
	results, _, err := d.QueryMultiContextWithMessages(ctx, query)
	return results, err
}

func (d *OptionalDriverAgentDB) QueryMultiContextWithMessages(ctx context.Context, query string) ([]connection.ResultSetData, []string, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	client, err := d.requireClient()
	if err != nil {
		return nil, nil, err
	}
	var results []connection.ResultSetData
	var messages []string
	request := optionalAgentRequest{Method: optionalAgentMethodQueryMulti, Query: query, TimeoutMs: timeoutMsFromContext(ctx)}
	applyOptionalAgentRequestBudget(ctx, &request)
	if err := client.callContext(ctx, request, &results, nil, &messages, nil); err != nil {
		if isOptionalAgentMultiResultUnsupportedError(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	return results, messages, nil
}

func (s *optionalDriverAgentSession) QueryContext(ctx context.Context, query string) ([]map[string]interface{}, []string, error) {
	data, fields, _, err := s.QueryContextWithMessages(ctx, query)
	return data, fields, err
}

func (s *optionalDriverAgentSession) QueryContextWithMessages(ctx context.Context, query string) ([]map[string]interface{}, []string, []string, error) {
	if err := s.ensureOpen(); err != nil {
		return nil, nil, nil, err
	}
	var data []map[string]interface{}
	var fields []string
	var messages []string
	request := optionalAgentRequest{
		Method: optionalAgentMethodQuery, SessionID: s.sessionID, Query: query, TimeoutMs: timeoutMsFromContext(ctx),
	}
	applyOptionalAgentRequestBudget(ctx, &request)
	if err := s.client.callContext(ctx, request, &data, &fields, &messages, nil); err != nil {
		return nil, nil, nil, err
	}
	return data, fields, messages, nil
}
