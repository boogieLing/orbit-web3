package community_test

import (
	"net/url"
	"strings"
	"testing"
	"usedit/internal/community"
)

// TestExperiencePublishingAndRecords 验证无钱包发布、记录归属、版本竞争与持久化。
func TestExperiencePublishingAndRecords(t *testing.T) {
	store := openStore(t)
	store.service.Config.ExperienceMode = true
	owner := newBrowser(store)
	user := owner.session(t)
	if !user.CanPublish || user.Wallet != nil {
		t.Fatal("社区身份权限或钱包语义错误")
	}
	post := owner.createPost(t, "experience-publish-1", "我的地址资料整理", "ens", []string{"域名"})
	duplicate := owner.createPost(t, "experience-publish-1", "我的地址资料整理", "ens", []string{"域名"})
	if duplicate.ID != post.ID {
		t.Fatal("重复提交创建了多篇内容")
	}
	path := "/posts/" + post.ID
	other := newBrowser(store)
	requireStatus(t, other.request(t, "POST", path+"/verify", map[string]string{"record_id": "ens-profile"}), 403)
	requireStatus(t, owner.request(t, "POST", path+"/verify", map[string]string{"record_id": "opensea-item"}), 400)
	requireStatus(t, owner.request(t, "POST", path+"/verify", map[string]string{"tx_hash": "0x" + strings.Repeat("1", 64)}), 400)
	attached := owner.request(t, "POST", path+"/verify", map[string]string{"record_id": "ens-profile"})
	requireStatus(t, attached, 200)
	value := readJSON[struct{ Post community.Post }](t, attached).Post
	if value.ProofStatus != "recorded" || value.TxHash != "" || value.Interaction == nil || value.Interaction.AppID != "ens" {
		t.Fatal("记录被误表示为链上核验")
	}
	// 记录关联推进修订号，旧编辑不得覆盖已关联结果。
	input := postBody("更新后的资料整理", "ens", []string{"域名"})
	input["revision"] = post.Revision
	requireStatus(t, owner.request(t, "PATCH", path, input), 409)
	input["revision"] = value.Revision
	requireStatus(t, owner.request(t, "PATCH", path, input), 200)
	if !hasPost(owner.feed(t, "?tab=verified").Items, post.ID) || !hasPost(owner.feed(t, "?q=ENS").Items, post.ID) {
		t.Fatal("记录筛选或应用搜索遗漏个人内容")
	}
	store.restart(t)
	store.service.Config.ExperienceMode = true
	restored := owner.getPost(t, post.ID)
	if restored.Title != "更新后的资料整理" || restored.Interaction == nil || !restored.Owned {
		t.Fatal("重启后未恢复内容与记录")
	}
	requireStatus(t, other.request(t, "DELETE", path, nil), 403)
	requireStatus(t, owner.request(t, "DELETE", path, nil), 200)
	requireStatus(t, owner.request(t, "GET", path, nil), 404)
}

// TestExperienceLibraryPagingAndFeedback 验证内容库可分页、可反馈且不赋予公共文章编辑权。
func TestExperienceLibraryPagingAndFeedback(t *testing.T) {
	store := openStore(t)
	store.service.Config.ExperienceMode = true
	owner := newBrowser(store)
	first := owner.feed(t, "?tab=latest")
	if len(first.Items) != 12 || first.NextCursor == nil {
		t.Fatal("完整内容库未按十二条分页")
	}
	second := owner.feed(t, "?tab=latest&cursor="+url.QueryEscape(*first.NextCursor))
	if len(second.Items) != 3 || second.NextCursor != nil {
		t.Fatal("末页内容不完整")
	}
	for _, post := range second.Items {
		if hasPost(first.Items, post.ID) {
			t.Fatal("分页重复")
		}
	}
	if len(owner.feed(t, "?tab=verified").Items) != 6 {
		t.Fatal("交互记录筛选不完整")
	}
	id := "story-ens-profile"
	requireStatus(t, owner.request(t, "DELETE", "/posts/"+id, nil), 403)
	requireStatus(t, owner.request(t, "PUT", "/posts/"+id+"/bookmark", map[string]bool{"active": true}), 200)
	requireStatus(t, owner.request(t, "PUT", "/posts/"+id+"/helpful", map[string]bool{"active": true}), 200)
	if !hasPost(owner.feed(t, "?tab=saved").Items, id) {
		t.Fatal("收藏未进入列表")
	}
	// 公共内容可共享，个人反馈不能影响另一份会话。
	other := newBrowser(store)
	if post := other.getPost(t, id); post.Bookmarked || post.Helpful || post.Owned {
		t.Fatal("个人状态跨会话泄漏")
	}
	requireStatus(t, owner.request(t, "POST", "/feedback", map[string]any{"post_id": id, "kind": "hide", "event_id": "experience-hide-1", "active": true}), 200)
	if hasPost(owner.feed(t, "").Items, id) {
		t.Fatal("不感兴趣未生效")
	}
	requireStatus(t, owner.request(t, "POST", "/feedback", map[string]any{"post_id": id, "kind": "hide", "event_id": "experience-hide-2", "active": false}), 200)
	if !hasPost(owner.feed(t, "").Items, id) {
		t.Fatal("撤销未恢复")
	}
}

