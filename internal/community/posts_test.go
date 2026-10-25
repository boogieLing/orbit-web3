package community_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"usedit/internal/community"
)

// TestPostIdempotencyOwnershipRevisionAndDeletion 串联发布幂等、作者权限、并发修订和删除持久化。
func TestPostIdempotencyOwnershipRevisionAndDeletion(t *testing.T) {
	store := openStore(t)
	owner := newBrowser(store)
	body := postBody("第一次使用记录", "ens", []string{"链上身份"})
	headers := map[string]string{"idempotency-key": "publish-key-0001"}
	requireStatus(t, owner.request(t, "POST", "/posts", body, headers), 401)
	owner.login(t, newSigner(t))
	created := owner.request(t, "POST", "/posts", body, headers)
	requireStatus(t, created, 201)
	post := readJSON[struct{ Post community.Post }](t, created).Post
	replayed := owner.request(t, "POST", "/posts", body, headers)
	requireStatus(t, replayed, 201)
	if again := readJSON[struct{ Post community.Post }](t, replayed).Post; again.ID != post.ID || again.Revision != 1 {
		t.Fatalf("幂等重试创建了其他内容: %+v", again)
	}
	changed := postBody("不同的使用记录", "ens", []string{"链上身份"})
	conflict := owner.request(t, "POST", "/posts", changed, headers)
	requireStatus(t, conflict, 409)
	if errorCode(t, conflict) != "idempotency_conflict" || store.count(t, "SELECT count(*) FROM posts") != 1 {
		t.Fatal("相同标识的不同内容未被拒绝")
	}
	other := newBrowser(store)
	other.login(t, newSigner(t))
	if visible := other.getPost(t, post.ID); visible.Owned || visible.Kind != "review" {
		t.Fatal("其他账户的公开内容所有权不正确")
	}
	changed["revision"] = 1
	for _, method := range []string{"PATCH", "DELETE"} {
		requireStatus(t, other.request(t, method, "/posts/"+post.ID, changed), 403)
	}
	requireStatus(t, other.request(t, "POST", "/posts/"+post.ID+"/verify", map[string]string{"tx_hash": "0x" + strings.Repeat("1", 64)}), 403)
	otherPost := other.createPost(t, "publish-key-0001", "另一账户独立记录", "ens", []string{"域名"})
	if otherPost.ID == post.ID {
		t.Fatal("不同账户共用了幂等记录")
	}
	// 两个编辑者持有同一修订号时，数据库比较更新只能接受其中一份。
	start := make(chan struct{})
	responses := make(chan community.Response, 2)
	for index := 0; index < 2; index++ {
		concurrent := *owner
		update := postBody(fmt.Sprintf("并发编辑版本%d", index), "ens", []string{"域名"})
		update["revision"] = 1
		encoded, encodeErr := json.Marshal(update)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		go func() {
			<-start
			responses <- concurrent.raw("PATCH", "/posts/"+post.ID, string(encoded), nil)
		}()
	}
	close(start)
	winners, conflicts := 0, 0
	winnerTitle := ""
	for index := 0; index < 2; index++ {
		response := <-responses
		switch response.Status {
		case 200:
			winners++
			winnerTitle = readJSON[struct{ Post community.Post }](t, response).Post.Title
		case 409:
			conflicts++
		default:
			t.Fatalf("并发编辑返回 %d: %s", response.Status, response.Body)
		}
	}
	updated := owner.getPost(t, post.ID)
	if winners != 1 || conflicts != 1 || updated.Revision != 2 || updated.Title != winnerTitle {
		t.Fatalf("修订比较更新不正确: 成功%d 冲突%d 内容%+v", winners, conflicts, updated)
	}
	requireStatus(t, owner.request(t, "DELETE", "/posts/"+post.ID, nil), 200)
	requireStatus(t, owner.request(t, "GET", "/posts/"+post.ID, nil), 404)
	requireStatus(t, owner.request(t, "POST", "/posts", body, headers), 404)
	requireStatus(t, owner.request(t, "PATCH", "/posts/"+post.ID, changed), 404)
	if hasPost(owner.feed(t, "?tab=latest").Items, post.ID) {
		t.Fatal("删除后的内容重新出现在信息流")
	}
	mine := owner.request(t, "GET", "/me/posts", nil)
	requireStatus(t, mine, 200)
	if len(readJSON[struct{ Items []community.Post }](t, mine).Items) != 0 {
		t.Fatal("删除后的内容仍出现在个人发布列表")
	}
	if count := store.count(t, "SELECT count(*) FROM posts WHERE id=? AND deleted=1", post.ID); count != 1 {
		t.Fatal("删除标志未持久化")
	}
	selection := collectionBody("已删除内容精选", 0)
	selection["post_ids"] = []string{post.ID}
	requireStatus(t, owner.request(t, "POST", "/collections", selection), 400)
}

