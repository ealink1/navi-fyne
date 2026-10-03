package ui

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ealink1/super-link/internal/infra/secrets"
	"github.com/ealink1/super-link/internal/upstream/connection"
)

func TestURIDoesNotRetainDefaultOrStaleBasicOverrides(t *testing.T) {
	cfg := connection.ConnectionConfig{Type: "mysql", Host: "127.0.0.1", Port: 3306, User: "old", Password: "old", Database: "old", Hosts: []string{"old:3306"}, URI: "mysql://test:secret@remote.example:3307/actual", UseSSH: true}
	if err := preferConnectionURI(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "" || cfg.Port != 0 || cfg.User != "" || cfg.Password != "" || cfg.Database != "" || len(cfg.Hosts) != 0 || cfg.URI == "" || !cfg.UseSSH {
		t.Fatal("stale form fields would override the URI")
	}
	cfg.Type = "sqlite"
	if err := preferConnectionURI(&cfg); err == nil {
		t.Fatal("accepted a URI that the file adapter would ignore")
	}
}

func TestUnchangedAdvancedEditorPreservesCredentials(t *testing.T) {
	cfg := connection.ConnectionConfig{Type: "mysql", Password: "main-secret", URI: "uri-secret", DSN: "dsn-secret", ConnectionParams: "token=params-secret", RedisSentinelPassword: "sentinel-secret", MySQLReplicaPassword: "replica-secret", MongoReplicaPassword: "mongo-secret", Host: "https://user:secret@db.test", Hosts: []string{"redis://user:secret@db.test"}, SSH: connection.SSHConfig{Password: "ssh-secret"}, Proxy: connection.ProxyConfig{Password: "proxy-secret"}, HTTPTunnel: connection.HTTPTunnelConfig{Host: "https://user:secret@tunnel.test", Password: "tunnel-secret"}}
	raw, err := secrets.PublicJSON(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") {
		t.Fatal("advanced editor exposed a credential")
	}
	var public map[string]any
	if err = json.Unmarshal(raw, &public); err != nil {
		t.Fatal(err)
	}
	if _, exists := public["password"]; exists {
		t.Fatal("advanced editor reintroduced an empty credential field")
	}
	if _, exists := public["httpTunnel"].(map[string]any)["host"]; exists {
		t.Fatal("credential-bearing tunnel URL reached the advanced editor")
	}
	actual, err := overlayConfig(cfg, string(raw))
	if err != nil || !reflect.DeepEqual(actual, cfg) {
		t.Fatal("an unchanged advanced editor erased connection credentials", err)
	}
	actual, err = overlayConfig(cfg, `{"connectionParams":""}`)
	if err != nil || actual.ConnectionParams != "" {
		t.Fatal("an explicit credential clear was ignored", err)
	}
}

func TestAdvancedEditorRejectsUnknownConfiguration(t *testing.T) {
	for _, patch := range []string{`{"jvm":{"enabled":true}}`, `{"ssh":{"unknownOption":true}}`, `{"timeuot":5}`, `null`, `[]`, `{} {}`} {
		if _, err := overlayConfig(connection.ConnectionConfig{Type: "mysql"}, patch); err == nil {
			t.Fatalf("accepted unsupported configuration: %s", patch)
		}
	}
}
