//go:build js && wasm

package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"syscall/js"
	"usedit/internal/community"
	"usedit/internal/database"
)

// main 注册服务入口，并保持运行时等待请求。
func main() {
	js.Global().Set("useditHandle", js.FuncOf(handle))
	select {}
}

// handle 将异步数据库操作转换成宿主 Promise。
func handle(this js.Value, args []js.Value) any {
	bridge, input, config := args[0], args[1].String(), args[2].String()
	executor := js.FuncOf(func(this js.Value, callbacks []js.Value) any {
		resolve, reject := callbacks[0], callbacks[1]
		// 在 goroutine 内等待数据库 Promise，让宿主事件循环继续处理回调。
		go func() {
			var request community.Request
			var settings community.Config
			if parseErr := json.Unmarshal([]byte(input), &request); parseErr != nil {
				slog.Error("worker_input", "error", parseErr)
				reject.Invoke("请求格式无效")
				return
			}
			if configErr := json.Unmarshal([]byte(config), &settings); configErr != nil {
				slog.Error("worker_config", "error", configErr)
				reject.Invoke("配置格式无效")
				return
			}
			result := community.New(&database.D1{Bridge: bridge}, settings).Handle(context.Background(), request)
			payload, marshalErr := json.Marshal(result)
			if marshalErr != nil {
				slog.Error("worker_output", "error", marshalErr)
				reject.Invoke(marshalErr.Error())
				return
			}
			resolve.Invoke(string(payload))
		}()
		return nil
	})
	promise := js.Global().Get("Promise").New(executor)
	executor.Release()
	return promise
}
