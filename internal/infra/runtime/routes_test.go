package runtime

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/upstream/connection"
)

func TestProxyRouteForwardsAndClosesActiveConnections(t *testing.T) {
	targets := make(chan string, 1)
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targets <- r.Host
		conn, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = buffer.Flush()
		_, _ = io.Copy(conn, conn)
	}))
	defer proxyServer.Close()
	host, port, _ := net.SplitHostPort(proxyServer.Listener.Addr().String())
	proxyPort, _ := strconv.Atoi(port)
	cfg := connection.ConnectionConfig{Host: "private.example", Port: 5432, UseProxy: true, Proxy: connection.ProxyConfig{Type: "http", Host: host, Port: proxyPort}}
	descriptor, _ := domain.Resolve("postgres")
	routed, routes, err := routeConfig(cfg, descriptor)
	if err != nil || len(routes) != 1 || routed.UseProxy {
		t.Fatal("route not created", err)
	}
	defer routes[0].Close()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(routed.Host, strconv.Itoa(routed.Port)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	_, _ = conn.Write([]byte("hello"))
	data := make([]byte, 5)
	if _, err = io.ReadFull(conn, data); err != nil || string(data) != "hello" {
		t.Fatal("proxy data did not round trip", err)
	}
	if target := <-targets; target != "private.example:5432" {
		t.Fatal("proxy used wrong logical target", target)
	}
	if err = routes[0].Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Read(data); err == nil {
		t.Fatal("active proxy connection survived close")
	}
	if extra, err := net.DialTimeout("tcp", net.JoinHostPort(routed.Host, strconv.Itoa(routed.Port)), time.Second); err == nil {
		extra.Close()
		t.Fatal("listener survived close")
	}
}

func TestProxySSHRetainsGatewayHostKeyIdentity(t *testing.T) {
	descriptor, _ := domain.Resolve("redis")
	cfg := connection.ConnectionConfig{UseSSH: true, UseProxy: true, SSH: connection.SSHConfig{Host: "bastion.example", Port: 2222}, Proxy: connection.ProxyConfig{Type: "socks5", Host: "127.0.0.1", Port: 1080}}
	routed, routes, err := routeConfig(cfg, descriptor)
	if err != nil {
		t.Fatal(err)
	}
	defer routes[0].Close()
	identityHost, identityPort := routed.SSH.HostKeyIdentity()
	if routed.UseProxy || !routed.UseSSH || routed.SSH.Host != "127.0.0.1" || identityHost != "bastion.example" || identityPort != 2222 {
		t.Fatal("SSH proxy lost logical gateway identity")
	}
	snapshot := routed.SSH.RuntimeSnapshot()
	restored := (connection.SSHConfig{Host: routed.SSH.Host, Port: routed.SSH.Port}).WithRuntimeSnapshot(snapshot)
	if restoredHost, restoredPort := restored.HostKeyIdentity(); restoredHost != identityHost || restoredPort != identityPort {
		t.Fatal("agent IPC snapshot lost host key identity")
	}
}

func TestProxyRoutesRejectUnsupportedOrBypassedSettings(t *testing.T) {
	base := connection.ConnectionConfig{Host: "db.example", Port: 5432, UseProxy: true, Proxy: connection.ProxyConfig{Type: "socks5", Host: "proxy.example", Port: 1080}}
	for _, test := range []struct {
		name, kind string
		modify     func(*connection.ConnectionConfig)
	}{
		{"file", "sqlite", func(c *connection.ConnectionConfig) {}},
		{"custom", "custom", func(c *connection.ConnectionConfig) {}},
		{"discovery", "pulsar", func(c *connection.ConnectionConfig) {}},
		{"URI", "postgres", func(c *connection.ConnectionConfig) { c.URI = "postgres://db.example/other" }},
		{"TLS", "postgres", func(c *connection.ConnectionConfig) { c.UseSSL = true }},
		{"TLS params", "mysql", func(c *connection.ConnectionConfig) { c.ConnectionParams = "tls=true" }},
		{"address params", "postgres", func(c *connection.ConnectionConfig) { c.ConnectionParams = "host=direct.example" }},
		{"failover", "postgres", func(c *connection.ConnectionConfig) { c.Hosts = []string{"direct.example:5432"} }},
		{"ambiguous", "postgres", func(c *connection.ConnectionConfig) { c.UseHTTPTunnel = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := base
			test.modify(&cfg)
			descriptor, _ := domain.Resolve(test.kind)
			_, routes, err := routeConfig(cfg, descriptor)
			for _, route := range routes {
				_ = route.Close()
			}
			if err == nil {
				t.Fatal("unsupported or bypassed proxy setting accepted")
			}
		})
	}
}
