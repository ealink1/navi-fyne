//go:build gonavi_starrocks_driver

package main

import "github.com/ealink1/navi-fyne/internal/upstream/db"

func init() {
	agentDriverType = "starrocks"
	agentDatabaseFactory = func() db.Database {
		return &db.StarRocksDB{}
	}
}
