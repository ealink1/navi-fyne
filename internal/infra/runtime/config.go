package runtime

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/ealink1/super-link/internal/domain"
)

// ConfigForScope preserves transport identity while selecting a database/schema.
// In particular an Oracle owner must never replace a service name or SID.
func ConfigForScope(p domain.Profile, scope string) (domain.Profile, error) {
	d, err := domain.Resolve(p.Config.Type)
	if err != nil {
		return p, err
	}
	p.Scope = scope
	if d.Key == "postgres" || d.Key == "oracle" {
		if text := strings.TrimSpace(p.Config.URI); text != "" {
			u, parseErr := url.Parse(text)
			if parseErr != nil || u.Hostname() == "" {
				return p, errors.New("invalid connection URI")
			}
			if d.Key == "postgres" && u.Scheme != "postgres" && u.Scheme != "postgresql" || d.Key == "oracle" && u.Scheme != "oracle" {
				return p, errors.New("URI scheme does not match the connection type")
			}
			p.Config.Host, p.Config.Port = u.Hostname(), d.Port
			if u.Port() != "" {
				port, parseErr := strconv.Atoi(u.Port())
				if parseErr != nil || port < 1 || port > 65535 {
					return p, errors.New("invalid URI port")
				}
				p.Config.Port = port
			}
			if u.User != nil {
				p.Config.User = u.User.Username()
				p.Config.Password, _ = u.User.Password()
			}
			if u.Path != "" && u.Path != "/" {
				p.Config.Database = strings.TrimPrefix(u.Path, "/")
			}
		}
	}
	if scope == "" {
		return p, nil
	}
	if d.Key == "oracle" || d.Key == "oceanbase" && strings.EqualFold(p.Config.OceanBaseProtocol, "oracle") {
		return p, nil // Applied on the pinned session, including agent connections.
	}
	if d.Key == "custom" {
		if scope != p.Config.Database {
			return p, errors.New("custom DSN database switching requires editing the DSN")
		}
		return p, nil
	}
	if d.Family == domain.SQL && d.Key != "sqlite" && d.Key != "duckdb" || d.Family == domain.Document {
		p.Config.Database = scope
	}
	return p, nil
}
