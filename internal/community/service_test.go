package community_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/sha3"
	"usedit/internal/community"
	"usedit/internal/database"
)

type testStore struct {
	db      *database.SQLite
	service *community.Service
	path    string
	now     atomic.Int64
}

// openStore 在独立文件中执行正式迁移，确保约束和事务与本地运行一致。
func openStore(t *testing.T) *testStore {
	t.Helper()
	// 使用项目正式迁移而非测试专用表，外键、唯一约束和索引都会参与验证。
	store := &testStore{path: filepath.Join(t.TempDir(), "community.sqlite")}
	store.now.Store(time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC).UnixNano())
	store.open(t)
	paths, globErr := filepath.Glob("../../drizzle/*.sql")
	if globErr != nil || len(paths) == 0 {
		t.Fatalf("迁移文件读取失败: %v, %v", paths, globErr)
	}
	for _, path := range paths {
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, migrateErr := store.db.Conn.ExecContext(context.Background(), string(source)); migrateErr != nil {
			t.Fatalf("执行正式迁移 %s: %v", path, migrateErr)
		}
	}
	t.Cleanup(func() {
		if closeErr := store.db.Conn.Close(); closeErr != nil {
			t.Errorf("关闭数据库: %v", closeErr)
		}
	})
	return store
}

// open 为已有路径创建正式数据库连接，并注入可推进的请求时钟。
func (store *testStore) open(t *testing.T) {
	t.Helper()
	connection, openErr := database.Open(store.path)
	if openErr != nil {
		t.Fatal(openErr)
	}
	store.db = connection
	store.service = community.New(connection, community.Config{})
	store.service.Now = func() time.Time { return time.Unix(0, store.now.Load()).UTC() }
}

// restart 关闭后重新打开同一文件，验证持久化状态而非进程内缓存。
func (store *testStore) restart(t *testing.T) {
	t.Helper()
	if closeErr := store.db.Conn.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	store.open(t)
}

// count 只读核对数据库行数，检查重复请求与失败事务的最终结果。
func (store *testStore) count(t *testing.T, query string, arguments ...any) int {
	t.Helper()
	var count int
	if queryErr := store.db.Conn.QueryRowContext(context.Background(), query, arguments...).Scan(&count); queryErr != nil {
		t.Fatalf("读取持久化计数: %v", queryErr)
	}
	return count
}

type browser struct {
	store    *testStore
	cookie   string
	origin   string
	platform string
}

// newBrowser 创建没有既有身份的请求端，各实例独立保存会话。
func newBrowser(store *testStore) *browser {
	return &browser{store: store, origin: "https://usedit.test", platform: "local-test"}
}

// raw 通过正式服务入口请求，并保存服务端返回的会话轮换结果。
func (client *browser) raw(method, path, body string, overrides map[string]string) community.Response {
	// 默认请求保持同源，只有明确传入的覆盖字段才改变协议边界条件。
	headers := map[string]string{"cookie": client.cookie, "origin": client.origin, "x-usedit-request": "1", "content-type": "application/json"}
	for key, value := range overrides {
		headers[key] = value
	}
	response := client.store.service.Handle(context.Background(), community.Request{Method: method, URL: client.origin + "/api/v1" + path, Headers: headers, Body: body, PlatformID: client.platform})
	if value := response.Headers["set-cookie"]; value != "" {
		parsed := (&http.Response{Header: http.Header{"Set-Cookie": {value}}}).Cookies()
		if len(parsed) == 1 {
			client.cookie = parsed[0].Name + "=" + parsed[0].Value
		}
	}
	return response
}

// request 编码业务输入并保留必要的请求头覆盖能力。
func (client *browser) request(t *testing.T, method, path string, body any, headers ...map[string]string) community.Response {
	t.Helper()
	// nil 表示没有请求体，其他值完整编码，以便区分无正文与 JSON 空值。
	text := ""
	if body != nil {
		encoded, encodeErr := json.Marshal(body)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		text = string(encoded)
	}
	var overrides map[string]string
	if len(headers) > 0 {
		overrides = headers[0]
	}
	return client.raw(method, path, text, overrides)
}

