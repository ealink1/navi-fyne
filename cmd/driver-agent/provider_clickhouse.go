//go:build gonavi_clickhouse_driver

package main

import "github.com/ealink1/navi-fyne/internal/upstream/db"

func init() {
	agentDriverType = "clickhouse"
	agentDatabaseFactory = func() db.Database {
		return &db.ClickHouseDB{}
	}
}
