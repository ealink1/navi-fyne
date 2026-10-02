//go:build gonavi_gaussdb_driver

package main

import "github.com/ealink1/navi-fyne/internal/upstream/db"

func init() {
	agentDriverType = "gaussdb"
	agentDatabaseFactory = func() db.Database {
		return &db.GaussDB{}
	}
}
