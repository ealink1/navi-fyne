//go:build gonavi_sqlserver_driver

package main

import "github.com/ealink1/super-link/internal/upstream/db"

func init() {
	agentDriverType = "sqlserver"
	agentDatabaseFactory = func() db.Database {
		return &db.SqlServerDB{}
	}
}
