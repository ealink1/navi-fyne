//go:build gonavi_trino_driver

package main

import "github.com/ealink1/navi-fyne/internal/upstream/db"

func init() {
	agentDriverType = "trino"
	agentDatabaseFactory = func() db.Database {
		return &db.TrinoDB{}
	}
}
