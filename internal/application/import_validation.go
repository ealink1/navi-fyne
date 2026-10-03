package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ealink1/super-link/internal/domain"
)

func (e *Engine) authorizeImport(p domain.Profile, request domain.ImportRequest) error {
	if request.Object.Kind == "view" || len(request.Rows) == 0 || len(request.Rows) > 100000 || len(request.Columns) == 0 || len(request.Columns) > 1024 || len(request.Mapping) == 0 || request.BatchSize < 1 || request.BatchSize > 1000 {
		return errors.New("invalid import target, size or mapping")
	}
	if len(request.Rows)*len(request.Columns) > 2000000 {
		return errors.New("import exceeds 2,000,000 cells")
	}
	var bytes int64
	for _, row := range request.Rows {
		if len(row) != len(request.Columns) {
			return errors.New("import row and column counts do not match")
		}
		for _, value := range row {
			fieldBytes, err := importValueSize(value, 0)
			if err != nil || fieldBytes > 1<<20 {
				return errors.New("invalid import value or field larger than 1 MiB")
			}
			bytes += fieldBytes
			if bytes > 64<<20 {
				return errors.New("import exceeds 64 MiB")
			}
		}
	}
	bound := request
	bound.Confirmation = ""
	raw, err := json.Marshal(bound)
	if err != nil || len(raw) > 64<<20 {
		return errors.New("invalid import or content larger than 64 MiB")
	}
	digest := sha256.Sum256(raw)
	return e.authorize(p, domain.Execution{Text: "import:" + hex.EncodeToString(digest[:]), Scope: request.Object.Scope, Confirmation: request.Confirmation})
}

// Bound nested source values before marshaling the request to bind a grant.
func importValueSize(value any, depth int) (int64, error) {
	if depth > 32 {
		return 0, errors.New("nested import data exceeds 32 levels")
	}
	var total int64
	switch v := value.(type) {
	case nil:
		return 4, nil
	case bool:
		return 5, nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return 28, nil
	case time.Time:
		return 40, nil
	case string:
		return int64(len(v)), nil
	case json.Number:
		return int64(len(v)), nil
	case []byte:
		return int64(len(v))*4/3 + 4, nil
	case map[string]any:
		if len(v) > 100000 {
			return 0, errors.New("too many nested fields")
		}
		for key, item := range v {
			size, err := importValueSize(item, depth+1)
			if err != nil {
				return 0, err
			}
			total += int64(len(key)) + size + 8
			if total > 1<<20 {
				return total, nil
			}
		}
	case []any:
		if len(v) > 100000 {
			return 0, errors.New("too many nested values")
		}
		for _, item := range v {
			size, err := importValueSize(item, depth+1)
			if err != nil {
				return 0, err
			}
			total += size + 1
			if total > 1<<20 {
				return total, nil
			}
		}
	default:
		return 0, fmt.Errorf("unsupported import value %T", value)
	}
	return total, nil
}
