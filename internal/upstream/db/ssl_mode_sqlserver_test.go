package db

import (
	"testing"

	"github.com/ealink1/navi-fyne/internal/upstream/connection"
)

func TestResolveSQLServerTLSSettingsUsesRemoteIdentityAfterSSHForwarding(t *testing.T) {
	t.Parallel()

	forwarded := connection.ConnectionConfig{
		Host: "127.0.0.1",
		Port: 1433,
	}
	encrypt, trust := resolveSQLServerTLSSettings(forwarded)
	if encrypt != "disable" || trust != "true" {
		t.Fatalf("local forward without SSL: encrypt=%q trust=%q, want disable/true", encrypt, trust)
	}

	remoteIdentity := forwarded
	remoteIdentity.Host = "myserver.database.windows.net"
	encrypt, trust = resolveSQLServerTLSSettings(remoteIdentity)
	if encrypt != "true" || trust != "true" {
		t.Fatalf("azure remote identity: encrypt=%q trust=%q, want true/true", encrypt, trust)
	}
}

func TestSQLServerHostNameInCertificateUsesRemoteIdentityAfterSSHForwarding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host string
		want string
	}{
		{name: "azure", host: "myserver.database.windows.net", want: "*.database.windows.net"},
		{name: "on premises host", host: "sql.internal.example.com", want: "sql.internal.example.com"},
		{name: "host with port", host: "sql.internal.example.com:1433", want: "sql.internal.example.com"},
		{name: "ipv6", host: "[2001:db8::1]:1433", want: "2001:db8::1"},
		{name: "bare ipv6", host: "2001:db8::1", want: "2001:db8::1"},
		{name: "bracketed ipv6", host: "[2001:db8::1]", want: "2001:db8::1"},
		{name: "azure mixed case and whitespace", host: "  MyServer.Database.Windows.NET  ", want: "*.database.windows.net"},
		{name: "azure trailing dot", host: "myserver.database.windows.net.", want: "*.database.windows.net"},
		{name: "empty", host: "", want: ""},
		{name: "local forward", host: "127.0.0.1", want: "127.0.0.1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := sqlServerHostNameInCertificate(test.host); got != test.want {
				t.Fatalf("sqlServerHostNameInCertificate(%q)=%q, want %q", test.host, got, test.want)
			}
		})
	}
}

func TestLooksLikeAzureSQLHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		host string
		want bool
	}{
		{host: "myserver.database.windows.net", want: true},
		{host: "myserver.database.chinacloudapi.cn", want: true},
		{host: "myserver.sql.azuresynapse.net", want: true},
		{host: "sql.example.com", want: false},
		{host: "127.0.0.1", want: false},
		{host: "", want: false},
	}
	for _, test := range tests {
		if got := looksLikeAzureSQLHost(test.host); got != test.want {
			t.Fatalf("looksLikeAzureSQLHost(%q)=%v, want %v", test.host, got, test.want)
		}
	}
}

func TestResolveSQLServerTLSSettingsForcesEncryptForAzureHosts(t *testing.T) {
	t.Parallel()

	encrypt, trust := resolveSQLServerTLSSettings(connection.ConnectionConfig{
		Host: "myserver.database.windows.net",
	})
	if encrypt != "true" || trust != "true" {
		t.Fatalf("azure without SSL: encrypt=%q trust=%q, want true/true", encrypt, trust)
	}

	encrypt, trust = resolveSQLServerTLSSettings(connection.ConnectionConfig{
		Host:    "myserver.database.windows.net",
		UseSSL:  true,
		SSLMode: "required",
	})
	if encrypt != "true" || trust != "false" {
		t.Fatalf("azure required SSL: encrypt=%q trust=%q, want true/false", encrypt, trust)
	}

	encrypt, trust = resolveSQLServerTLSSettings(connection.ConnectionConfig{
		Host: "127.0.0.1",
	})
	if encrypt != "disable" {
		t.Fatalf("on-prem without SSL: encrypt=%q, want disable", encrypt)
	}
}

func TestAzureSQLHostNameInCertificate(t *testing.T) {
	t.Parallel()

	if got := azureSQLHostNameInCertificate("myserver.database.windows.net"); got != "*.database.windows.net" {
		t.Fatalf("windows.net cert host = %q", got)
	}
	if got := azureSQLHostNameInCertificate("sql.example.com"); got != "" {
		t.Fatalf("non-azure cert host = %q, want empty", got)
	}
}
