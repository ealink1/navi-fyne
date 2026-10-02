//go:build gonavi_sphinx_driver

package main

import "github.com/ealink1/navi-fyne/internal/upstream/db"

func init() {
	agentDriverType = "sphinx"
	agentDatabaseFactory = func() db.Database {
		return &db.SphinxDB{}
	}
}
