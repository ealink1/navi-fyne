//go:build gonavi_highgo_driver

package main

import "github.com/ealink1/navi-fyne/internal/upstream/db"

func init() {
	agentDriverType = "highgo"
	agentDatabaseFactory = func() db.Database {
		return &db.HighGoDB{}
	}
}
