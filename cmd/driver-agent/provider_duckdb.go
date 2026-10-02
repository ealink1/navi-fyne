//go:build gonavi_duckdb_driver

package main

import "github.com/ealink1/navi-fyne/internal/upstream/db"

func init() {
	agentDriverType = "duckdb"
	agentDatabaseFactory = func() db.Database {
		return &db.DuckDB{}
	}
}
