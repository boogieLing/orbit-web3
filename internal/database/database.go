// Package database 定义领域服务与两种 SQL 运行环境之间的最小边界。
package database

import "context"

type Row map[string]any
type Statement struct {
	SQL  string
	Args []any
}
type DB interface {
	Query(context.Context, string, ...any) ([]Row, error)
	Batch(context.Context, []Statement) ([][]Row, error)
}

// String 将 SQLite 和 D1 的文本返回值统一为字符串。
func String(r Row, key string) string { v, _ := r[key].(string); return v }

// Int 将两种驱动的数值返回统一为整数。
func Int(r Row, key string) int {
	// 原生驱动返回 int64，宿主 JSON 解码返回 float64。
	switch v := r[key].(type) {
	case int64:
		return int(v)
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}
