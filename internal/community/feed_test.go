package community_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"usedit/internal/catalog"
	"usedit/internal/community"
)

// TestPreferenceAndBookmarkFeedbackChangeRecommendation 验证明确兴趣和实际收藏改变排序及真实解释。
func TestPreferenceAndBookmarkFeedbackChangeRecommendation(t *testing.T) {
	// 空数据库先建立目录资料基线，再分别改变兴趣与收藏观察排序和理由。
	store := openStore(t)
	client := newBrowser(store)
	before := client.feed(t, "?tab=for-you")
	if len(before.Items) != 3 {
		t.Fatalf("空社区应只返回目录资料: %+v", before.Items)
	}
	for _, post := range before.Items {
		if post.Kind != "reference" || post.SourceURL == "" || post.Author.Wallet != nil || post.HelpfulCount != 0 {
			t.Fatalf("资料笔记被当作用户评价: %+v", post)
		}
	}
	requireStatus(t, client.request(t, "PUT", "/preferences", map[string]any{"tags": []string{"数字藏品", "数字藏品"}}), 200)
	after := client.feed(t, "?tab=for-you")
	if len(after.Items) == 0 || after.Items[0].AppID != "opensea" || !strings.Contains(after.Items[0].Reason, "数字藏品") {
		t.Fatalf("显式兴趣未改变推荐或理由: %+v", after.Items)
	}
	if current := client.session(t); len(current.Preferences) != 1 {
		t.Fatal("重复偏好未去重")
	}
	requireStatus(t, client.request(t, "PUT", "/preferences", map[string]any{"tags": []string{}}), 200)
	requireStatus(t, client.request(t, "PUT", "/posts/reference-uniswap/bookmark", map[string]bool{"active": true}), 200)
	afterBookmark := client.feed(t, "?tab=for-you")
	if afterBookmark.Items[0].AppID != "uniswap" || !strings.Contains(afterBookmark.Items[0].Reason, "收藏") {
		t.Fatalf("真实收藏没有影响排序或解释: %+v", afterBookmark.Items)
	}
	if len(client.feed(t, "?tab=verified").Items) != 0 {
		t.Fatal("未核验的资料进入了凭据信息流")
	}
}

// TestFeedCursorBindsAccountFiltersAndLifetimeAndRechecksDeletion 验证游标范围、有效期与翻页时可见性。
func TestFeedCursorBindsAccountFiltersAndLifetimeAndRechecksDeletion(t *testing.T) {
	// 创建足够条目取得真实第二页，并在翻页前保留原快照验证隔离与删除。
	store := openStore(t)
	owner := newBrowser(store)
	owner.login(t, newSigner(t))
	for index := 0; index < 14; index++ {
		store.now.Add(int64(time.Second))
		owner.createPost(t, fmt.Sprintf("feed-record-%04d", index), fmt.Sprintf("使用记录第%d篇", index), "ens", []string{"链上身份"})
	}
	reader := newBrowser(store)
	first := reader.feed(t, "?tab=latest")
	if len(first.Items) != 12 || first.NextCursor == nil {
		t.Fatalf("首屏分页不正确: %+v", first)
	}
	path := cursorPath(*first.NextCursor)
	response := reader.request(t, "GET", path, nil)
	requireStatus(t, response, 200)
	second := readJSON[feed](t, response)
	if len(second.Items) != 5 || second.NextCursor != nil {
		t.Fatalf("第二页分页不正确: %+v", second)
	}
	for _, post := range second.Items {
		if hasPost(first.Items, post.ID) {
			t.Fatal("不同页出现重复内容")
		}
	}
	repeated := reader.request(t, "GET", path, nil)
	requireStatus(t, repeated, 200)
	if !reflect.DeepEqual(second, readJSON[feed](t, repeated)) {
		t.Fatal("同一快照重复读取的次序改变")
	}
	other := newBrowser(store)
	requireStatus(t, other.request(t, "GET", path, nil), 410)
	requireStatus(t, reader.request(t, "GET", path+"&category=identity", nil), 410)
	requireStatus(t, reader.request(t, "GET", "/feed?cursor=bad!", nil), 400)
	store.now.Add(int64(time.Second))
	added := owner.createPost(t, "feed-record-added", "翻页期间新发布", "ens", []string{"域名"})
	deleted := second.Items[0]
	hidden := second.Items[1]
	if deleted.Kind != "review" || hidden.Kind != "review" {
		t.Fatal("测试顺序需要第二页包含两篇已发布内容")
	}
	requireStatus(t, owner.request(t, "DELETE", "/posts/"+deleted.ID, nil), 200)
	requireStatus(t, reader.request(t, "POST", "/feedback", map[string]any{"post_id": hidden.ID, "kind": "hide", "event_id": "snapshot-hide-0001"}), 200)
	refreshed := reader.request(t, "GET", path, nil)
	requireStatus(t, refreshed, 200)
	page := readJSON[feed](t, refreshed)
	if hasPost(page.Items, deleted.ID) || hasPost(page.Items, hidden.ID) || hasPost(page.Items, added.ID) || len(page.Items) != 3 {
		t.Fatalf("快照未保留次序或未刷新可见性: %+v", page.Items)
	}
	store.now.Add(int64(16 * time.Minute))
	requireStatus(t, reader.request(t, "GET", path, nil), 410)
}

