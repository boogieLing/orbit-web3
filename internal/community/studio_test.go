package community_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"usedit/internal/studio"
)

const appDocument = `<!doctype html><html><head><title>清单</title></head><body><button>添加</button><script>localStorage.setItem('items','[]')</script></body></html>`

type studioResult struct {
	Project struct {
		ID                string
		Version, Revision int
		ActiveRun         string `json:"active_run"`
	}
	Runs     []struct{ ID, Status, Prompt string }
	Versions []struct{ Version int }
}

// connectStudio 使用本地 TLS 服务检验协议、计费去重和状态迁移，不作为真实模型验收。
func connectStudio(t *testing.T, store *testStore, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	store.service.Config.Studio = studio.Config{URL: server.URL, Key: "test-only", Model: "protocol-test", Provider: "Protocol"}
	store.service.HTTP = server.Client()
}

// createStudio 经过正式鉴权入口创建项目。
func createStudio(t *testing.T, client *browser) string {
	t.Helper()
	client.session(t)
	response := client.request(t, "POST", "/studio/projects", map[string]any{"id": "project-id-for-tests", "title": "收藏工具"})
	requireStatus(t, response, 201)
	return "/studio/projects/" + readJSON[studioResult](t, response).Project.ID
}

// queueStudio 返回持久任务标识，并验证接收状态。
func queueStudio(t *testing.T, client *browser, path, prompt string, revision int) string {
	t.Helper()
	response := client.request(t, "POST", path+"/runs", map[string]any{"prompt": prompt, "revision": revision, "request_key": fmt.Sprintf("request-key-%020d", revision)})
	requireStatus(t, response, 202)
	return readJSON[struct{ Run struct{ ID string } }](t, response).Run.ID
}

// TestStudioLifecycle 验证多轮上下文、持久恢复、数据冲突与回退后的版本分支。
func TestStudioLifecycle(t *testing.T) {
	store := openStore(t)
	client := newBrowser(store)
	path := createStudio(t, client)
	var calls atomic.Int32
	var contexts [][]studio.Message
	connectStudio(t, store, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var input struct{ Messages []studio.Message }
		if decodeErr := json.NewDecoder(r.Body).Decode(&input); decodeErr != nil {
			t.Error(decodeErr)
		}
		contexts = append(contexts, input.Messages)
		if r.Header.Get("Authorization") != "Bearer test-only" {
			t.Error("认证头缺失")
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": appDocument}}}})
	})
	// 同一持久任务重复执行不重复访问上游；旧 revision 不得覆盖状态。
	first := queueStudio(t, client, path, "添加收藏功能", 0)
	for range 2 {
		requireStatus(t, client.request(t, "POST", path+"/runs/"+first+"/execute", nil), 200)
	}
	requireStatus(t, client.request(t, "PUT", path+"/state", map[string]any{"data": map[string]string{"items": "saved"}, "revision": 0}), 200)
	requireStatus(t, client.request(t, "PUT", path+"/state", map[string]any{"data": map[string]string{"items": "stale"}, "revision": 0}), 409)
	second := queueStudio(t, client, path, "增加搜索功能", 1)
	requireStatus(t, client.request(t, "POST", path+"/runs/"+second+"/execute", nil), 200)
	if calls.Load() != 2 || len(contexts[1]) != 4 || contexts[1][2].Content != appDocument {
		t.Fatalf("调用次数或上下文错误: %d", calls.Load())
	}
	// 恢复旧代码只移动当前指针，数据、完整对话和后续版本仍然可读。
	requireStatus(t, client.request(t, "POST", path+"/restore", map[string]any{"version": 1, "revision": 2}), 200)
	requireStatus(t, client.request(t, "POST", path+"/restore", map[string]any{"version": 2, "revision": 2}), 409)
	store.now.Add(int64(time.Minute))
	third := queueStudio(t, client, path, "调整背景颜色", 3)
	requireStatus(t, client.request(t, "POST", path+"/runs/"+third+"/execute", nil), 200)
	if strings.Contains(contexts[2][1].Content, "搜索") {
		t.Fatal("恢复后的分支混入被撤回需求")
	}
	store.restart(t)
	saved := client.request(t, "GET", path, nil)
	requireStatus(t, saved, 200)
	detail := readJSON[studioResult](t, saved)
	if len(detail.Runs) != 3 || len(detail.Versions) != 3 || detail.Project.Version != 4 {
		t.Fatalf("重启后恢复错误: %+v", detail)
	}
	state := client.request(t, "GET", path+"/state", nil)
	requireStatus(t, state, 200)
	if !strings.Contains(state.Body, "saved") {
		t.Fatal("业务数据丢失")
	}
	stranger := newBrowser(store)
	stranger.platform = "another-user"
	stranger.session(t)
	for _, suffix := range []string{"", "/versions/1", "/state", "/export"} {
		requireStatus(t, stranger.request(t, "GET", path+suffix, nil), 404)
	}
}

