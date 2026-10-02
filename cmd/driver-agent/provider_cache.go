//go:build gonavi_cache_driver

package main

import "github.com/ealink1/navi-fyne/internal/upstream/db"

func init() {
	agentDriverType = "cache"
	agentDatabaseFactory = func() db.Database {
		return &db.CacheDB{}
	}
}