// requireStatus 同时检查业务状态、请求追踪和身份相关响应缓存策略。
func requireStatus(t *testing.T, response community.Response, expected int) {
	t.Helper()
	// 成功和拒绝响应都必须带追踪标识，且不能缓存带会话状态的内容。
	if response.Status != expected {
		t.Fatalf("状态码 %d，期望 %d；响应 %s", response.Status, expected, response.Body)
	}
	if response.Headers["cache-control"] != "no-store" || response.Headers["x-request-id"] == "" {
		t.Fatalf("缺少请求追踪或缓存控制: %#v", response.Headers)
	}
}

// readJSON 将正式接口响应还原为合同类型，格式错误时保留响应便于定位。
func readJSON[T any](t *testing.T, response community.Response) T {
	t.Helper()
	var value T
	if parseErr := json.Unmarshal([]byte(response.Body), &value); parseErr != nil {
		t.Fatalf("响应 JSON 解码: %v; %s", parseErr, response.Body)
	}
	return value
}

// errorCode 读取稳定业务错误码，区分预期冲突与存储故障。
func errorCode(t *testing.T, response community.Response) string {
	t.Helper()
	return readJSON[struct {
		Error struct{ Code string } `json:"error"`
	}](t, response).Error.Code
}

// session 经过公开接口初始化或恢复身份，不直接创建用户记录。
func (client *browser) session(t *testing.T) community.User {
	t.Helper()
	response := client.request(t, "GET", "/session", nil)
	requireStatus(t, response, 200)
	return readJSON[struct{ User community.User }](t, response).User
}

type signer struct {
	key     *secp256k1.PrivateKey
	address string
}

// newSigner 使用实际曲线密钥，不绕过挑战或账户校验。
func newSigner(t *testing.T) signer {
	t.Helper()
	key, generateErr := secp256k1.GeneratePrivateKey()
	if generateErr != nil {
		t.Fatal(generateErr)
	}
	hash := sha3.NewLegacyKeccak256()
	hash.Write(key.PubKey().SerializeUncompressed()[1:])
	return signer{key: key, address: "0x" + hex.EncodeToString(hash.Sum(nil)[12:])}
}

// sign 按个人消息签名协议计算摘要并输出带恢复位的曲线签名。
func (account signer) sign(message string) string {
	hash := sha3.NewLegacyKeccak256()
	hash.Write([]byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len([]byte(message)))))
	hash.Write([]byte(message))
	compact := ecdsa.SignCompact(account.key, hash.Sum(nil), false)
	personal := append(append([]byte{}, compact[1:]...), compact[0])
	return "0x" + hex.EncodeToString(personal)
}

type challenge struct {
	ID      string `json:"challenge_id"`
	Message string `json:"message"`
}

// challenge 获取当前会话绑定的签名请求，确保认证使用完整公开流程。
func (client *browser) challenge(t *testing.T, account signer) challenge {
	t.Helper()
	response := client.request(t, "POST", "/auth/challenge", map[string]string{"address": account.address})
	requireStatus(t, response, 200)
	return readJSON[challenge](t, response)
}

// login 完成挑战签名和验证，并检查返回的钱包与实际签名密钥对应。
func (client *browser) login(t *testing.T, account signer) community.User {
	t.Helper()
	issued := client.challenge(t, account)
	response := client.request(t, "POST", "/auth/verify", map[string]string{"challenge_id": issued.ID, "signature": account.sign(issued.Message)})
	requireStatus(t, response, 200)
	user := readJSON[struct{ User community.User }](t, response).User
	if user.Wallet == nil || *user.Wallet != account.address {
		t.Fatalf("钱包身份未绑定: %+v", user)
	}
	return user
}

// postBody 提供满足正文边界的发布输入，调用者显式决定应用与标签。
func postBody(title, app string, tags []string) map[string]any {
	return map[string]any{"app_id": app, "title": title, "body": "我在实际使用中先检查网络与费用，再逐步完成操作并记录结果。", "sentiment": "neutral", "tags": tags}
}

// createPost 通过钱包权限与幂等检查创建记录，返回实际持久化标识。
func (client *browser) createPost(t *testing.T, key, title, app string, tags []string) community.Post {
	t.Helper()
	response := client.request(t, "POST", "/posts", postBody(title, app, tags), map[string]string{"idempotency-key": key})
	requireStatus(t, response, 201)
	return readJSON[struct{ Post community.Post }](t, response).Post
}

