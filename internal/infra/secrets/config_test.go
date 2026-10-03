package secrets

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ealink1/super-link/internal/upstream/connection"
)

func TestSplitJoinProtectsNestedCredentials(t *testing.T) {
	cfg := connection.ConnectionConfig{Type: "mysql", Host: "https://user:host-secret@example.test", Hosts: []string{"redis://user:hosts-secret@db.test", "plain.test:123"}, Password: "main-secret", SavePassword: true, DSN: "dsn-secret", URI: "uri-secret", ConnectionParams: "token=params-secret", SSH: connection.SSHConfig{Password: "ssh-secret"}, Proxy: connection.ProxyConfig{Password: "proxy-secret"}, RedisSentinelPassword: "sentinel-secret"}
	public, bundle, err := Split(cfg)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(public)
	for _, secret := range []string{"main-secret", "host-secret", "hosts-secret", "dsn-secret", "uri-secret", "params-secret", "ssh-secret", "proxy-secret", "sentinel-secret"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("secret leaked: %s", secret)
		}
	}
	if !public.SavePassword {
		t.Fatal("save preference must remain public metadata")
	}
	joined, err := Join(public, bundle)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := json.Marshal(cfg)
	actual, _ := json.Marshal(joined)
	if string(original) != string(actual) {
		t.Fatal("secret round trip changed connection configuration")
	}
}
func TestSessionVaultCopiesDataAndDropsDeletedCredentials(t *testing.T) {
	v := New()
	original := []byte(`{"password":"secret"}`)
	ref, err := v.Put(original, false)
	if err != nil {
		t.Fatal(err)
	}
	clear(original)
	data, err := v.Get(ref)
	if err != nil || !strings.Contains(string(data), "secret") {
		t.Fatal("vault aliased input")
	}
	clear(data)
	data, err = v.Get(ref)
	if err != nil || !strings.Contains(string(data), "secret") {
		t.Fatal("vault aliased output")
	}
	if err = v.Delete(ref); err != nil {
		t.Fatal(err)
	}
	if _, err = v.Get(ref); err != ErrMissing {
		t.Fatal("deleted credentials remained accessible")
	}
}
