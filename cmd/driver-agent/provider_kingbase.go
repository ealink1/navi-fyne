//go:build gonavi_kingbase_driver

package main

import "github.com/ealink1/navi-fyne/internal/upstream/db"

func init() {
	agentDriverType = "kingbase"
	agentDatabaseFactory = func() db.Database {
		return &db.KingbaseDB{}
	}
}
