// Package runtime adapts protocol clients without depending on any UI toolkit.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ealink1/navi-fyne/internal/domain"
	"github.com/ealink1/navi-fyne/internal/upstream/connection"
)

const MaxRows = 10000
const MaxBytes = 64 << 20
const MaxFieldBytes = 1 << 20

type Client interface {
	Close() error
	Scopes(context.Context) ([]string, error)
	Objects(context.Context, string) ([]domain.Object, error)
	Schema(context.Context, string, string) (string, error)
	Execute(context.Context, domain.Execution) ([]domain.Result, error)
}

type Factory func(context.Context, domain.Profile) (Client, error)

func Open(ctx context.Context, p domain.Profile) (Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var err error
	p, err = ConfigForScope(p, p.Scope)
	if err != nil {
		return nil, err
	}
	descriptor, err := domain.Resolve(p.Config.Type)
	if err != nil {
		return nil, err
	}
	cfg, routes, err := routeConfig(p.Config, descriptor)
	if err != nil {
		return nil, err
	}
	p.Config = cfg
	var client Client
	switch descriptor.Family {
	case domain.Cache:
		client, err = openRedis(ctx, p.Config)
	case domain.Configuration:
		client, err = openNacos(ctx, p.Config)
	default:
		client, err = openDatabase(ctx, p, descriptor)
	}
	if err != nil {
		for _, route := range routes {
			_ = route.Close()
		}
		return nil, err
	}
	if len(routes) != 0 {
		client = &routedClient{Client: client, routes: routes}
	}
	return client, nil
}

func Ordered(rows []map[string]interface{}, columns []string) domain.Result {
	r := domain.Result{}
	for i, name := range columns {
		r.Columns = append(r.Columns, domain.Column{ID: fmt.Sprintf("col:%d", i), Name: name})
	}
	bytesUsed := 0
	for _, row := range rows {
		if len(r.Rows) >= MaxRows {
			r.Truncated = true
			break
		}
		values := make([]any, len(columns))
		for i, name := range columns {
			value := row[name]
			switch v := value.(type) {
			case string:
				if len(v) > MaxFieldBytes {
					end := MaxFieldBytes
					for end > 0 && !utf8.RuneStart(v[end]) {
						end--
					}
					value = v[:end] + "… [field truncated]"
					r.Truncated = true
				}
			case []byte:
				if len(v) > MaxFieldBytes {
					value = append([]byte(nil), v[:MaxFieldBytes]...)
					r.Truncated = true
				}
			}
			encoded, _ := json.Marshal(value)
			bytesUsed += len(encoded)
			values[i] = value
		}
		if bytesUsed > MaxBytes {
			r.Truncated = true
			break
		}
		r.Rows = append(r.Rows, values)
	}
	return r
}

func JSONResult(value any) ([]domain.Result, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxBytes {
		return nil, errors.New("response exceeds 64 MiB")
	}
	var rows []map[string]interface{}
	decode := func(target any) error {
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		return decoder.Decode(target)
	}
	if err = decode(&rows); err != nil {
		var row map[string]interface{}
		if err = decode(&row); err != nil {
			row = map[string]interface{}{"value": value}
		}
		rows = []map[string]interface{}{row}
	}
	keys := map[string]bool{}
	for _, row := range rows {
		for key := range row {
			keys[key] = true
		}
	}
	columns := make([]string, 0, len(keys))
	for key := range keys {
		columns = append(columns, key)
	}
	sort.Strings(columns)
	return []domain.Result{Ordered(rows, columns)}, nil
}

func connectionContext(ctx context.Context, cfg connection.ConnectionConfig) (context.Context, context.CancelFunc) {
	seconds := cfg.Timeout
	if seconds <= 0 {
		seconds = 15
	}
	if seconds > 120 {
		seconds = 120
	}
	return context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
}