// getPost 读取公开内容及当前身份的互动状态。
func (client *browser) getPost(t *testing.T, id string) community.Post {
	t.Helper()
	response := client.request(t, "GET", "/posts/"+id, nil)
	requireStatus(t, response, 200)
	return readJSON[struct{ Post community.Post }](t, response).Post
}

type feed struct {
	Items      []community.Post `json:"items"`
	NextCursor *string          `json:"next_cursor"`
}

// feed 获取一页推荐结果并校验成功响应。
func (client *browser) feed(t *testing.T, query string) feed {
	t.Helper()
	response := client.request(t, "GET", "/feed"+query, nil)
	requireStatus(t, response, 200)
	return readJSON[feed](t, response)
}

// hasPost 按持久化标识检查内容是否出现在结果中。
func hasPost(posts []community.Post, id string) bool {
	// 仅比较内容标识，标题和排序变化不能影响可见性判断。
	for _, post := range posts {
		if post.ID == id {
			return true
		}
	}
	return false
}

// collectionBody 使用真实目录条目构建可编译的精选页保存参数。
func collectionBody(title string, version int) map[string]any {
	return map[string]any{"title": title, "description": "整理实际选中的应用与资料。", "instructions": "", "layout": "editorial", "theme": "sage", "app_ids": []string{"ens"}, "post_ids": []string{"reference-ens"}, "version": version}
}

// createCollection 完成内容读取、页面编译与首版持久化的完整请求。
func (client *browser) createCollection(t *testing.T, title string) community.Collection {
	t.Helper()
	response := client.request(t, "POST", "/collections", collectionBody(title, 0))
	requireStatus(t, response, 201)
	return readJSON[struct{ Collection community.Collection }](t, response).Collection
}

// cursorPath 保持分页筛选条件不变并正确编码服务端游标。
func cursorPath(cursor string) string {
	return "/feed?tab=latest&cursor=" + url.QueryEscape(cursor)
}

// TestGuestSessionPersistsAndStaysBoundToPlatform 验证游客持久化、平台隔离和令牌只存摘要。
func TestGuestSessionPersistsAndStaysBoundToPlatform(t *testing.T) {
	// 文件重开后继续使用原 Cookie，排除仅靠进程内对象保留身份的可能。
	store := openStore(t)
	client := newBrowser(store)
	response := client.request(t, "GET", "/session", nil)
	requireStatus(t, response, 200)
	user := readJSON[struct{ User community.User }](t, response).User
	parsed := (&http.Response{Header: http.Header{"Set-Cookie": {response.Headers["set-cookie"]}}}).Cookies()
	if len(parsed) != 1 || !parsed[0].HttpOnly || !parsed[0].Secure || parsed[0].SameSite != http.SameSiteLaxMode || parsed[0].Path != "/" || parsed[0].MaxAge <= 0 {
		t.Fatalf("会话 Cookie 标志不完整: %+v", parsed)
	}
	if user.Wallet != nil || user.ID == "" {
		t.Fatalf("游客身份不正确: %+v", user)
	}
	for index := 0; index < 2; index++ {
		requireStatus(t, client.request(t, "PUT", "/posts/reference-ens/bookmark", map[string]bool{"active": true}), 200)
	}
	store.restart(t)
	if restored := client.session(t); restored.ID != user.ID {
		t.Fatalf("重启后身份丢失: %s != %s", restored.ID, user.ID)
	}
	saved := client.feed(t, "?tab=saved")
	if len(saved.Items) != 1 || saved.Items[0].ID != "reference-ens" || !saved.Items[0].Bookmarked {
		t.Fatalf("游客收藏未持久化或重复: %+v", saved.Items)
	}
	other := *client
	other.platform = "another-platform"
	if outsider := other.session(t); outsider.ID == user.ID {
		t.Fatal("跨平台请求复用了会话身份")
	}
	if len(other.feed(t, "?tab=saved").Items) != 0 {
		t.Fatal("其他身份读取到游客收藏")
	}
	if got := store.count(t, "SELECT count(*) FROM sessions WHERE token=?", strings.TrimPrefix(client.cookie, "usedit_session=")); got != 0 {
		t.Fatal("数据库保存了原始会话令牌")
	}
}
