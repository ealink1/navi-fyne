package main

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ealink1/navi-fyne/internal/bootstrap"
	"github.com/ealink1/navi-fyne/internal/domain"
	"github.com/ealink1/navi-fyne/internal/upstream/connection"
)

func seedFixture(services *bootstrap.Services) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	path := filepath.Join(services.Root, "fixture.sqlite")
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer fixture.Close()
	_, err = fixture.ExecContext(ctx, `CREATE TABLE samples (id INTEGER PRIMARY KEY, name TEXT);
		WITH RECURSIVE seq(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM seq WHERE i<120)
		INSERT INTO samples SELECT i, '内存测试记录 ' || i FROM seq;`)
	if err != nil {
		return fmt.Errorf("create isolated fixture: %w", err)
	}
	profile, err := services.Profiles.Save(ctx, domain.Profile{Name: "内存测试 SQLite", Environment: "test", ReadOnly: true,
		Config: connection.ConnectionConfig{Type: "sqlite", Database: path}})
	if err != nil {
		return err
	}
	for i := range 2 {
		err := services.Store.SaveDraft(ctx, domain.Draft{ID: fmt.Sprintf("memory-draft-%d", i), ProfileID: profile.ID,
			Scope: path, Title: fmt.Sprintf("内存测试查询 %d", i+1), Text: "SELECT id, name FROM samples LIMIT 100;\n-- 中文查询、结果、字段与连接", UpdatedAt: time.Now()})
		if err != nil {
			return err
		}
	}
	return nil
}
