//go:build gonavi_mysql_driver

package main

import "github.com/ealink1/navi-fyne/internal/upstream/db"

func init() {
	agentDriverType = "mysql"
	agentDatabaseFactory = func() db.Database {
		return &db.MySQLDB{}
	}
}
