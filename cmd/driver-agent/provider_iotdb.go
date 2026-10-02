//go:build gonavi_iotdb_driver

package main

import "github.com/ealink1/navi-fyne/internal/upstream/db"

func init() {
	agentDriverType = "iotdb"
	agentDatabaseFactory = func() db.Database {
		return &db.IoTDBDB{}
	}
}
