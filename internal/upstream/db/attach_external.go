package db

import (
	"context"
	"errors"
)

// ErrExternalAttachNotAttached 表示要卸载的外部数据源别名当前未附加。
// 调用方据此把 DETACH 映射为幂等的“无需卸载”提示而不是失败。
var ErrExternalAttachNotAttached = errors.New("external attach: alias not attached")

// 外部数据源附加种类，取值为 DuckDB 扩展/原生 ATTACH 的 TYPE：
// mysql、postgres、sqlite；duckdb 表示原生 DuckDB 文件。
const (
	ExternalAttachKindMySQL    = "mysql"
	ExternalAttachKindPostgres = "postgres"
	ExternalAttachKindSQLite   = "sqlite"
	ExternalAttachKindDuckDB   = "duckdb"
)

// ExternalAttachSpec 是应用层从保存连接解析出的具体附加参数。
// 驱动层据此执行 DuckDB 的 SECRET + ATTACH，只接触具体连接参数，
// 不感知保存连接仓库与凭据存储（分层见 AGENTS.md 第 2 条）。
type ExternalAttachSpec struct {
	Kind       string // ExternalAttachKind* 之一
	Host       string
	Port       int
	User       string
	Password   string
	Database   string // mysql：要附加的数据库名；postgres：进入 SECRET 的 database
	FilePath   string // sqlite / duckdb 的文件路径
	Alias      string // 目标 catalog 别名，调用方保证是合法标识符
	ReadOnly   bool
	SecretName string // mysql / postgres 使用的会话级 SECRET 名
	// ConnectionID 标记来源保存连接（仅用于附加状态展示与同源判定，
	// 不参与连接参数）；代理与驱动原样保存并在列表查询中返回。
	ConnectionID string
}

// ExternalAttachmentInfo 描述当前会话中一个由本驱动创建的附加关系。
type ExternalAttachmentInfo struct {
	Alias        string `json:"alias"`
	ConnectionID string `json:"connectionId,omitempty"`
	Kind         string `json:"kind"`
	ReadOnly     bool   `json:"readOnly"`
}

// ExternalDatabaseAttacher 是驱动可选能力：把外部数据源附加到当前
// DuckDB 实例（参考 TableExistsChecker 的可选接口模式）。
type ExternalDatabaseAttacher interface {
	AttachExternalDatabase(ctx context.Context, spec ExternalAttachSpec) error
	// DetachExternalDatabase 卸载指定别名；未附加时返回 ErrExternalAttachNotAttached。
	DetachExternalDatabase(ctx context.Context, alias string) error
}

// ExternalAttachmentLister 是可选能力：列出当前会话由本驱动创建、仍有效的
// 附加关系（供“附加已保存数据源”选择器展示状态，见 issue #1270 讨论）。
type ExternalAttachmentLister interface {
	ListExternalAttachments(ctx context.Context) ([]ExternalAttachmentInfo, error)
}
