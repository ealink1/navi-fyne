//go:build gonavi_dameng_driver

package main

import "github.com/ealink1/navi-fyne/internal/upstream/db"

func init() {
	agentDriverType = "dameng"
	agentDatabaseFactory = func() db.Database {
		return &db.DamengDB{}
	}
}
