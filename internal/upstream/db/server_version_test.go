package db

import (
	"testing"

	"github.com/ealink1/super-link/internal/upstream/connection"
)

func TestServerVersionQueryCoversSQLDialectsAndSkipsNonSQL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  connection.ConnectionConfig
		wantSQL string
		wantOK  bool
	}{
		{name: "mysql", config: connection.ConnectionConfig{Type: "mysql"}, wantSQL: "SELECT VERSION() AS version", wantOK: true},
		{name: "kingbase", config: connection.ConnectionConfig{Type: "KingBase"}, wantSQL: "SELECT VERSION() AS version", wantOK: true},
		{name: "sqlserver", config: connection.ConnectionConfig{Type: "sqlserver"}, wantSQL: "SELECT @@VERSION AS version", wantOK: true},
		{name: "oracle", config: connection.ConnectionConfig{Type: "oracle"}, wantSQL: "SELECT banner AS version FROM v$version WHERE ROWNUM = 1", wantOK: true},
		{name: "dameng", config: connection.ConnectionConfig{Type: "dameng"}, wantSQL: "SELECT banner AS version FROM v$version WHERE ROWNUM = 1", wantOK: true},
		{name: "sqlite", config: connection.ConnectionConfig{Type: "sqlite"}, wantSQL: "SELECT sqlite_version() AS version", wantOK: true},
		{name: "tdengine", config: connection.ConnectionConfig{Type: "tdengine"}, wantSQL: "SELECT SERVER_VERSION() AS version", wantOK: true},
		{name: "custom mysql driver", config: connection.ConnectionConfig{Type: "custom", Driver: "mysql"}, wantSQL: "SELECT VERSION() AS version", wantOK: true},
		{name: "mongodb", config: connection.ConnectionConfig{Type: "mongodb"}, wantOK: false},
		{name: "redis", config: connection.ConnectionConfig{Type: "redis"}, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSQL, ok := ServerVersionQuery(tt.config)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if gotSQL != tt.wantSQL {
				t.Fatalf("sql = %q, want %q", gotSQL, tt.wantSQL)
			}
		})
	}
}

func TestSanitizeServerVersionDropsSecretsAndCollapsesWhitespace(t *testing.T) {
	t.Parallel()

	if got := SanitizeServerVersion("  5.7.44-log \n"); got != "5.7.44-log" {
		t.Fatalf("trimmed version = %q", got)
	}
	if got := SanitizeServerVersion("mysql://user:secret@localhost"); got != "" {
		t.Fatalf("secret banner should be empty, got %q", got)
	}
}

func TestFirstQueryRowValuePrefersNamedVersionColumn(t *testing.T) {
	t.Parallel()

	got := FirstQueryRowValue([]map[string]interface{}{{
		"extra":   "ignore-me",
		"version": "5.7.44-log",
	}})
	if got != "5.7.44-log" {
		t.Fatalf("version = %q", got)
	}
}
