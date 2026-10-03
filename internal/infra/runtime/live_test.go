//go:build integration

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/infra/drivers"
	"github.com/ealink1/super-link/internal/upstream/connection"
	"github.com/ealink1/super-link/internal/upstream/db"
)

func installTestAgent(t *testing.T, kind string) {
	t.Helper()
	directory := os.Getenv("SUPERLINK_TEST_DRIVERS")
	if directory == "" {
		t.Fatal("SUPERLINK_TEST_DRIVERS must point to built agents")
	}
	raw, err := os.ReadFile(filepath.Join(directory, "bundle.json"))
	if err != nil {
		t.Fatal(err)
	}
	var bundle drivers.Bundle
	if err = json.Unmarshal(raw, &bundle); err != nil {
		t.Fatal(err)
	}
	manager := &drivers.Manager{Root: t.TempDir()}
	found := false
	for _, record := range bundle.Drivers {
		if record.Type != kind {
			continue
		}
		source := filepath.Join(directory, kind+"-driver-agent")
		if err = manager.Install(context.Background(), kind, source, record.SHA256, "selfcheck"); err != nil {
			t.Fatal(err)
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("agent %s not built", kind)
	}
	db.SetExternalDriverDownloadDirectory(manager.Root)
}
func TestLive(t *testing.T) {
	raw, err := os.ReadFile(os.Getenv("SUPERLINK_LIVE_CONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	var configs []connection.ConnectionConfig
	if err = json.Unmarshal(raw, &configs); err != nil {
		t.Fatal(err)
	}
	for _, cfg := range configs {
		t.Run(cfg.Type, func(t *testing.T) {
			descriptor, _ := domain.Resolve(cfg.Type)
			if descriptor.Agent {
				installTestAgent(t, cfg.Type)
			}
			var client Client
			var err error
			deadline := time.Now().Add(100 * time.Second)
			for time.Now().Before(deadline) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				client, err = Open(ctx, domain.Profile{Config: cfg})
				cancel()
				if err == nil {
					break
				}
				time.Sleep(time.Second)
			}
			if err != nil {
				t.Fatalf("service not ready: %v", err)
			}
			defer client.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			switch descriptor.Family {
			case domain.SQL:
				verifyLiveSQL(t, ctx, client, cfg)
			case domain.Cache:
				verifyLiveRedis(t, ctx, client)
			case domain.Message:
				verifyLiveMessage(t, ctx, client, cfg)
			case domain.Document:
				verifyLiveMongo(t, ctx, client)
			case domain.Vector:
				verifyLiveVector(t, ctx, client, cfg.Type)
			case domain.Search:
				verifyLiveSearch(t, ctx, client)
			case domain.Configuration:
				verifyLiveNacos(t, ctx, client)
			}
		})
	}
}
func executeLive(t *testing.T, ctx context.Context, client Client, text string, write bool) []domain.Result {
	t.Helper()
	result, err := client.Execute(ctx, domain.Execution{Text: text, Write: write})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func verifyLiveSQL(t *testing.T, ctx context.Context, client Client, cfg connection.ConnectionConfig) {
	executeLive(t, ctx, client, "CREATE TABLE superlink_fixture(id BIGINT PRIMARY KEY, name VARCHAR(100), nullable_value VARCHAR(10))", true)
	executeLive(t, ctx, client, "INSERT INTO superlink_fixture VALUES(9223372036854775807,'中文测试',NULL)", true)
	result := executeLive(t, ctx, client, "SELECT id, name, nullable_value FROM superlink_fixture", false)
	if len(result) != 1 || len(result[0].Rows) != 1 || fmt.Sprint(result[0].Rows[0][0]) != "9223372036854775807" || result[0].Rows[0][1] != "中文测试" || result[0].Rows[0][2] != nil {
		t.Fatalf("type fidelity: %#v", result)
	}
	duplicate := executeLive(t, ctx, client, "SELECT 1 AS same, 2 AS same", false)
	if len(duplicate[0].Columns) != 2 || duplicate[0].Columns[0].Name == duplicate[0].Columns[1].Name || fmt.Sprint(duplicate[0].Rows[0][0]) != "1" || fmt.Sprint(duplicate[0].Rows[0][1]) != "2" {
		t.Fatal("duplicate columns lost")
	}
	objects, err := client.Objects(ctx, cfg.Database)
	if err != nil || len(objects) == 0 {
		t.Fatalf("object browsing failed: %v", err)
	}
	ddl, err := client.Schema(ctx, cfg.Database, "superlink_fixture")
	if err != nil || ddl == "" {
		t.Fatalf("DDL browsing failed: %v", err)
	}
	executeLive(t, ctx, client, "DROP TABLE superlink_fixture", true)
}
func verifyLiveRedis(t *testing.T, ctx context.Context, client Client) {
	executeLive(t, ctx, client, `SET superlink:test "中文测试"`, true)
	result := executeLive(t, ctx, client, "GET superlink:test", false)
	if !strings.Contains(fmt.Sprint(result), "中文测试") {
		t.Fatal("Redis value missing")
	}
	objects, err := client.Objects(ctx, "superlink:*")
	if err != nil || len(objects) != 1 {
		t.Fatalf("Redis SCAN failed %v", err)
	}
	info, err := client.Schema(ctx, "", "superlink:test")
	if err != nil || !strings.Contains(info, "string") {
		t.Fatal("Redis type and TTL failed")
	}
	executeLive(t, ctx, client, "DEL superlink:test", true)
}
func verifyLiveMessage(t *testing.T, ctx context.Context, client Client, cfg connection.ConnectionConfig) {
	objects, err := client.Objects(ctx, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("discovered %d messaging objects", len(objects))
	if cfg.Type == "mqtt" {
		executeLive(t, ctx, client, `{"publish":"superlink/test","payload":"中文消息","qos":1,"retain":true}`, true)
		result := executeLive(t, ctx, client, `SELECT * FROM "superlink/test" LIMIT 1`, false)
		if len(result) == 0 || len(result[0].Rows) == 0 {
			t.Fatal("MQTT retained message not received")
		}
		executeLive(t, ctx, client, `{"publish":"superlink/test","payload":"","retain":true}`, true)
	}
	if cfg.Type == "rabbitmq" {
		address := fmt.Sprintf("http://127.0.0.1:%d/api/queues/%%2F/superlink_fixture", cfg.Port)
		request, _ := http.NewRequestWithContext(ctx, http.MethodPut, address, strings.NewReader(`{"durable":true,"auto_delete":false,"arguments":{}}`))
		request.SetBasicAuth(cfg.User, cfg.Password)
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 4096))
		response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode > 299 {
			t.Fatalf("fixture queue creation failed: %d %s", response.StatusCode, body)
		}
		executeLive(t, ctx, client, `{"publish":"superlink_fixture","payload":"中文队列"}`, true)
		for i := 0; i < 2; i++ {
			result := executeLive(t, ctx, client, `SELECT * FROM "superlink_fixture" LIMIT 1`, true)
			if !strings.Contains(fmt.Sprint(result), "中文队列") {
				t.Fatal("RabbitMQ publication or requeue preview failed")
			}
		}
	}
}
func verifyLiveMongo(t *testing.T, ctx context.Context, client Client) {
	executeLive(t, ctx, client, `{"insert":"superlink_fixture","documents":[{"id":1,"name":"中文文档"}]}`, true)
	result := executeLive(t, ctx, client, `{"find":"superlink_fixture","filter":{},"limit":10}`, false)
	if len(result) == 0 || len(result[0].Rows) != 1 {
		t.Fatal("MongoDB document query failed")
	}
}
func verifyLiveVector(t *testing.T, ctx context.Context, client Client, kind string) {
	command := `{"create_collection":"superlink_fixture","vectors":{"size":3,"distance":"Cosine"}}`
	if kind == "chroma" {
		command = `{"create_collection":"superlink_fixture"}`
	}
	executeLive(t, ctx, client, command, true)
	objects, err := client.Objects(ctx, "")
	if err != nil || len(objects) == 0 {
		t.Fatalf("collection browsing failed: %v", err)
	}
	executeLive(t, ctx, client, `{"list_collections":true}`, false)
	upsert := `{"upsert":"superlink_fixture","points":[{"id":1,"vector":[1,0,0],"payload":{"name":"中文向量"}}]}`
	query := `{"search":"superlink_fixture","vector":[1,0,0],"limit":1}`
	if kind == "chroma" {
		upsert = `{"upsert":"superlink_fixture","ids":["one"],"embeddings":[[1,0,0]],"documents":["中文向量"]}`
		query = `{"query":"superlink_fixture","query_embeddings":[[1,0,0]],"n_results":1}`
	}
	executeLive(t, ctx, client, upsert, true)
	result := executeLive(t, ctx, client, query, false)
	if !strings.Contains(fmt.Sprint(result), "中文向量") {
		t.Fatal("native vector search lost payload")
	}
}
func TestLocalFileAgents(t *testing.T) {
	for _, kind := range []string{"sqlite", "duckdb"} {
		t.Run(kind, func(t *testing.T) {
			installTestAgent(t, kind)
			filename := filepath.Join(t.TempDir(), "本地数据.sqlite")
			cfg := connection.ConnectionConfig{Type: kind, Host: filename, Timeout: 5, QueryTimeout: 10}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			client, err := Open(ctx, domain.Profile{Config: cfg})
			if err != nil {
				t.Fatal(err)
			}
			verifyLiveSQL(t, ctx, client, cfg)
			query := "WITH RECURSIVE numbers(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM numbers WHERE x<10002) SELECT x FROM numbers"
			if kind == "duckdb" {
				query = "SELECT range FROM range(10002)"
			}
			result := executeLive(t, ctx, client, query, false)
			if len(result[0].Rows) != MaxRows || !result[0].Truncated {
				t.Fatal("agent row budget failed")
			}
			if err = client.Close(); err != nil {
				t.Fatal(err)
			}
			if kind == "sqlite" {
				readonly, err := Open(ctx, domain.Profile{ReadOnly: true, Config: cfg})
				if err != nil {
					t.Fatal(err)
				}
				defer readonly.Close()
				if _, err = readonly.Execute(ctx, domain.Execution{Text: "CREATE TABLE blocked(id INTEGER)", Write: true}); err == nil {
					t.Fatal("SQLite database read-only mode was bypassed")
				}
			}
		})
	}
}
func verifyLiveSearch(t *testing.T, ctx context.Context, client Client) {
	executeLive(t, ctx, client, "PUT /superlink_fixture\n{}", true)
	executeLive(t, ctx, client, "PUT /superlink_fixture/_doc/1?refresh=true\n{\"name\":\"中文搜索\"}", true)
	result := executeLive(t, ctx, client, "GET /superlink_fixture/_search\n{\"query\":{\"match_all\":{}}}", false)
	if !strings.Contains(fmt.Sprint(result), "中文搜索") {
		t.Fatal("Elasticsearch REST console lost document")
	}
	executeLive(t, ctx, client, "DELETE /superlink_fixture", true)
}

func verifyLiveNacos(t *testing.T, ctx context.Context, client Client) {
	executeLive(t, ctx, client, `{"op":"publish","dataId":"superlink_fixture","group":"DEFAULT_GROUP","content":"中文配置","type":"text"}`, true)
	// Nacos propagates a successful publication to its read cache asynchronously.
	// Poll reads only; repeating the write would conceal unknown write outcomes.
	var result []domain.Result
	var readErr error
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		result, readErr = client.Execute(ctx, domain.Execution{Text: `{"op":"get","dataId":"superlink_fixture","group":"DEFAULT_GROUP"}`})
		if readErr == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(fmt.Sprint(result), "中文配置") {
		t.Fatal("Nacos configuration content lost")
	}
	objects, err := client.Objects(ctx, "")
	if err != nil || len(objects) == 0 {
		t.Fatalf("Nacos configuration browsing failed: %v", err)
	}
	if _, err = client.Scopes(ctx); err != nil {
		t.Fatal(err)
	}
	executeLive(t, ctx, client, `{"op":"delete","dataId":"superlink_fixture","group":"DEFAULT_GROUP"}`, true)
}
