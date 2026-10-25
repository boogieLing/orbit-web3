// Package recommend 对有界候选进行可解释、确定性的内存排序。
package recommend

import (
	"log/slog"
	"sort"
	"strings"
	"time"
)

const MaxCandidates = 200

// Item 仅包含排序所需信息，不接收钱包余额、收益或身份画像。
type Item struct {
	ID        string
	AppID     string
	Tags      []string
	CreatedAt time.Time
	Verified  bool
	Helpful   int
	Kind      string
	Hidden    bool
}

// Ranked 的 Score 是当次选取时包含多样性扣分的排序分，不代表内容质量。
type Ranked struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
	Score  int    `json:"score"`
}

type candidate struct {
	item   Item
	score  int
	reason string
}

// Rank 对最多 200 条候选评分；调用方须提前过滤权限并批量读取反馈。
// 普通偏好对应明确兴趣标签，app: 前缀只应由服务端从真实收藏记录派生。
func Rank(items []Item, preferences []string, now time.Time) []Ranked {
	if len(items) > MaxCandidates {
		slog.Warn("推荐候选超过上限", "count", len(items), "limit", MaxCandidates)
		return []Ranked{}
	}
	interests, apps := preferenceSets(preferences)
	candidates := make([]candidate, 0, len(items))
	for _, item := range items {
		if item.Hidden || item.ID == "" || item.AppID == "" || (item.Kind != "review" && item.Kind != "reference") {
			continue
		}
		candidates = append(candidates, scoreItem(item, interests, apps, now))
	}
	// 先固定同分顺序再去重，输入顺序不会影响相同候选集的结果。
	sort.Slice(candidates, func(left, right int) bool { return better(candidates[left], candidates[right]) })
	candidates = uniqueCandidates(candidates)
	return diversify(candidates)
}

// preferenceSets 分离明确兴趣与服务端传入的收藏应用，并归一化去重。
func preferenceSets(preferences []string) (map[string]bool, map[string]bool) {
	// 内部应用偏好与公开标签分开存储，不能让应用标识参与标签交集。
	interests := make(map[string]bool)
	apps := make(map[string]bool)
	for _, preference := range preferences {
		value := strings.TrimSpace(preference)
		if strings.HasPrefix(value, "app:") {
			if id := strings.TrimSpace(strings.TrimPrefix(value, "app:")); id != "" {
				apps[id] = true
			}
			continue
		}
		if value != "" {
			interests[strings.ToLower(value)] = true
		}
	}
	return interests, apps
}

// scoreItem 按兴趣、新鲜度和有限反馈评分，并选择有事实依据的推荐理由。
func scoreItem(item Item, interests, apps map[string]bool, now time.Time) candidate {
	matches := matchingTags(item.Tags, interests)
	score := min(len(matches), 3) * 30
	reason := "发现不同应用的使用记录"
	if item.Kind == "reference" {
		reason = "来自应用资料的阅读笔记"
	}
	if len(matches) > 0 {
		reason = "符合你的兴趣：" + strings.Join(matches[:min(len(matches), 2)], "、")
	}
	if apps[item.AppID] {
		score += 20
		if len(matches) == 0 {
			reason = "来自你收藏过的应用"
		}
	}
	if !item.CreatedAt.IsZero() {
		age := max(now.Sub(item.CreatedAt), 0)
		freshness := max(20-int(age/(24*time.Hour)), 0)
		score += freshness
		if len(matches) == 0 && !apps[item.AppID] && age < 72*time.Hour {
			if item.Kind == "reference" {
				reason = "近期整理的应用资料笔记"
			} else {
				reason = "近期发布的使用体验"
			}
		}
	}
	// 凭据只说明一次交互发生，权重小于兴趣与新鲜度，不构成质量背书。
	if item.Verified && item.Kind == "review" {
		score += 3
	}
	score += min(max(item.Helpful, 0), 20) / 4
	return candidate{item: item, score: score, reason: reason}
}

// matchingTags 生成去重且有固定次序的兴趣交集，避免标签次序影响评分解释。
func matchingTags(tags []string, interests map[string]bool) []string {
	// 相同标签只加分一次，大小写和首尾空格不能制造额外匹配。
	set := make(map[string]bool)
	for _, tag := range tags {
		value := strings.ToLower(strings.TrimSpace(tag))
		if interests[value] {
			set[value] = true
		}
	}
	matched := make([]string, 0, len(set))
	for tag := range set {
		matched = append(matched, tag)
	}
	sort.Strings(matched)
	return matched
}

// better 用分数、发布时间和固定标识依次判定同分候选的稳定顺序。
func better(left, right candidate) bool {
	// 每一级都提供确定的平分规则，避免结果依赖数据库的偶然返回顺序。
	if left.score != right.score {
		return left.score > right.score
	}
	if !left.item.CreatedAt.Equal(right.item.CreatedAt) {
		return left.item.CreatedAt.After(right.item.CreatedAt)
	}
	if left.item.ID != right.item.ID {
		return left.item.ID < right.item.ID
	}
	if left.item.AppID != right.item.AppID {
		return left.item.AppID < right.item.AppID
	}
	return left.reason < right.reason
}

// uniqueCandidates 在排序后按内容标识去重，保留同一记录的优先候选。
func uniqueCandidates(candidates []candidate) []candidate {
	// 输入已经固定顺序，首次出现即为该内容标识最终保留的候选。
	seen := make(map[string]bool, len(candidates))
	unique := make([]candidate, 0, len(candidates))
	for _, item := range candidates {
		if !seen[item.item.ID] {
			seen[item.item.ID] = true
			unique = append(unique, item)
		}
	}
	return unique
}

// diversify 逐次加入有限的同应用扣分，增加相邻结果中的应用多样性。
func diversify(candidates []candidate) []Ranked {
	result := make([]Ranked, 0, len(candidates))
	appCounts := make(map[string]int)
	// 最多 200 个元素，逐次选择可直接表达应用重复惩罚，且没有逐条外部读取。
	for len(candidates) > 0 {
		best := 0
		bestScore := candidates[0].score - min(appCounts[candidates[0].item.AppID], 3)*18
		for index := 1; index < len(candidates); index++ {
			adjusted := candidates[index].score - min(appCounts[candidates[index].item.AppID], 3)*18
			if adjusted > bestScore {
				best, bestScore = index, adjusted
			}
		}
		chosen := candidates[best]
		result = append(result, Ranked{ID: chosen.item.ID, Reason: chosen.reason, Score: bestScore})
		appCounts[chosen.item.AppID]++
		candidates = append(candidates[:best], candidates[best+1:]...)
	}
	return result
}
