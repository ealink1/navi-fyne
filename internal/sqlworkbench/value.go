package sqlworkbench

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ConvertValue keeps integer and decimal precision when binding edited values.
func ConvertValue(kind string, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	text := fmt.Sprint(value)
	if object, ok := value.(map[string]any); ok {
		raw, err := json.Marshal(object)
		if err != nil {
			return nil, err
		}
		text = string(raw)
	}
	if list, ok := value.([]any); ok {
		raw, err := json.Marshal(list)
		if err != nil {
			return nil, err
		}
		text = string(raw)
	}
	upper := strings.ToUpper(kind)
	switch {
	case strings.Contains(upper, "BOOL"):
		return strconv.ParseBool(text)
	case strings.Contains(upper, "INT"):
		value, err := strconv.ParseInt(text, 10, 64)
		if err == nil {
			return value, nil
		}
		if _, unsignedErr := strconv.ParseUint(text, 10, 64); unsignedErr == nil {
			return text, nil
		}
		return nil, err
	case strings.Contains(upper, "REAL") || strings.Contains(upper, "DOUBLE") || strings.Contains(upper, "FLOAT"):
		value, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return nil, err
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, errors.New("value must be a finite number")
		}
		return value, nil
	case strings.Contains(upper, "NUMERIC") || strings.Contains(upper, "DECIMAL") || strings.Contains(upper, "NUMBER"):
		value, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, errors.New("invalid decimal value")
		}
		return text, nil
	case strings.Contains(upper, "BLOB") || strings.Contains(upper, "BYTEA") || strings.Contains(upper, "BINARY"):
		if bytes, ok := value.([]byte); ok {
			return bytes, nil
		}
		return base64.StdEncoding.DecodeString(text)
	default:
		return text, nil
	}
}
