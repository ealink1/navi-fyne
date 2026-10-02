//go:build gonavi_elasticsearch_driver

package main

import "github.com/ealink1/navi-fyne/internal/upstream/db"

func init() {
	agentDriverType = "elasticsearch"
	agentDatabaseFactory = func() db.Database {
		return &db.ElasticsearchDB{}
	}
}
