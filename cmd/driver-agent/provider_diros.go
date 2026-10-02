//go:build gonavi_diros_driver

package main

import "github.com/ealink1/navi-fyne/internal/upstream/db"

func init() {
	agentDriverType = "diros"
	agentDatabaseFactory = func() db.Database {
		return &db.DirosDB{}
	}
}
