package db

import (
	"context"
	"database/sql"
	"fmt"
)

// 本文件与 query_args_*.go 集中实现各 database/sql 支撑驱动的位置参数绑定方法。
// 每个实现与其所属驱动的 QueryContext/ExecContext 保持同一扫描方言与
// 空连接错误语义，仅多传 args；未在此实现的驱动（非 SQL 引擎）由能力
// 声明层标记为不支持参数绑定。构建标签按驱动文件镜像拆分。

func queryContextWithArgsOnConn(ctx context.Context, conn *sql.DB, dialect string, query string, args []any) ([]map[string]interface{}, []string, error) {
	rows, err := conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	if dialect == "" {
		return scanRowsContext(ctx, rows)
	}
	return scanRowsForDialectContext(ctx, rows, dialect)
}

// 事务/会话执行器的参数化变体：与各自 QueryContext/ExecContext 保持同一
// 锁语义、状态机与扫描方言，仅向底层 Conn/Tx 多传位置参数。

func (e *sqlConnTransactionExecer) QueryContextWithArgs(ctx context.Context, query string, args []any) ([]map[string]interface{}, []string, error) {
	conn, err := e.activeConn()
	if err != nil {
		return nil, nil, err
	}
	rows, err := conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	return scanRowsForDialectContext(ctx, rows, e.scanDialect)
}

func (e *sqlConnTransactionExecer) ExecContextWithArgs(ctx context.Context, query string, args []any) (int64, error) {
	conn, err := e.activeConn()
	if err != nil {
		return 0, err
	}
	res, err := conn.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (e *sqlTxStatementExecer) QueryContextWithArgs(ctx context.Context, query string, args []any) ([]map[string]interface{}, []string, error) {
	tx, err := e.activeTx()
	if err != nil {
		return nil, nil, err
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	return scanRowsContext(ctx, rows)
}

func (e *sqlTxStatementExecer) ExecContextWithArgs(ctx context.Context, query string, args []any) (int64, error) {
	tx, err := e.activeTx()
	if err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func execContextWithArgsOnConn(ctx context.Context, conn *sql.DB, query string, args []any) (int64, error) {
	res, err := conn.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (m *MySQLDB) QueryContextWithArgs(ctx context.Context, query string, args []any) ([]map[string]interface{}, []string, error) {
	if m.conn == nil {
		return nil, nil, fmt.Errorf("连接未打开")
	}
	return queryContextWithArgsOnConn(ctx, m.conn, "mysql", query, args)
}

func (m *MySQLDB) ExecContextWithArgs(ctx context.Context, query string, args []any) (int64, error) {
	if m.conn == nil {
		return 0, fmt.Errorf("连接未打开")
	}
	return execContextWithArgsOnConn(ctx, m.conn, query, args)
}

func (p *PostgresDB) QueryContextWithArgs(ctx context.Context, query string, args []any) ([]map[string]interface{}, []string, error) {
	if p.conn == nil {
		return nil, nil, fmt.Errorf("连接未打开")
	}
	return queryContextWithArgsOnConn(ctx, p.conn, "", query, args)
}

func (p *PostgresDB) ExecContextWithArgs(ctx context.Context, query string, args []any) (int64, error) {
	if p.conn == nil {
		return 0, fmt.Errorf("连接未打开")
	}
	return execContextWithArgsOnConn(ctx, p.conn, query, args)
}

func (o *OracleDB) QueryContextWithArgs(ctx context.Context, query string, args []any) ([]map[string]interface{}, []string, error) {
	if o.conn == nil {
		return nil, nil, fmt.Errorf("连接未打开")
	}
	return queryContextWithArgsOnConn(ctx, o.conn, o.scanDialect, query, args)
}

func (o *OracleDB) ExecContextWithArgs(ctx context.Context, query string, args []any) (int64, error) {
	if o.conn == nil {
		return 0, fmt.Errorf("连接未打开")
	}
	return execContextWithArgsOnConn(ctx, o.conn, query, args)
}

func (c *CustomDB) QueryContextWithArgs(ctx context.Context, query string, args []any) ([]map[string]interface{}, []string, error) {
	if c.conn == nil {
		return nil, nil, fmt.Errorf("连接未打开")
	}
	return queryContextWithArgsOnConn(ctx, c.conn, c.scanDialect(), query, args)
}

func (c *CustomDB) ExecContextWithArgs(ctx context.Context, query string, args []any) (int64, error) {
	if c.conn == nil {
		return 0, fmt.Errorf("连接未打开")
	}
	return execContextWithArgsOnConn(ctx, c.conn, query, args)
}

func (e *sqlConnStatementExecer) QueryContextWithArgs(ctx context.Context, query string, args []any) ([]map[string]interface{}, []string, error) {
	if e == nil || e.conn == nil {
		return nil, nil, localizedDatabaseRuntimeError("db.backend.error.connection_not_open", nil)
	}
	rows, err := e.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	return scanRowsForDialectContext(ctx, rows, e.scanDialect)
}

func (e *sqlConnStatementExecer) ExecContextWithArgs(ctx context.Context, query string, args []any) (int64, error) {
	if e == nil || e.conn == nil {
		return 0, localizedDatabaseRuntimeError("db.backend.error.connection_not_open", nil)
	}
	res, err := e.conn.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
