//go:build js && wasm

package database

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"syscall/js"
)

type D1 struct{ Bridge js.Value }
type sqlResult struct {
	Success bool   `json:"success"`
	Results []Row  `json:"results"`
	Error   string `json:"error"`
}

// Query 通过宿主提供的请求内数据库能力执行一个语句。
func (d *D1) Query(ctx context.Context, query string, args ...any) ([]Row, error) {
	if contextErr := ctx.Err(); contextErr != nil {
		slog.Warn("d1_context", "error", contextErr)
		return nil, contextErr
	}
	if args == nil {
		args = []any{}
	}
	payload, marshalErr := json.Marshal(args)
	if marshalErr != nil {
		slog.Error("d1_args", "error", marshalErr)
		return nil, marshalErr
	}
	// 宿主完成异步 IO 后才解码，失败信封不能当作空查询结果。
	raw, awaitErr := Await(d.Bridge.Call("query", query, string(payload)))
	if awaitErr != nil {
		return nil, awaitErr
	}
	var result sqlResult
	if parseErr := json.Unmarshal([]byte(raw.String()), &result); parseErr != nil {
		slog.Error("d1_result", "error", parseErr)
		return nil, parseErr
	}
	if !result.Success {
		slog.Error("d1_failure", "error", result.Error)
		return nil, fmt.Errorf("D1 query failed: %s", result.Error)
	}
	if result.Results == nil {
		result.Results = []Row{}
	}
	return result.Results, nil
}

// Batch 一次提交原子语句组，避免在业务层按记录循环发起 IO。
func (d *D1) Batch(ctx context.Context, statements []Statement) ([][]Row, error) {
	payload, marshalErr := json.Marshal(statements)
	if marshalErr != nil {
		slog.Error("d1_batch_args", "error", marshalErr)
		return nil, marshalErr
	}
	raw, awaitErr := Await(d.Bridge.Call("batch", string(payload)))
	if awaitErr != nil {
		return nil, awaitErr
	}
	var results []sqlResult
	if parseErr := json.Unmarshal([]byte(raw.String()), &results); parseErr != nil {
		slog.Error("d1_batch_result", "error", parseErr)
		return nil, parseErr
	}
	// 批次由 D1 原子提交，这里的循环只检查返回值，不再发起 IO。
	rows := make([][]Row, len(results))
	for i, result := range results {
		if !result.Success {
			slog.Error("d1_batch_failure", "error", result.Error)
			return nil, fmt.Errorf("D1 batch failed: %s", result.Error)
		}
		rows[i] = result.Results
	}
	return rows, nil
}

// Await 等待宿主异步回调，不阻塞 JavaScript 事件循环。
func Await(promise js.Value) (js.Value, error) {
	type outcome struct {
		value   js.Value
		failure string
	}
	channel := make(chan outcome, 1)
	success := js.FuncOf(func(_ js.Value, args []js.Value) any { channel <- outcome{value: args[0]}; return nil })
	failure := js.FuncOf(func(_ js.Value, args []js.Value) any { channel <- outcome{failure: args[0].String()}; return nil })
	promise.Call("then", success).Call("catch", failure)
	// 回调结束后释放 JS 引用，避免每个数据库调用积累桥接资源。
	result := <-channel
	success.Release()
	failure.Release()
	if result.failure != "" {
		slog.Error("host_operation", "error", result.failure)
		return js.Undefined(), fmt.Errorf("host operation failed: %s", result.failure)
	}
	return result.value, nil
}
