package studio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const validDocument = `<!doctype html><html lang="zh"><head><title>工具</title></head><body><button>添加</button><script>console.log('ready')</script></body></html>`

// TestBuildResponseBoundary 覆盖格式封装、截断、外部资源与服务端拒绝。
func TestBuildResponseBoundary(t *testing.T) {
	encoded, _ := json.Marshal(map[string]string{"html": validDocument})
	cases := []struct {
		name, content, finish string
		status                int
		good                  bool
	}{
		{"完整文件", validDocument, "stop", 200, true},
		{"结构文件", string(encoded), "stop", 200, true},
		{"Markdown包装", "```html\n" + validDocument + "\n```", "stop", 200, true},
		{"截断文件", validDocument, "length", 200, false},
		{"缺少文件", "{}", "stop", 200, false},
		{"网络访问", strings.Replace(validDocument, "console.log('ready')", "fetch('https://example.com')", 1), "stop", 200, false},
		{"外部脚本", strings.Replace(validDocument, "<script>", `<script src="https://example.com/a.js">`, 1), "stop", 200, false},
		{"余额不足", validDocument, "stop", 402, false},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(item.status)
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": item.finish, "message": map[string]string{"content": item.content}}}})
			}))
			defer server.Close()
			// 使用本地 TLS 只检验传输合同，不消费线上额度。
			result, buildErr := Build(context.Background(), server.Client(), Config{URL: server.URL, Key: "test-only", Model: "test"}, Context("", nil, "制作工具"))
			if (buildErr == nil) != item.good {
				t.Fatalf("结果与预期不符: %v", buildErr)
			}
			if item.good && result != validDocument {
				t.Fatal("文件内容发生意外变化")
			}
		})
	}
}

// TestBuildDoesNotForwardRedirect 验证凭据不会通过重定向发往其他端点。
func TestBuildDoesNotForwardRedirect(t *testing.T) {
	received := false
	destination := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received = true }))
	defer destination.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 307) }))
	defer source.Close()
	_, buildErr := Build(context.Background(), source.Client(), Config{URL: source.URL, Key: "test-only", Model: "test"}, Context("", nil, "制作工具"))
	if buildErr == nil || received {
		t.Fatal("重定向边界失效")
	}
}
