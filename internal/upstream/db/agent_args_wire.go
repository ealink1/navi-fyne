package db

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"
)

// ErrInvalidAgentArguments excludes parameter values from diagnostics.
var ErrInvalidAgentArguments = errors.New("invalid or unsupported driver-agent parameter")

func (c *optionalDriverAgentClient) supportsTypedArguments() bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return agentSupportsParameterBinding(c.protocolSchema) && c.typedArguments
}

// EncodeAgentArguments preserves database/sql scalar types across JSON lines.
func EncodeAgentArguments(args []any) ([]any, []string, error) {
	if len(args) > 10000 {
		return nil, nil, ErrInvalidAgentArguments
	}
	values, kinds := make([]any, len(args)), make([]string, len(args))
	for i, arg := range args {
		value, kind, err := encodeAgentArgument(arg)
		if err != nil {
			return nil, nil, err
		}
		values[i], kinds[i] = value, kind
	}
	return values, kinds, nil
}

func encodeAgentArgument(arg any) (any, string, error) {
	switch value := arg.(type) {
	case nil:
		return nil, "null", nil
	case string:
		return value, "string", nil
	case bool:
		return value, "boolean", nil
	case int:
		return strconv.FormatInt(int64(value), 10), "integer", nil
	case int64:
		return strconv.FormatInt(value, 10), "integer", nil
	case float64:
		if !math.IsInf(value, 0) && !math.IsNaN(value) {
			return strconv.FormatFloat(value, 'g', -1, 64), "float", nil
		}
	case []byte:
		return base64.StdEncoding.EncodeToString(value), "binary", nil
	case time.Time:
		return value.Format(time.RFC3339Nano), "datetime", nil
	}
	return nil, "", ErrInvalidAgentArguments
}

// DecodeAgentArguments also accepts scalar legacy v2 requests. New clients only
// send bound requests after the typedArguments capability handshake succeeds.
func DecodeAgentArguments(args []any, kinds []string) ([]any, error) {
	if len(args) > 10000 || len(kinds) != 0 && len(kinds) != len(args) {
		return nil, ErrInvalidAgentArguments
	}
	values := make([]any, len(args))
	for i, value := range args {
		var err error
		if len(kinds) == 0 {
			values[i], err = decodeLegacyArgument(value)
		} else {
			values[i], err = decodeTypedArgument(value, kinds[i])
		}
		if err != nil {
			return nil, ErrInvalidAgentArguments
		}
	}
	return values, nil
}

func decodeTypedArgument(value any, kind string) (any, error) {
	if kind == "null" && value == nil {
		return nil, nil
	}
	if kind == "boolean" {
		if boolean, ok := value.(bool); ok {
			return boolean, nil
		}
	}
	text, ok := value.(string)
	if !ok {
		return nil, ErrInvalidAgentArguments
	}
	switch kind {
	case "string":
		return text, nil
	case "integer":
		return strconv.ParseInt(text, 10, 64)
	case "float":
		v, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsInf(v, 0) || math.IsNaN(v) {
			return nil, ErrInvalidAgentArguments
		}
		return v, nil
	case "binary":
		return base64.StdEncoding.DecodeString(text)
	case "datetime":
		return time.Parse(time.RFC3339Nano, text)
	}
	return nil, ErrInvalidAgentArguments
}

func decodeLegacyArgument(value any) (any, error) {
	if number, ok := value.(json.Number); ok {
		if integer, err := number.Int64(); err == nil {
			return integer, nil
		}
		if floating, err := number.Float64(); err == nil && !math.IsInf(floating, 0) && !math.IsNaN(floating) {
			return floating, nil
		}
		return nil, ErrInvalidAgentArguments
	}
	switch value.(type) {
	case nil, string, bool:
		return value, nil
	}
	return nil, ErrInvalidAgentArguments
}
