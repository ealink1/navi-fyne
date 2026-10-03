package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/upstream/connection"
	"github.com/ealink1/super-link/internal/upstream/proxy"
)

// routedClient owns every ephemeral listener created for this connection.
type routedClient struct {
	Client
	routes []*routeForwarder
}

func (c *routedClient) Close() error {
	err := c.Client.Close()
	for _, route := range c.routes {
		err = errors.Join(err, route.Close())
	}
	return err
}

func routeConfig(cfg connection.ConnectionConfig, d domain.Descriptor) (connection.ConnectionConfig, []*routeForwarder, error) {
	if (d.Key == "sqlite" || d.Key == "duckdb" || d.Key == "custom") && (cfg.UseSSH || cfg.UseProxy || cfg.UseHTTPTunnel) {
		return cfg, nil, errors.New("本地文件和自定义 DSN 不支持在此处配置 SSH / 代理")
	}
	if d.Key == "pulsar" && (cfg.UseSSH || cfg.UseProxy || cfg.UseHTTPTunnel) {
		return cfg, nil, errors.New("Pulsar 发现的 broker 地址需要单独路由，当前不支持 SSH / 代理")
	}
	if cfg.UseHTTPTunnel {
		if cfg.UseProxy {
			return cfg, nil, errors.New("HTTP 隧道与网络代理不能同时启用")
		}
		parsed, _ := url.Parse(strings.TrimSpace(cfg.HTTPTunnel.Host))
		if parsed != nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
			if (d.Key != "mysql" && d.Key != "goldendb") || cfg.UseSSH {
				return cfg, nil, errors.New("脚本 HTTP 隧道仅支持 MySQL / GoldenDB，且不能与 SSH 同时启用")
			}
			return cfg, nil, nil
		}
		port := cfg.HTTPTunnel.Port
		if port == 0 {
			port = 8080
		}
		cfg.UseProxy = true
		cfg.Proxy = connection.ProxyConfig{Type: "http", Host: cfg.HTTPTunnel.Host, Port: port, User: cfg.HTTPTunnel.User, Password: cfg.HTTPTunnel.Password}
		cfg.UseHTTPTunnel = false
		cfg.HTTPTunnel = connection.HTTPTunnelConfig{}
	}
	if !cfg.UseProxy {
		return cfg, nil, nil
	}
	normalized, err := proxy.NormalizeConfig(cfg.Proxy)
	if err != nil {
		return cfg, nil, err
	}
	cfg.Proxy = normalized
	if cfg.UseSSH {
		port := cfg.SSH.Port
		if port == 0 {
			port = 22
		}
		if strings.TrimSpace(cfg.SSH.Host) == "" || port < 1 || port > 65535 {
			return cfg, nil, errors.New("请填写有效的 SSH 网关主机和端口")
		}
		forwarder, err := newRouteForwarder(cfg.Proxy, net.JoinHostPort(cfg.SSH.Host, strconv.Itoa(port)))
		if err != nil {
			return cfg, nil, err
		}
		cfg.SSH = cfg.SSH.WithHostKeyIdentity(cfg.SSH.Host, port)
		cfg.SSH.Host, cfg.SSH.Port = forwarder.address()
		cfg.UseProxy = false
		cfg.Proxy = connection.ProxyConfig{}
		return cfg, []*routeForwarder{forwarder}, nil
	}
	if d.Family != domain.SQL {
		// Native protocol transports dial each discovered endpoint through the proxy.
		return cfg, nil, nil
	}
	if cfg.URI != "" || cfg.DSN != "" || strings.Contains(cfg.Host, "://") || len(cfg.Hosts) != 0 || cfg.Topology != "" && cfg.Topology != "single" {
		return cfg, nil, errors.New("SQL 代理当前仅支持单主机连接；请使用主机 / 端口字段，清空 URI、DSN 和多主机配置")
	}
	if sqlProxyTLS(cfg) {
		return cfg, nil, errors.New("SQL 代理与 TLS 的服务器名称校验尚未适配；请使用直接 TLS 连接或 SSH 隧道")
	}
	params, err := url.ParseQuery(strings.TrimPrefix(strings.TrimSpace(cfg.ConnectionParams), "?"))
	if err != nil {
		return cfg, nil, errors.New("连接参数不是有效的 URL 查询参数")
	}
	for key := range params {
		switch strings.ToLower(key) {
		case "host", "hostaddr", "port", "server", "address", "addr", "endpoint", "net", "socket":
			return cfg, nil, fmt.Errorf("SQL 代理不允许连接参数覆盖目标地址：%s", key)
		}
	}
	if strings.TrimSpace(cfg.Host) == "" || cfg.Port < 1 || cfg.Port > 65535 {
		return cfg, nil, errors.New("请填写有效的目标主机和端口")
	}
	forwarder, err := newRouteForwarder(cfg.Proxy, net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)))
	if err != nil {
		return cfg, nil, err
	}
	cfg.Host, cfg.Port = forwarder.address()
	cfg.UseProxy = false
	cfg.Proxy = connection.ProxyConfig{}
	return cfg, []*routeForwarder{forwarder}, nil
}

func sqlProxyTLS(cfg connection.ConnectionConfig) bool {
	if cfg.UseSSL && cfg.SSLMode != "disable" {
		return true
	}
	params, _ := url.ParseQuery(strings.TrimPrefix(strings.TrimSpace(cfg.ConnectionParams), "?"))
	for key, values := range params {
		switch strings.ToLower(key) {
		case "tls", "sslmode", "usessl", "ssl", "encrypt", "secure", "encryption":
			for _, value := range values {
				switch strings.ToLower(value) {
				case "", "disable", "false", "0", "off":
				default:
					return true
				}
			}
		}
	}
	return false
}

type routeForwarder struct {
	listener net.Listener
	cancel   context.CancelFunc
	mu       sync.Mutex
	closed   bool
	conns    map[net.Conn]struct{}
	wg       sync.WaitGroup
}

func newRouteForwarder(cfg connection.ProxyConfig, target string) (*routeForwarder, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	f := &routeForwarder{listener: listener, cancel: cancel, conns: make(map[net.Conn]struct{})}
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			if !f.track(conn) {
				return
			}
			f.wg.Add(1)
			go func() {
				defer f.wg.Done()
				defer f.release(conn)
				dialCtx, stop := context.WithTimeout(ctx, 8*time.Second)
				remote, err := proxy.DialContext(dialCtx, cfg, "tcp", target)
				stop()
				if err != nil || !f.track(remote) {
					return
				}
				defer f.release(remote)
				done := make(chan struct{})
				go func() { _, _ = io.Copy(remote, conn); _ = remote.Close(); _ = conn.Close(); close(done) }()
				_, _ = io.Copy(conn, remote)
				_ = conn.Close()
				_ = remote.Close()
				<-done
			}()
		}
	}()
	return f, nil
}

func (f *routeForwarder) address() (string, int) {
	address := f.listener.Addr().(*net.TCPAddr)
	return address.IP.String(), address.Port
}
func (f *routeForwarder) track(conn net.Conn) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		_ = conn.Close()
		return false
	}
	f.conns[conn] = struct{}{}
	return true
}
func (f *routeForwarder) release(conn net.Conn) {
	_ = conn.Close()
	f.mu.Lock()
	delete(f.conns, conn)
	f.mu.Unlock()
}
func (f *routeForwarder) Close() error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil
	}
	f.closed = true
	f.cancel()
	err := f.listener.Close()
	for conn := range f.conns {
		_ = conn.Close()
	}
	f.mu.Unlock()
	f.wg.Wait()
	return err
}
