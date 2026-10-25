//go:build !js

package database

import (
	"context"
	"database/sql"
	"log/slog"
	_ "modernc.org/sqlite"
)

type SQLite struct{ Conn *sql.DB }

// Open 为本地运行启用外键、WAL 和有限写入等待。
func Open(path string) (*SQLite, error) {
	conn, openErr := sql.Open("sqlite", path)
	if openErr != nil {
		slog.Error("sqlite_open", "error", openErr)
		return nil, openErr
	}
	// 连接级 PRAGMA 需要落在后续复用的同一连接上。
	conn.SetMaxOpenConns(1)
	if _, setupErr := conn.Exec("PRAGMA foreign_keys=ON; PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;"); setupErr != nil {
		slog.Error("sqlite_setup", "error", setupErr)
		conn.Close()
		return nil, setupErr
	}
	return &SQLite{Conn: conn}, nil
}

// Query 在有界查询后立即释放游标。
func (d *SQLite) Query(ctx context.Context, query string, args ...any) ([]Row, error) {
	rows, queryErr := d.Conn.QueryContext(ctx, query, args...)
	if queryErr != nil {
		slog.Error("sqlite_query", "error", queryErr)
		return nil, queryErr
	}
	return collect(rows)
}

// Batch 在一个事务中执行适配层语句批次；业务层不逐项发起数据库往返。
func (d *SQLite) Batch(ctx context.Context, statements []Statement) ([][]Row, error) {
	tx, beginErr := d.Conn.BeginTx(ctx, nil)
	if beginErr != nil {
		slog.Error("sqlite_begin", "error", beginErr)
		return nil, beginErr
	}
	// 任一步失败退出时整体回滚，只有全部语句成功才提交。
	defer tx.Rollback()
	result := make([][]Row, 0, len(statements))
	for _, statement := range statements {
		rows, queryErr := tx.QueryContext(ctx, statement.SQL, statement.Args...)
		if queryErr != nil {
			slog.Error("sqlite_batch_query", "error", queryErr)
			return nil, queryErr
		}
		items, scanErr := collect(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, items)
	}
	if commitErr := tx.Commit(); commitErr != nil {
		slog.Error("sqlite_commit", "error", commitErr)
		return nil, commitErr
	}
	return result, nil
}

// collect 复制结果并保留驱动的结束错误。
func collect(rows *sql.Rows) ([]Row, error) {
	defer rows.Close()
	columns, columnsErr := rows.Columns()
	if columnsErr != nil {
		slog.Error("sqlite_columns", "error", columnsErr)
		return nil, columnsErr
	}
	items := []Row{}
	// 当前游标内读取不增加数据库往返，复制值后才能释放游标。
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if scanErr := rows.Scan(pointers...); scanErr != nil {
			slog.Error("sqlite_scan", "error", scanErr)
			return nil, scanErr
		}
		row := Row{}
		for i, column := range columns {
			row[column] = values[i]
		}
		items = append(items, row)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		slog.Error("sqlite_rows", "error", rowsErr)
		return nil, rowsErr
	}
	return items, nil
}
