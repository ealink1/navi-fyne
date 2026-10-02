package domain

import "strings"

// SQLDialect distinguishes OceanBase's Oracle and MySQL compatibility modes.
func (p Profile) SQLDialect() string {
	if p.Config.Type == "custom" {
		switch p.Config.Driver {
		case "pgx", "pq", "postgres":
			return "postgres"
		case "mysql":
			return "mysql"
		case "sqlserver":
			return "sqlserver"
		case "sqlite", "sqlite3":
			return "sqlite"
		case "godror":
			return "oracle"
		}
	}
	if p.Config.Type == "oceanbase" && strings.EqualFold(p.Config.OceanBaseProtocol, "oracle") {
		return "oracle"
	}
	return p.Config.Type
}
