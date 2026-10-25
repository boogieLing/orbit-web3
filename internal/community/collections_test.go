package community_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"usedit/internal/community"
)

// TestCollectionVersionsAreAtomicAndPrivate 验证所有权、导出隔离、不可变历史、事务回滚和并发版本。
func TestCollectionVersionsAreAtomicAndPrivate(t *testing.T) {
	store := openStore(t)
	owner := newBrowser(store)
	initial := owner.createCollection(t, "身份资料精选")
	if initial.Version != 1 || !strings.Contains(initial.HTML, "资料笔记") || !strings.Contains(initial.HTML, "ENS") {
		t.Fatalf("初始精选页不完整: %+v", initial)
	}
	path := "/collections/" + initial.ID
	other := newBrowser(store)
	for _, suffix := range []string{"", "/versions/1", "/export"} {
		requireStatus(t, other.request(t, "GET", path+suffix, nil), 404)
	}
	requireStatus(t, other.request(t, "PATCH", path, collectionBody("无权修改", 1)), 404)
	requireStatus(t, other.request(t, "POST", path+"/compose", map[string]string{"instruction": "调整布局"}), 404)
	response := owner.request(t, "PATCH", path, collectionBody("第二版资料精选", 1))
	requireStatus(t, response, 200)
	second := readJSON[struct{ Collection community.Collection }](t, response).Collection
	if second.Version != 2 || second.CreatedAt != initial.CreatedAt || second.HTML == initial.HTML {
		t.Fatal("保存未生成正确的新版本")
	}
	conflict := owner.request(t, "PATCH", path, collectionBody("过期编辑", 1))
	requireStatus(t, conflict, 409)
	firstVersion := owner.request(t, "GET", path+"/versions/1", nil)
	requireStatus(t, firstVersion, 200)
	if original := readJSON[struct{ Collection community.Collection }](t, firstVersion).Collection; original.HTML != initial.HTML || original.Title != initial.Title {
		t.Fatal("保存新版本改变了历史正文")
	}
	exported := owner.request(t, "GET", path+"/export", nil)
	requireStatus(t, exported, 200)
	if exported.Body != second.HTML || !strings.HasPrefix(exported.Headers["content-type"], "text/html") || !strings.Contains(exported.Headers["content-disposition"], "attachment") || !strings.Contains(exported.Headers["content-security-policy"], "sandbox allow-scripts") {
		t.Fatal("导出正文或隔离策略不正确")
	}
	requireStatus(t, owner.request(t, "POST", path+"/compose", map[string]string{"instruction": "重新安排页面"}), 503)
	// 让真实数据库拒绝历史版本写入，验证当前版本与历史会一起回滚。
	if _, triggerErr := store.db.Conn.ExecContext(context.Background(), `CREATE TRIGGER reject_third_version BEFORE INSERT ON collection_versions WHEN NEW.version=3 BEGIN SELECT RAISE(ABORT, 'history unavailable'); END`); triggerErr != nil {
		t.Fatal(triggerErr)
	}
	requireStatus(t, owner.request(t, "PATCH", path, collectionBody("第三版资料精选", 2)), 500)
	current := owner.request(t, "GET", path, nil)
	requireStatus(t, current, 200)
	if unchanged := readJSON[struct{ Collection community.Collection }](t, current).Collection; unchanged.Version != 2 || unchanged.HTML != second.HTML {
		t.Fatal("历史保存失败后当前版本未回滚")
	}
	if count := store.count(t, "SELECT count(*) FROM collection_versions WHERE collection_id=?", initial.ID); count != 2 {
		t.Fatalf("失败保存留下部分版本: %d", count)
	}
	if _, dropErr := store.db.Conn.ExecContext(context.Background(), "DROP TRIGGER reject_third_version"); dropErr != nil {
		t.Fatal(dropErr)
	}
	requireStatus(t, owner.request(t, "PATCH", path, collectionBody("第三版资料精选", 2)), 200)
	start := make(chan struct{})
	responses := make(chan community.Response, 2)
	for _, title := range []string{"并发保存甲", "并发保存乙"} {
		concurrent := *owner
		encoded, encodeErr := json.Marshal(collectionBody(title, 3))
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		go func() {
			<-start
			responses <- concurrent.raw("PATCH", path, string(encoded), nil)
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
			winnerTitle = readJSON[struct{ Collection community.Collection }](t, response).Collection.Title
		case 409:
			conflicts++
		default:
			t.Fatalf("并发保存状态异常: %d %s", response.Status, response.Body)
		}
	}
	if winners != 1 || conflicts != 1 || store.count(t, "SELECT count(*) FROM collection_versions WHERE collection_id=?", initial.ID) != 4 {
		t.Fatal("并发保存未保持一个当前版本和连续历史")
	}
	store.restart(t)
	current = owner.request(t, "GET", path, nil)
	requireStatus(t, current, 200)
	final := readJSON[struct{ Collection community.Collection }](t, current).Collection
	if final.Version != 4 || final.Title != winnerTitle {
		t.Fatal("重启后未保留胜出的版本")
	}
	history := owner.request(t, "GET", path+"/versions/4", nil)
	requireStatus(t, history, 200)
	if historical := readJSON[struct{ Collection community.Collection }](t, history).Collection; historical.HTML != final.HTML || historical.Title != final.Title {
		t.Fatal("当前页与同版本历史正文不一致")
	}
}

// TestCollectionCreateRollsBackWhenHistoryCannotBeSaved 验证初始版本保存失败不会留下半成品。
func TestCollectionCreateRollsBackWhenHistoryCannotBeSaved(t *testing.T) {
	// 只让历史表插入失败，证明主表插入与历史写入处在同一真实事务中。
	store := openStore(t)
	client := newBrowser(store)
	client.session(t)
	if _, triggerErr := store.db.Conn.ExecContext(context.Background(), `CREATE TRIGGER reject_first_version BEFORE INSERT ON collection_versions BEGIN SELECT RAISE(ABORT, 'history unavailable'); END`); triggerErr != nil {
		t.Fatal(triggerErr)
	}
	requireStatus(t, client.request(t, "POST", "/collections", collectionBody("首次创建资料页", 0)), 500)
	if count := store.count(t, "SELECT count(*) FROM collections"); count != 0 {
		t.Fatalf("初始历史写入失败留下 %d 个精选页", count)
	}
	if count := store.count(t, "SELECT count(*) FROM collection_versions"); count != 0 {
		t.Fatal("失败创建留下历史记录")
	}
}
