//go:build gonavi_oceanbase_driver

package main

import "github.com/ealink1/navi-fyne/internal/upstream/db"

func init() {
	agentDriverType = "oceanbase"
	agentDatabaseFactory = func() db.Database {
		return &db.OceanBaseDB{}
	}
}
