package ui

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ealink1/super-link/internal/domain"
)

func TestCSVPreservesNumbersAndNeutralizesFormulas(t *testing.T) {
	r := domain.Result{Columns: []domain.Column{{Name: "=formula"}, {Name: "integer"}, {Name: "null"}}, Rows: [][]any{{"  =SUM(1,2)", json.Number("-9223372036854775807"), nil}}}
	var output bytes.Buffer
	if err := writeResult(context.Background(), &output, "CSV", r); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(output.String())).ReadAll()
	if err != nil || rows[0][0] != "'=formula" || rows[1][0] != "'  =SUM(1,2)" || rows[1][1] != "-9223372036854775807" || rows[1][2] != "NULL" {
		t.Fatal(rows, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = writeResult(ctx, &output, "CSV", r); !errors.Is(err, context.Canceled) {
		t.Fatal("ignored cancellation")
	}
}
