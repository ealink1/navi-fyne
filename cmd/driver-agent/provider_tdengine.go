//go:build gonavi_tdengine_driver

package main

import "github.com/ealink1/navi-fyne/internal/upstream/db"

func init() {
	agentDriverType = "tdengine"
	agentDatabaseFactory = func() db.Database {
		return &db.TDengineDB{}
	}
}
