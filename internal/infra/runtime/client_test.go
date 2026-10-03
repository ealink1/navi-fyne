package runtime

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/upstream/connection"
)

func TestOrderedValuesAndBoundedUTF8(t *testing.T) {
	long := strings.Repeat("中", MaxFieldBytes/3+1)
	result := Ordered([]map[string]interface{}{{"text": long, "a": int64(9223372036854775807), "empty": nil}}, []string{"a", "empty", "text"})
	if result.Rows[0][0] != int64(9223372036854775807) || result.Rows[0][1] != nil {
		t.Fatal("value fidelity")
	}
	if !result.Truncated || !utf8.ValidString(result.Rows[0][2].(string)) {
		t.Fatal("invalid UTF8 field preview")
	}
	rows := make([]map[string]interface{}, MaxRows+1)
	for i := range rows {
		rows[i] = map[string]interface{}{"a": i}
	}
	result = Ordered(rows, []string{"a"})
	if len(result.Rows) != MaxRows || !result.Truncated {
		t.Fatal("row budget not enforced")
	}
}

func TestRuntimePreservesFineGrainedProtection(t *testing.T) {
	p := domain.Profile{Config: connection.ConnectionConfig{Type: "mysql", Protection: connection.ConnectionProtectionConfig{RestrictDataEdit: true}}}
	actual, err := ConfigForScope(p, "")
	if err != nil || !actual.Config.Protection.RestrictDataEdit {
		t.Fatal("protection was lost", err)
	}
}

func TestJSONNumbersKeepPrecision(t *testing.T) {
	results, err := JSONResult(map[string]any{"id": int64(9223372036854775807)})
	if err != nil || results[0].Rows[0][0].(json.Number).String() != "9223372036854775807" {
		t.Fatal("JSON number rounded")
	}
}
func TestSchemaSelectionPreservesOracleTransport(t *testing.T) {
	for _, kind := range []string{"oracle", "oceanbase"} {
		p := domain.Profile{Config: connection.ConnectionConfig{Type: kind, Database: "ORCLPDB1", OceanBaseProtocol: "oracle"}}
		selected, err := ConfigForScope(p, "APP_OWNER")
		if err != nil || selected.Config.Database != "ORCLPDB1" || selected.Scope != "APP_OWNER" {
			t.Fatal("schema replaced Oracle service")
		}
	}
	p := domain.Profile{Config: connection.ConnectionConfig{Type: "postgres", Host: "127.0.0.1", URI: "postgresql://user:password@db.test:5433/base"}}
	selected, err := ConfigForScope(p, "selected")
	if err != nil || selected.Config.Database != "selected" || selected.Config.Host != "db.test" || selected.Config.Port != 5433 {
		t.Fatal("URI or database selection ignored")
	}
	p.Config = connection.ConnectionConfig{Type: "custom", Driver: "mysql", DSN: "opaque", Database: "base"}
	if _, err = ConfigForScope(p, "different"); err == nil {
		t.Fatal("silently ignored a custom DSN database switch")
	}
}
func TestMongoCommandOrderAndLimit(t *testing.T) {
	text, err := mongoCommand(`{"filter":{"name":"中文"},"find":"items","limit":0}`)
	if err != nil || !strings.HasPrefix(text, `{"find":`) || !strings.Contains(text, `"limit":10001`) {
		t.Fatal(text, err)
	}
	if _, err = mongoCommand(`{"find":"a","drop":"b"}`); err == nil {
		t.Fatal("accepted ambiguous command")
	}
}
