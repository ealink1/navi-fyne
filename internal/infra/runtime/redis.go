package runtime

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/upstream/connection"
	"github.com/ealink1/super-link/internal/upstream/proxy"
	"github.com/ealink1/super-link/internal/upstream/ssh"
	"github.com/ealink1/super-link/internal/upstream/tlsconfig"
	"github.com/google/shlex"
	"github.com/redis/go-redis/v9"
)

type redisClient struct {
	client redis.UniversalClient
	cfg    connection.ConnectionConfig
}

func openRedis(ctx context.Context, cfg connection.ConnectionConfig) (Client, error) {
	options := &redis.UniversalOptions{Addrs: append([]string{net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))}, cfg.Hosts...), Username: cfg.User, Password: cfg.Password, DB: cfg.RedisDB, MasterName: cfg.RedisSentinelMaster, SentinelUsername: cfg.RedisSentinelUser, SentinelPassword: cfg.RedisSentinelPassword, MaxRedirects: 3, MaxRetries: -1, ContextTimeoutEnabled: true}
	if cfg.URI != "" {
		parsed, err := redis.ParseURL(cfg.URI)
		if err != nil {
			return nil, errors.New("invalid Redis URI")
		}
		options.Addrs = []string{parsed.Addr}
		options.Username = parsed.Username
		options.Password = parsed.Password
		options.DB = parsed.DB
		options.TLSConfig = parsed.TLSConfig
	}
	options.DialTimeout = time.Duration(max(cfg.Timeout, 1)) * time.Second
	options.ReadTimeout = 15 * time.Second
	options.WriteTimeout = 15 * time.Second
	if cfg.UseSSL {
		tlsOptions, err := tlsconfig.BuildClientConfig(tlsconfig.ClientConfigOptions{Enabled: true, InsecureSkipVerify: cfg.SSLMode == "skip-verify", CAPath: cfg.SSLCAPath, CertPath: cfg.SSLCertPath, KeyPath: cfg.SSLKeyPath})
		if err != nil {
			return nil, err
		}
		tlsOptions.ServerName = cfg.Host
		options.TLSConfig = tlsOptions
	}
	if options.TLSConfig == nil && cfg.UseSSL {
		options.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: cfg.Host}
	}
	if cfg.UseSSH || cfg.UseProxy {
		options.Dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
			var conn net.Conn
			var err error
			if cfg.UseSSH {
				conn, err = ssh.DialContextThroughSSH(ctx, cfg.SSH, network, addr)
			} else {
				conn, err = proxy.DialContext(ctx, cfg.Proxy, network, addr)
			}
			if err != nil {
				return nil, err
			}
			if options.TLSConfig != nil {
				secure := tls.Client(conn, options.TLSConfig.Clone())
				if err = secure.HandshakeContext(ctx); err != nil {
					_ = conn.Close()
					return nil, err
				}
				return secure, nil
			}
			return conn, nil
		}
	}
	client := redis.NewUniversalClient(options)
	connectCtx, cancel := connectionContext(ctx, cfg)
	defer cancel()
	if err := client.Ping(connectCtx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return &redisClient{client: client, cfg: cfg}, nil
}
func (c *redisClient) Close() error { return c.client.Close() }
func (c *redisClient) Scopes(ctx context.Context) ([]string, error) {
	return []string{strconv.Itoa(c.cfg.RedisDB)}, ctx.Err()
}
func (c *redisClient) Objects(ctx context.Context, pattern string) ([]domain.Object, error) {
	if pattern == "" {
		pattern = "*"
	}
	objects := []domain.Object{}
	cursor := uint64(0)
	for scans := 0; scans < 20 && len(objects) < 1000; scans++ {
		keys, next, err := c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			if len(objects) == 1000 {
				break
			}
			objects = append(objects, domain.Object{Name: key, Kind: "key", Scope: pattern})
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return objects, nil
}
func (c *redisClient) Schema(ctx context.Context, scope, key string) (string, error) {
	kind, err := c.client.Type(ctx, key).Result()
	if err != nil {
		return "", err
	}
	ttl, err := c.client.TTL(ctx, key).Result()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Key: %s\nType: %s\nTTL: %s", key, kind, ttl), nil
}
func (c *redisClient) Execute(ctx context.Context, e domain.Execution) ([]domain.Result, error) {
	args, err := shlex.Split(e.Text)
	if err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, errors.New("empty Redis command")
	}
	command := strings.ToUpper(args[0])
	if command == "KEYS" {
		return nil, errors.New("use SCAN instead of blocking KEYS")
	}
	values := make([]any, len(args))
	for i, arg := range args {
		values[i] = arg
	}
	raw, err := c.client.Do(ctx, values...).Result()
	if errors.Is(err, redis.Nil) {
		raw = nil
		err = nil
	}
	if err != nil {
		return nil, err
	}
	return JSONResult(map[string]any{"command": command, "result": raw})
}
