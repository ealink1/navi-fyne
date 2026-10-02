//go:build gonavi_mongodb_driver_v1

package main

import "github.com/ealink1/navi-fyne/internal/upstream/db"

func init() {
	agentDriverType = "mongodb"
	agentDatabaseFactory = func() db.Database {
		return &db.MongoDBV1{}
	}
}
