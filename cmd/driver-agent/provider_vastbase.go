//go:build gonavi_vastbase_driver

package main

import "github.com/ealink1/navi-fyne/internal/upstream/db"

func init() {
	agentDriverType = "vastbase"
	agentDatabaseFactory = func() db.Database {
		return &db.VastbaseDB{}
	}
}
