package domain

import (
	"fmt"
	"strings"
)

type Family string

const (
	SQL           Family = "sql"
	Document      Family = "document"
	Search        Family = "search"
	Vector        Family = "vector"
	Cache         Family = "cache"
	Message       Family = "message"
	Configuration Family = "configuration"
)

type Descriptor struct {
	Key          string
	Name         string
	Family       Family
	Port         int
	Agent        bool
	Example      string
	WriteExample string
}

var catalog = []Descriptor{
	{Key: "mysql", Name: "MySQL", Family: SQL, Port: 3306},
	{Key: "goldendb", Name: "GoldenDB", Family: SQL, Port: 1523},
	{Key: "postgres", Name: "PostgreSQL", Family: SQL, Port: 5432},
	{Key: "oracle", Name: "Oracle", Family: SQL, Port: 1521, Example: "SELECT 1 FROM dual;"},
	{Key: "mariadb", Name: "MariaDB", Family: SQL, Port: 3306, Agent: true},
	{Key: "oceanbase", Name: "OceanBase", Family: SQL, Port: 2881, Agent: true},
	{Key: "diros", Name: "Apache Doris", Family: SQL, Port: 9030, Agent: true},
	{Key: "starrocks", Name: "StarRocks", Family: SQL, Port: 9030, Agent: true},
	{Key: "sphinx", Name: "Sphinx", Family: SQL, Port: 9306, Agent: true, Example: "SHOW TABLES"},
	{Key: "sqlserver", Name: "SQL Server", Family: SQL, Port: 1433, Agent: true},
	{Key: "sqlite", Name: "SQLite", Family: SQL, Agent: true},
	{Key: "duckdb", Name: "DuckDB", Family: SQL, Agent: true},
	{Key: "dameng", Name: "达梦 Dameng", Family: SQL, Port: 5236, Agent: true},
	{Key: "kingbase", Name: "金仓 Kingbase", Family: SQL, Port: 54321, Agent: true},
	{Key: "highgo", Name: "瀚高 HighGo", Family: SQL, Port: 5866, Agent: true},
	{Key: "vastbase", Name: "海量 Vastbase", Family: SQL, Port: 5432, Agent: true},
	{Key: "opengauss", Name: "openGauss", Family: SQL, Port: 5432, Agent: true},
	{Key: "gaussdb", Name: "GaussDB", Family: SQL, Port: 5432, Agent: true},
	{Key: "iris", Name: "InterSystems IRIS", Family: SQL, Port: 1972, Agent: true},
	{Key: "cache", Name: "InterSystems Caché", Family: SQL, Port: 1972, Agent: true},
	{Key: "mongodb", Name: "MongoDB", Family: Document, Port: 27017, Agent: true, Example: `{"find":"collection_name","filter":{},"limit":100}`},
	{Key: "tdengine", Name: "TDengine", Family: SQL, Port: 6041, Agent: true},
	{Key: "iotdb", Name: "Apache IoTDB", Family: SQL, Port: 6667, Agent: true, Example: "SHOW DATABASES"},
	{Key: "clickhouse", Name: "ClickHouse", Family: SQL, Port: 8123, Agent: true},
	{Key: "elasticsearch", Name: "Elasticsearch", Family: Search, Port: 9200, Agent: true, Example: "GET /_cat/indices?format=json"},
	{Key: "trino", Name: "Trino", Family: SQL, Port: 8080, Agent: true},
	{Key: "redis", Name: "Redis", Family: Cache, Port: 6379, Example: "PING"},
	{Key: "chroma", Name: "Chroma", Family: Vector, Port: 8000, Example: `{"list_collections":true}`},
	{Key: "qdrant", Name: "Qdrant", Family: Vector, Port: 6333, Example: `{"list_collections":true}`},
	{Key: "milvus", Name: "Milvus", Family: Vector, Port: 19530, Example: `{"list_collections":true}`},
	{Key: "rocketmq", Name: "RocketMQ", Family: Message, Port: 9876, Example: "SHOW TOPICS"},
	{Key: "mqtt", Name: "MQTT", Family: Message, Port: 1883, Example: "SHOW TOPICS"},
	{Key: "kafka", Name: "Kafka", Family: Message, Port: 9092, Example: "SHOW TOPICS"},
	{Key: "rabbitmq", Name: "RabbitMQ", Family: Message, Port: 15672, Example: "SHOW QUEUES"},
	{Key: "pulsar", Name: "Pulsar", Family: Message, Port: 6650, Example: "SHOW TOPICS"},
	{Key: "nacos", Name: "Nacos", Family: Configuration, Port: 8848, Example: `{"op":"configs","namespace":"","group":"DEFAULT_GROUP"}`},
	{Key: "custom", Name: "Custom Driver / DSN", Family: SQL},
}

func Catalog() []Descriptor { return append([]Descriptor(nil), catalog...) }

func Resolve(key string) (Descriptor, error) {
	key = strings.ToLower(strings.TrimSpace(key))
	switch key {
	case "doris":
		key = "diros"
	case "postgresql", "pg":
		key = "postgres"
	case "sqlite3":
		key = "sqlite"
	case "mssql":
		key = "sqlserver"
	}
	for _, item := range catalog {
		if item.Key == key {
			return item, nil
		}
	}
	return Descriptor{}, fmt.Errorf("unknown data source type %q", key)
}

func (d Descriptor) DefaultQuery() string {
	if d.Example != "" {
		return d.Example
	}
	if d.Family == SQL {
		return "SELECT 1;"
	}
	return ""
}