// TestSearchTreatsWildcardCharactersAsText 验证搜索输入只按文本匹配而不能改变 SQL 范围。
func TestSearchTreatsWildcardCharactersAsText(t *testing.T) {
	// 两个相近标题仅百分号不同，用于区分字面匹配与 SQL 通配符匹配。
	store := openStore(t)
	client := newBrowser(store)
	client.login(t, newSigner(t))
	wanted := client.createPost(t, "search-percent-0001", "费用 100% 记录", "ens", []string{"域名"})
	client.createPost(t, "search-percent-0002", "费用 1000 记录", "ens", []string{"域名"})
	result := client.feed(t, "?tab=latest&q="+url.QueryEscape("100%"))
	if len(result.Items) != 1 || result.Items[0].ID != wanted.ID {
		t.Fatalf("搜索将百分号作为了通配符: %+v", result.Items)
	}
	empty := client.feed(t, "?tab=latest&q="+url.QueryEscape("' OR 1=1 --"))
	if len(empty.Items) != 0 {
		t.Fatal("搜索文本改变了查询范围")
	}
}

// TestSavedAndVerifiedSnapshotsRecheckCurrentState 验证取消收藏及凭据降级会立即影响旧快照翻页。
func TestSavedAndVerifiedSnapshotsRecheckCurrentState(t *testing.T) {
	store := openStore(t)
	owner := newBrowser(store)
	account := newSigner(t)
	owner.login(t, account)
	app, found := catalog.Get("ens")
	if !found || len(app.Contracts) == 0 {
		t.Fatal("目录缺少已收录的 ENS 合约")
	}
	txHash := "0x" + strings.Repeat("a", 64)
	blockHash := "0x" + strings.Repeat("b", 64)
	var rejected atomic.Bool
	// 独立 HTTP 服务按正式批量协议返回交易、回执与区块资料。
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var requests []struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if decodeErr := json.NewDecoder(request.Body).Decode(&requests); decodeErr != nil {
			t.Errorf("RPC 请求解码: %v", decodeErr)
			http.Error(writer, "invalid request", 400)
			return
		}
		responses := make([]map[string]any, 0, len(requests))
		for _, call := range requests {
			var result any
			switch call.Method {
			case "eth_chainId":
				result = "0x1"
			case "eth_blockNumber":
				result = "0x20"
			case "eth_getTransactionByHash":
				result = map[string]any{"hash": txHash, "from": account.address, "to": app.Contracts[0], "chainId": "0x1", "blockNumber": "0x10", "blockHash": blockHash}
			case "eth_getTransactionReceipt":
				status := "0x1"
				if rejected.Load() {
					status = "0x0"
				}
				result = map[string]any{"transactionHash": txHash, "from": account.address, "to": app.Contracts[0], "status": status, "blockNumber": "0x10", "blockHash": blockHash}
			case "eth_getBlockByNumber":
				result = map[string]string{"number": "0x10", "hash": blockHash}
			default:
				t.Errorf("非预期 RPC 方法: %s", call.Method)
				http.Error(writer, "unknown method", 400)
				return
			}
			responses = append(responses, map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": result})
		}
		writer.Header().Set("Content-Type", "application/json")
		if encodeErr := json.NewEncoder(writer).Encode(responses); encodeErr != nil {
			t.Errorf("RPC 响应编码: %v", encodeErr)
		}
	}))
	t.Cleanup(upstream.Close)
	store.service.Config.RPCURL = upstream.URL
	store.service.HTTP = upstream.Client()
	for index := 0; index < 14; index++ {
		if index%3 == 0 {
			store.now.Add(int64(time.Minute))
		}
		post := owner.createPost(t, fmt.Sprintf("proof-feed-%04d", index), fmt.Sprintf("已核对交互记录%d", index), "ens", []string{"域名"})
		verified := owner.request(t, "POST", "/posts/"+post.ID+"/verify", map[string]string{"tx_hash": txHash})
		requireStatus(t, verified, 200)
		if result := readJSON[struct{ Post community.Post }](t, verified).Post; result.ProofStatus != "verified" || result.Revision != 2 {
			t.Fatalf("凭据没有通过真实接口落库: %+v", result)
		}
		requireStatus(t, owner.request(t, "PUT", "/posts/"+post.ID+"/bookmark", map[string]bool{"active": true}), 200)
	}
	saved := owner.feed(t, "?tab=saved")
	verified := owner.feed(t, "?tab=verified")
	if saved.NextCursor == nil || verified.NextCursor == nil {
		t.Fatal("缺少用于状态刷新验证的第二页")
	}
	savedPath := "/feed?tab=saved&cursor=" + url.QueryEscape(*saved.NextCursor)
	verifiedPath := "/feed?tab=verified&cursor=" + url.QueryEscape(*verified.NextCursor)
	response := owner.request(t, "GET", savedPath, nil)
	requireStatus(t, response, 200)
	second := readJSON[feed](t, response)
	if len(second.Items) != 2 {
		t.Fatalf("收藏快照第二页条数不正确: %+v", second)
	}
	target := second.Items[0]
	requireStatus(t, owner.request(t, "PUT", "/posts/"+target.ID+"/bookmark", map[string]bool{"active": false}), 200)
	rejected.Store(true)
	rechecked := owner.request(t, "POST", "/posts/"+target.ID+"/verify", map[string]string{"tx_hash": txHash})
	requireStatus(t, rechecked, 200)
	if post := readJSON[struct{ Post community.Post }](t, rechecked).Post; post.ProofStatus != "rejected" {
		t.Fatalf("执行失败回执未改变凭据状态: %+v", post)
	}
	for _, path := range []string{savedPath, verifiedPath} {
		response := owner.request(t, "GET", path, nil)
		requireStatus(t, response, 200)
		if hasPost(readJSON[feed](t, response).Items, target.ID) {
			t.Fatalf("状态变更后的内容仍在旧快照中: %s", path)
		}
	}
}
