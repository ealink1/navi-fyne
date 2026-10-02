//go:build gonavi_opengauss_driver

package main

import "github.com/ealink1/navi-fyne/internal/upstream/db"

func init() {
	agentDriverType = "opengauss"
	agentDatabaseFactory = func() db.Database {
		return &db.OpenGaussDB{}
	}
}
