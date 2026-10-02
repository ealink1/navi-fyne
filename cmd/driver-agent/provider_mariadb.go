//go:build gonavi_mariadb_driver

package main

import "github.com/ealink1/navi-fyne/internal/upstream/db"

func init() {
	agentDriverType = "mariadb"
	agentDatabaseFactory = func() db.Database {
		return &db.MariaDB{}
	}
}