// TestStudioFailureAndExpiry 验证中断、上游拒绝和重复请求不会产生成功版本。
func TestStudioFailureAndExpiry(t *testing.T) {
	store := openStore(t)
	client := newBrowser(store)
	path := createStudio(t, client)
	var calls atomic.Int32
	connectStudio(t, store, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(402) })
	run := queueStudio(t, client, path, "制作收藏工具", 0)
	requireStatus(t, client.request(t, "POST", path+"/runs", map[string]any{"prompt": "制作收藏工具", "revision": 0, "request_key": "request-key-00000000000000000000"}), 200)
	requireStatus(t, client.request(t, "POST", path+"/runs", map[string]any{"prompt": "不同需求", "revision": 0, "request_key": "request-key-00000000000000000000"}), 409)
	requireStatus(t, client.request(t, "POST", path+"/runs/"+run+"/execute", nil), 502)
	requireStatus(t, client.request(t, "POST", path+"/runs/"+run+"/execute", nil), 200)
	if calls.Load() != 1 {
		t.Fatal("失败任务被自动重试")
	}
	// 排队超时由读取项目收敛；过期任务不会在恢复页面后继续扣费。
	response := client.request(t, "POST", path+"/runs", map[string]any{"prompt": "重新制作收藏工具", "revision": 0, "request_key": "second-request-key-for-tests"})
	requireStatus(t, response, 202)
	store.now.Add(int64(151 * time.Second))
	detail := readJSON[studioResult](t, client.request(t, "GET", path, nil))
	if detail.Project.ActiveRun != "" || detail.Project.Version != 0 || len(detail.Versions) != 0 {
		t.Fatalf("超时状态不正确: %+v", detail)
	}
	for _, run := range detail.Runs {
		if run.Status != "failed" {
			t.Fatal("超时未落为失败状态")
		}
	}
}

// TestStudioConcurrentExecute 多个执行请求竞争同一任务时，上游仅接收一次调用。
func TestStudioConcurrentExecute(t *testing.T) {
	store := openStore(t)
	client := newBrowser(store)
	path := createStudio(t, client)
	var calls atomic.Int32
	connectStudio(t, store, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": appDocument}}}})
	})
	run := queueStudio(t, client, path, "制作收藏清单", 0)
	// 复制已建立的会话，避免测试客户端自身共享可变 cookie。
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			parallel := *client
			response := parallel.raw("POST", path+"/runs/"+run+"/execute", "{}", nil)
			if response.Status != 200 {
				t.Errorf("并发执行返回 %d: %s", response.Status, response.Body)
			}
		}()
	}
	group.Wait()
	if calls.Load() != 1 {
		t.Fatalf("重复调用次数: %d", calls.Load())
	}
	if store.count(t, "SELECT COUNT(*) FROM studio_versions") != 1 {
		t.Fatal("版本提交重复")
	}
}