// TestFeedbackDeduplicatesAndOlderEventsDoNotOverrideLaterChoices 验证唯一事件不会重复累计或覆盖后续选择。
func TestFeedbackDeduplicatesAndOlderEventsDoNotOverrideLaterChoices(t *testing.T) {
	// 分别核对目标状态接口和唯一事件接口，二者都不能因重放重复累计。
	store := openStore(t)
	client := newBrowser(store)
	user := client.session(t)
	for index := 0; index < 3; index++ {
		requireStatus(t, client.request(t, "PUT", "/posts/reference-ens/helpful", map[string]bool{"active": true}), 200)
	}
	if count := client.getPost(t, "reference-ens").HelpfulCount; count != 1 {
		t.Fatalf("重复点赞累计为 %d", count)
	}
	for index := 0; index < 2; index++ {
		requireStatus(t, client.request(t, "PUT", "/posts/reference-ens/helpful", map[string]bool{"active": false}), 200)
	}
	if post := client.getPost(t, "reference-ens"); post.HelpfulCount != 0 || post.Helpful {
		t.Fatal("取消互动未落盘")
	}
	opened := map[string]any{"post_id": "reference-ens", "kind": "open", "event_id": "open-event-0001"}
	for index := 0; index < 2; index++ {
		requireStatus(t, client.request(t, "POST", "/feedback", opened), 200)
	}
	if count := store.count(t, "SELECT count(*) FROM events WHERE user_id=? AND event_id=?", user.ID, "open-event-0001"); count != 1 {
		t.Fatalf("重复事件入库 %d 次", count)
	}
	if count := store.count(t, "SELECT count(*) FROM actions WHERE user_id=? AND post_id=? AND kind='open'", user.ID, "reference-ens"); count != 1 {
		t.Fatalf("阅读反馈累计为 %d", count)
	}
	conflicting := map[string]any{"post_id": "reference-uniswap", "kind": "open", "event_id": "open-event-0001"}
	requireStatus(t, client.request(t, "POST", "/feedback", conflicting), 409)
	if count := store.count(t, "SELECT count(*) FROM actions WHERE user_id=? AND post_id=?", user.ID, "reference-uniswap"); count != 0 {
		t.Fatal("冲突事件修改了其他内容")
	}
	hidden := map[string]any{"post_id": "reference-ens", "kind": "hide", "event_id": "hide-event-0001", "active": true}
	requireStatus(t, client.request(t, "POST", "/feedback", hidden), 200)
	if hasPost(client.feed(t, "?tab=for-you").Items, "reference-ens") {
		t.Fatal("已排除内容仍在推荐中")
	}
	requireStatus(t, client.request(t, "POST", "/feedback", map[string]any{"post_id": "reference-ens", "kind": "hide", "event_id": "undo-event-0002", "active": false}), 200)
	requireStatus(t, client.request(t, "POST", "/feedback", hidden), 200)
	if !hasPost(client.feed(t, "?tab=for-you").Items, "reference-ens") {
		t.Fatal("旧隐藏事件重放覆盖了后续恢复选择")
	}
	for index := 0; index < 2; index++ {
		requireStatus(t, client.request(t, "POST", "/feedback", map[string]any{"post_id": "reference-ens", "kind": "impression", "event_id": "impression-event-0003"}), 200)
	}
	if count := store.count(t, "SELECT count(*) FROM events WHERE user_id=?", user.ID); count != 4 {
		t.Fatalf("反馈唯一事件计数不正确: %d", count)
	}
}

// TestRequestBodyMediaTypeAndOriginBoundaries 检查解析、媒体类型、体积和同源约束均先于业务写入。
func TestRequestBodyMediaTypeAndOriginBoundaries(t *testing.T) {
	store := openStore(t)
	client := newBrowser(store)
	client.session(t)
	for _, input := range []struct {
		name   string
		body   string
		status int
	}{
		{name: "null", body: " \nnull\t", status: 400},
		{name: "array", body: "[]", status: 400},
		{name: "string", body: `"名字"`, status: 400},
		{name: "empty", body: "", status: 400},
		{name: "unknown", body: `{"display_name":"有效昵称","user_id":"foreign"}`, status: 400},
		{name: "multiple", body: `{"display_name":"有效昵称"} {}`, status: 400},
		{name: "malformed", body: `{"display_name":`, status: 400},
		{name: "large", body: strings.Repeat(" ", 65537), status: 413},
	} {
		t.Run(input.name, func(t *testing.T) {
			requireStatus(t, client.raw("PATCH", "/profile", input.body, nil), input.status)
		})
	}
	for _, contentType := range []string{"", "text/plain", "application/jsonjunk", "application/json; charset", "application/json; charset=utf-8; charset=gbk"} {
		t.Run("media_"+contentType, func(t *testing.T) {
			requireStatus(t, client.raw("PATCH", "/profile", `{"display_name":"合法昵称"}`, map[string]string{"content-type": contentType}), 415)
		})
	}
	requireStatus(t, client.raw("PATCH", "/profile", `{"display_name":"合法昵称"}`, map[string]string{"content-type": "Application/JSON; charset=utf-8"}), 200)
	requireStatus(t, client.request(t, "PATCH", "/profile", map[string]string{"display_name": "新的名字"}, map[string]string{"x-usedit-request": ""}), 403)
	requireStatus(t, client.request(t, "PATCH", "/profile", map[string]string{"display_name": "新的名字"}, map[string]string{"origin": "https://unrelated.test"}), 403)
	if current := client.session(t); current.DisplayName != "合法昵称" {
		t.Fatal("被拒绝的请求改变了用户资料")
	}
	requireStatus(t, client.request(t, "PUT", "/posts/reference-ens/bookmark", map[string]any{}), 400)
	requireStatus(t, client.request(t, "PUT", "/preferences", map[string]any{"tags": []string{"app:ens"}}), 400)
}
