package recommend

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// referenceTime 固定评分时钟，保证新鲜度断言不会随执行日期变化。
func referenceTime() time.Time { return time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC) }

// TestExplicitInterestOutweighsPopularity 验证明确兴趣优先于有限互动加分。
func TestExplicitInterestOutweighsPopularity(t *testing.T) {
	now := referenceTime()
	items := []Item{
		{ID: "popular", AppID: "exchange", Tags: []string{"交易"}, CreatedAt: now, Helpful: 999999, Verified: true, Kind: "review"},
		{ID: "matching", AppID: "identity", Tags: []string{"身份"}, CreatedAt: now.Add(-7 * 24 * time.Hour), Kind: "review"},
	}
	ranked := Rank(items, []string{"身份"}, now)
	if len(ranked) != 2 || ranked[0].ID != "matching" || ranked[0].Reason != "符合你的兴趣：身份" {
		t.Fatalf("interest ranking: %+v", ranked)
	}
}

// TestSavedApplicationPreferenceIsExplained 验证收藏应用偏好加分并给出相符理由。
func TestSavedApplicationPreferenceIsExplained(t *testing.T) {
	now := referenceTime()
	items := []Item{
		{ID: "a", AppID: "new", CreatedAt: now, Kind: "review"},
		{ID: "b", AppID: "saved-app", CreatedAt: now, Kind: "reference"},
	}
	ranked := Rank(items, []string{"app:saved-app"}, now)
	if ranked[0].ID != "b" || ranked[0].Reason != "来自你收藏过的应用" {
		t.Fatalf("saved app ranking: %+v", ranked)
	}
}

// TestHiddenAndInvalidCandidatesExcluded 验证隐藏、无标识和非法类型候选被排除。
func TestHiddenAndInvalidCandidatesExcluded(t *testing.T) {
	// 把有效候选与各类应排除项混合，确认过滤不会牺牲合法结果。
	items := []Item{
		{ID: "visible", AppID: "app", Kind: "review"},
		{ID: "hidden", AppID: "app", Kind: "review", Hidden: true},
		{ID: "unknown", AppID: "app", Kind: "advertisement"},
		{AppID: "app", Kind: "review"},
		{ID: "missing-app", Kind: "reference"},
		{ID: "visible", AppID: "app", Kind: "review"},
	}
	ranked := Rank(items, nil, referenceTime())
	if len(ranked) != 1 || ranked[0].ID != "visible" {
		t.Fatalf("candidate exclusion: %+v", ranked)
	}
}

// TestRankingStableAndInputUnchanged 检查输入重排不改变结果且不会原地修改调用方数据。
func TestRankingStableAndInputUnchanged(t *testing.T) {
	// 打乱相同候选集合后再次排序，同时对照原输入检查副作用。
	now := referenceTime()
	items := []Item{
		{ID: "c", AppID: "one", Tags: []string{"身份", "支付"}, CreatedAt: now, Kind: "review"},
		{ID: "a", AppID: "two", Tags: []string{"支付", "身份"}, CreatedAt: now, Kind: "review"},
		{ID: "b", AppID: "three", Tags: []string{"身份"}, CreatedAt: now, Kind: "review"},
	}
	copyItems := append([]Item(nil), items...)
	first := Rank(items, []string{"身份", "支付", "身份"}, now)
	if !reflect.DeepEqual(items, copyItems) {
		t.Fatal("ranking mutated input order")
	}
	reversed := []Item{items[2], items[1], items[0]}
	second := Rank(reversed, []string{"支付", "身份"}, now)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("input order changed decisions: %+v %+v", first, second)
	}
	if first[0].ID != "a" {
		t.Fatalf("stable ID tie break not applied: %+v", first)
	}
}

// TestApplicationDiversity 验证相近分数下不会让同一应用连续占据全部前列。
func TestApplicationDiversity(t *testing.T) {
	// 同应用候选初始分接近，用第二个应用观察重复扣分是否实际改变前列。
	now := referenceTime()
	items := []Item{
		{ID: "a", AppID: "same", CreatedAt: now, Kind: "review"},
		{ID: "b", AppID: "same", CreatedAt: now, Kind: "review"},
		{ID: "c", AppID: "other", CreatedAt: now.Add(-time.Hour), Kind: "review"},
	}
	ranked := Rank(items, nil, now)
	if len(ranked) != 3 || ranked[0].ID != "a" || ranked[1].ID != "c" || ranked[2].ID != "b" {
		t.Fatalf("application variety: %+v", ranked)
	}
	if ranked[2].Score != 2 {
		t.Fatalf("selection score should include repetition cost: %+v", ranked[2])
	}
}

// TestHelpfulCapAndEvidenceWeight 检查有用数封顶和凭据的小权重边界。
func TestHelpfulCapAndEvidenceWeight(t *testing.T) {
	// 将极端互动数与封顶值比较，证明数量增长不能无限推高分数。
	now := referenceTime()
	base := Item{ID: "same", AppID: "app", CreatedAt: now, Kind: "review"}
	baseScore := Rank([]Item{base}, nil, now)[0].Score
	base.Helpful = 20
	boundedScore := Rank([]Item{base}, nil, now)[0].Score
	base.Helpful = 1000000
	unboundedScore := Rank([]Item{base}, nil, now)[0].Score
	if boundedScore != unboundedScore || boundedScore-baseScore != 5 {
		t.Fatalf("helpful cap: base=%d capped=%d large=%d", baseScore, boundedScore, unboundedScore)
	}
	base.Helpful = -100
	base.Verified = true
	if score := Rank([]Item{base}, nil, now)[0].Score; score-baseScore != 3 {
		t.Fatalf("evidence weight: %d", score-baseScore)
	}
	base.Kind = "reference"
	if score := Rank([]Item{base}, nil, now)[0].Score; score != baseScore {
		t.Fatalf("reference received evidence weight: %d", score)
	}
}

// TestFreshnessAndReferenceReasons 验证新鲜度衰减以及资料笔记的准确理由。
func TestFreshnessAndReferenceReasons(t *testing.T) {
	// 固定当前时间并分开检查旧内容、近期内容和资料类型，保证解释与事实一致。
	now := referenceTime()
	items := []Item{
		{ID: "old", AppID: "old-app", CreatedAt: now.Add(-90 * 24 * time.Hour), Kind: "review"},
		{ID: "recent", AppID: "recent-app", CreatedAt: now, Kind: "reference"},
	}
	ranked := Rank(items, nil, now)
	if ranked[0].ID != "recent" || !strings.Contains(ranked[0].Reason, "资料笔记") || ranked[1].Score != 0 {
		t.Fatalf("freshness: %+v", ranked)
	}
	future := items[1]
	future.CreatedAt = now.Add(24 * time.Hour)
	if Rank([]Item{future}, nil, now)[0].Score != ranked[0].Score {
		t.Fatal("future timestamp gained extra freshness")
	}
}

// TestCandidateLimit 检查排序严格拒绝超过候选上限的调用。
func TestCandidateLimit(t *testing.T) {
	// 直接传入超出上限的候选集，排序器须拒绝而非默默截断制造不稳定结果。
	if ranked := Rank(make([]Item, MaxCandidates+1), nil, referenceTime()); len(ranked) != 0 {
		t.Fatal("candidate limit not enforced")
	}
	if ranked := Rank(nil, nil, referenceTime()); ranked == nil || len(ranked) != 0 {
		t.Fatalf("empty ranking should be an empty slice: %+v", ranked)
	}
}