// TestExperienceCollectionAdjustments 验证内置内容、页面调整、历史与导出走同一条保存链路。
func TestExperienceCollectionAdjustments(t *testing.T) {
	// 使用正式保存接口推进三个版本，再读历史与导出核对同一结果。
	store := openStore(t)
	store.service.Config.ExperienceMode = true
	owner := newBrowser(store)
	input := collectionBody("我的链上阅读清单", 0)
	input["post_ids"] = []string{"story-ens-profile", "story-opensea-item"}
	created := owner.request(t, "POST", "/collections", input)
	requireStatus(t, created, 201)
	first := readJSON[struct{ Collection community.Collection }](t, created).Collection
	path := "/collections/" + first.ID
	response := owner.request(t, "POST", path+"/compose", map[string]string{"instruction": "深色卡片；标题：周末工具箱；简介：留给下一次发现"})
	requireStatus(t, response, 200)
	second := readJSON[struct{ Collection community.Collection }](t, response).Collection
	if second.Version != 2 || second.Theme != "ink" || second.Layout != "grid" || second.Title != "周末工具箱" || second.Description != "留给下一次发现" || !strings.Contains(second.HTML, "把零散的地址") {
		t.Fatal("样式或内容未进入新版本")
	}
	old := owner.request(t, "GET", path+"/versions/1", nil)
	requireStatus(t, old, 200)
	if readJSON[struct{ Collection community.Collection }](t, old).Collection.HTML != first.HTML {
		t.Fatal("历史被覆盖")
	}
	requireStatus(t, owner.request(t, "POST", path+"/compose", map[string]string{"instruction": "加入一个未知功能"}), 422)
	if store.count(t, "SELECT count(*) FROM collection_versions WHERE collection_id=?", first.ID) != 2 {
		t.Fatal("无效要求产生了版本")
	}
	other := newBrowser(store)
	requireStatus(t, other.request(t, "POST", path+"/compose", map[string]string{"instruction": "清爽列表"}), 404)
	response = owner.request(t, "POST", path+"/compose", map[string]string{"instruction": "清爽列表"})
	requireStatus(t, response, 200)
	third := readJSON[struct{ Collection community.Collection }](t, response).Collection
	if third.Theme != "sage" || third.Layout != "editorial" || third.Version != 3 {
		t.Fatal("第二次页面调整失败")
	}
	exported := owner.request(t, "GET", path+"/export", nil)
	requireStatus(t, exported, 200)
	if exported.Body != third.HTML {
		t.Fatal("导出不是当前版本")
	}
	store.restart(t)
	store.service.Config.ExperienceMode = true
	current := owner.request(t, "GET", path, nil)
	requireStatus(t, current, 200)
	if readJSON[struct{ Collection community.Collection }](t, current).Collection.HTML != third.HTML {
		t.Fatal("重启后精选未保留")
	}
}

// TestExperienceBoundaryCanBeDisabled 验证环境开关不弱化原有钱包发布要求。
func TestExperienceBoundaryCanBeDisabled(t *testing.T) {
	// 先建立可写会话，再关闭运行开关，确认旧会话不能保留额外权限。
	store := openStore(t)
	store.service.Config.ExperienceMode = true
	owner := newBrowser(store)
	post := owner.createPost(t, "experience-boundary-1", "环境边界检查", "ens", nil)
	store.service.Config.ExperienceMode = false
	if owner.session(t).CanPublish {
		t.Fatal("关闭后仍允许游客发布")
	}
	requireStatus(t, owner.request(t, "POST", "/posts", postBody("新的发布内容", "ens", nil), map[string]string{"idempotency-key": "experience-boundary-2"}), 401)
	requireStatus(t, owner.request(t, "POST", "/posts/"+post.ID+"/verify", map[string]string{"record_id": "ens-profile"}), 403)
	requireStatus(t, owner.request(t, "GET", "/posts/story-ens-profile", nil), 404)
	for _, item := range owner.feed(t, "").Items {
		if strings.HasPrefix(item.ID, "story-") {
			t.Fatal("关闭后仍展示内置内容")
		}
	}
}
