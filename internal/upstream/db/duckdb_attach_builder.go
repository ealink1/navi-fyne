package db

import (
	"fmt"
	"regexp"
	"strings"
)

// duckDBAttachAliasPattern 限制别名为安全标识符，防止拼进 ATTACH/DETACH 语句的
// 别名注入额外子句。
var duckDBAttachAliasPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// duckDBAttachmentSpec 记录附加关系的关键参数，用于同源替换判定（保存文件重跑
// 时同源即替换重建、绑定最新凭据）与异源冲突报错。仅存活于连接实例内存中。
type duckDBAttachmentSpec struct {
	kind         string
	host         string
	port         int
	user         string
	password     string
	database     string
	filePath     string
	readOnly     bool
	connectionID string
}

// sameExternalAttachmentIdentity 判断两条附加记录是否同一数据源：
// 忽略 password（凭据轮换属同源重跑，应替换重建而非冲突）与 readOnly
// （切换只读/读写同样是同源重跑，重建即生效，不应误报别名被占用）。
func sameExternalAttachmentIdentity(a, b duckDBAttachmentSpec) bool {
	a.password = ""
	b.password = ""
	a.readOnly = false
	b.readOnly = false
	return a == b
}

// buildCreateExternalSecretStatement 生成创建外部数据源 SECRET 的 DDL。
// mysql 扩展要求库名经 SECRET 的 DATABASE 传递（path 被当作主机 DSN）；
// postgres 分支形态与之对齐。返回 "" 表示该 kind 不需要 SECRET（如 sqlite/duckdb 原生）。
func buildCreateExternalSecretStatement(spec ExternalAttachSpec) string {
	switch spec.Kind {
	case ExternalAttachKindMySQL:
		return fmt.Sprintf("CREATE OR REPLACE SECRET %s (TYPE MYSQL, HOST %s, PORT %d, USER %s, PASSWORD %s, DATABASE %s)",
			spec.SecretName, quoteDuckDBStringLiteral(spec.Host), spec.Port,
			quoteDuckDBStringLiteral(spec.User), quoteDuckDBStringLiteral(spec.Password),
			quoteDuckDBStringLiteral(spec.Database))
	case ExternalAttachKindPostgres:
		return fmt.Sprintf("CREATE OR REPLACE SECRET %s (TYPE POSTGRES, HOST %s, PORT %d, USER %s, PASSWORD %s, DATABASE %s)",
			spec.SecretName, quoteDuckDBStringLiteral(spec.Host), spec.Port,
			quoteDuckDBStringLiteral(spec.User), quoteDuckDBStringLiteral(spec.Password),
			quoteDuckDBStringLiteral(spec.Database))
	default:
		return ""
	}
}

func buildDuckDBAttachStatement(spec ExternalAttachSpec) string {
	readOnly := ""
	if spec.ReadOnly {
		readOnly = ", READ_ONLY"
	}
	switch spec.Kind {
	case ExternalAttachKindMySQL:
		// mysql 扩展把 ATTACH 的 path 当作主机 DSN 而非库名：path 必须为空串，
		// 库名经 SECRET 的 DATABASE 提供（与 Postgres 分支形态一致）。
		return fmt.Sprintf("ATTACH '' AS %s (TYPE MYSQL, SECRET %s%s)",
			quoteDuckDBAttachAlias(spec.Alias), spec.SecretName, readOnly)
	case ExternalAttachKindPostgres:
		return fmt.Sprintf("ATTACH '' AS %s (TYPE POSTGRES, SECRET %s%s)",
			quoteDuckDBAttachAlias(spec.Alias), spec.SecretName, readOnly)
	case ExternalAttachKindSQLite:
		return fmt.Sprintf("ATTACH %s AS %s (TYPE SQLITE%s)",
			quoteDuckDBStringLiteral(spec.FilePath), quoteDuckDBAttachAlias(spec.Alias), readOnly)
	default:
		if spec.ReadOnly {
			return fmt.Sprintf("ATTACH %s AS %s (READ_ONLY)", quoteDuckDBStringLiteral(spec.FilePath), quoteDuckDBAttachAlias(spec.Alias))
		}
		return fmt.Sprintf("ATTACH %s AS %s", quoteDuckDBStringLiteral(spec.FilePath), quoteDuckDBAttachAlias(spec.Alias))
	}
}

func quoteDuckDBStringLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// quoteDuckDBAttachAlias 把别名包进双引号，避免 from/table 等保留字别名
// 触发解析错误；标识符白名单已保证不含双引号，此处仅防御性转义。
func quoteDuckDBAttachAlias(value string) string {
	return "\"" + strings.ReplaceAll(value, "\"", "\"\"") + "\""
}
