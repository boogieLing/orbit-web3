package community

import (
	"encoding/base64"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
	"usedit/internal/catalog"
	"usedit/internal/database"
	"usedit/internal/recommend"
)

type feedItem struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// feed 建立短期排序快照，翻页只刷新可见性与互动状态。
func (s *Service) feed(c *call) (int, any, error) {
	tab := c.url.Query().Get("tab")
	if tab == "" {
		tab = "for-you"
	}
	category := c.url.Query().Get("category")
	if category == "" {
		category = "all"
	}
	search := strings.TrimSpace(c.url.Query().Get("q"))
	if tab != "for-you" && tab != "latest" && tab != "verified" && tab != "saved" {
		return 0, nil, fail(400, "tab", "内容分类不存在")
	}
	if category != "all" && category != "identity" && category != "exchange" && category != "collectibles" {
		return 0, nil, fail(400, "category", "应用类别不存在")
	}
	if !validText(search, 0, 80) {
		return 0, nil, fail(400, "search", "搜索内容过长")
	}
	filterHash := digest(encode([]string{tab, category, search}))
	cursor := c.url.Query().Get("cursor")
	if cursor != "" {
		return s.feedPage(c, cursor, filterHash)
	}
	refs := s.libraryPosts()
	where := ""
	args := []any{}
	if category != "all" {
		ids := []string{}
		for _, app := range catalog.All() {
			if app.Category == category {
				ids = append(ids, app.ID)
			}
		}
		if len(ids) > 0 {
			holders := make([]string, len(ids))
			for i, id := range ids {
				holders[i] = "?"
				args = append(args, id)
			}
			where += " AND p.app_id IN (" + strings.Join(holders, ",") + ")"
		} else {
			where += " AND 1=0"
		}
	}
	if search != "" {
		where += " AND (p.title LIKE ? ESCAPE '\\' OR p.body LIKE ? ESCAPE '\\' OR p.app_id=?)"
		escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(search)
		appID := ""
		for _, app := range catalog.All() {
			if strings.EqualFold(app.Name, search) {
				appID = app.ID
			}
		}
		args = append(args, "%"+escaped+"%", "%"+escaped+"%", appID)
	}
	if tab == "verified" {
		if s.Config.ExperienceMode {
			where += " AND p.proof_status IN ('verified','recorded')"
		} else {
			where += " AND p.proof_status='verified'"
		}
	}
	if tab == "saved" {
		where += " AND EXISTS(SELECT 1 FROM actions a WHERE a.user_id=? AND a.post_id=p.id AND a.kind='bookmark')"
		args = append(args, c.user.ID)
	}
	posts, loadErr := s.loadPosts(c, where, args, 200-len(refs))
	if loadErr != nil {
		return 0, nil, loadErr
	}
	for _, note := range refs {
		app, _ := catalog.Get(note.AppID)
		if category != "all" && app.Category != category {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(note.Title+" "+note.Body+" "+app.Name+" "+strings.Join(note.Tags, " ")), strings.ToLower(search)) {
			continue
		}
		if tab != "verified" || note.ProofStatus == "recorded" {
			posts = append(posts, note)
		}
	}
	hydrated, hydrateErr := s.hydrate(c, posts)
	if hydrateErr != nil {
		return 0, nil, hydrateErr
	}
	posts = []Post{}
	for _, post := range hydrated {
		if post.Hidden && tab != "saved" {
			continue
		}
		if tab == "saved" && !post.Bookmarked {
			continue
		}
		posts = append(posts, post)
	}
	items := []feedItem{}
	if tab == "for-you" {
		preferences := append([]string{}, c.user.Preferences...)
		candidates := make([]recommend.Item, 0, len(posts))
		for _, post := range posts {
			if post.Bookmarked {
				preferences = append(preferences, "app:"+post.AppID)
			}
			created, _ := time.Parse(time.RFC3339, post.CreatedAt)
			candidates = append(candidates, recommend.Item{ID: post.ID, AppID: post.AppID, Tags: post.Tags, CreatedAt: created, Verified: post.ProofStatus == "verified", Helpful: post.HelpfulCount, Kind: post.Kind, Hidden: post.Hidden})
		}
		for _, ranked := range recommend.Rank(candidates, preferences, c.now) {
			items = append(items, feedItem{ID: ranked.ID, Reason: ranked.Reason})
		}
	} else {
		sort.Slice(posts, func(i, j int) bool {
			if posts[i].CreatedAt == posts[j].CreatedAt {
				return posts[i].ID < posts[j].ID
			}
			return posts[i].CreatedAt > posts[j].CreatedAt
		})
		for _, post := range posts {
			items = append(items, feedItem{ID: post.ID})
		}
	}
	snapshot, idErr := randomID(18)
	if idErr != nil {
		return 0, nil, idErr
	}
	// 清理只扫描各过期索引的前 100 条，避免请求中积累无界维护工作。
	_, saveErr := s.DB.Batch(c.ctx, []database.Statement{{SQL: "INSERT INTO feed_snapshots(id,user_id,filter_hash,items,expires) VALUES(?,?,?,?,?)", Args: []any{snapshot, c.user.ID, filterHash, encode(items), c.now.Add(15 * time.Minute).Unix()}}, {SQL: "DELETE FROM feed_snapshots WHERE id IN (SELECT id FROM feed_snapshots WHERE expires<? ORDER BY expires LIMIT 100)", Args: []any{c.now.Unix()}}, {SQL: "DELETE FROM rates WHERE key IN (SELECT key FROM rates WHERE expires<? ORDER BY expires LIMIT 100)", Args: []any{c.now.Unix()}}, {SQL: "DELETE FROM challenges WHERE id IN (SELECT id FROM challenges WHERE expires<? ORDER BY expires LIMIT 100)", Args: []any{c.now.Unix()}}})
	if saveErr != nil {
		return 0, nil, saveErr
	}
	return s.feedPage(c, encodeCursor(snapshot, 0), filterHash)
}

// encodeCursor 只编码随机快照标识和位置，所有权在读取时校验。
func encodeCursor(id string, offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(id + ":" + strconv.Itoa(offset)))
}

// feedPage 批量读取快照页，排除后来删除或屏蔽的内容。
func (s *Service) feedPage(c *call, cursor, filterHash string) (int, any, error) {
	if len(cursor) > 150 {
		return 0, nil, fail(400, "cursor", "分页标识无效")
	}
	raw, decodeErr := base64.RawURLEncoding.DecodeString(cursor)
	if decodeErr != nil {
		return 0, nil, fail(400, "cursor", "分页标识无效")
	}
	parts := strings.Split(string(raw), ":")
	if len(parts) != 2 {
		return 0, nil, fail(400, "cursor", "分页标识无效")
	}
	offset, offsetErr := strconv.Atoi(parts[1])
	if offsetErr != nil || offset < 0 || offset > 200 {
		return 0, nil, fail(400, "cursor", "分页位置无效")
	}
	// 游标可读不代表可用，还需匹配身份、筛选条件和生命周期。
	rows, queryErr := s.query(c.ctx, "SELECT items FROM feed_snapshots WHERE id=? AND user_id=? AND filter_hash=? AND expires>?", parts[0], c.user.ID, filterHash, c.now.Unix())
	if queryErr != nil {
		return 0, nil, queryErr
	}
	if len(rows) != 1 {
		return 0, nil, fail(410, "feed_expired", "这次推荐已过期，请刷新列表")
	}
	items := []feedItem{}
	if parseErr := json.Unmarshal([]byte(database.String(rows[0], "items")), &items); parseErr != nil {
		return 0, nil, fail(500, "feed_data", "推荐记录读取失败")
	}
	if offset > len(items) {
		return 0, nil, fail(400, "cursor", "分页位置无效")
	}
	end := offset + 12
	if end > len(items) {
		end = len(items)
	}
	page := items[offset:end]
	if len(page) == 0 {
		return 200, map[string]any{"items": []Post{}, "next_cursor": nil}, nil
	}
	holders := make([]string, len(page))
	args := make([]any, len(page))
	byID := map[string]Post{}
	for _, post := range s.libraryPosts() {
		byID[post.ID] = post
	}
	for i, item := range page {
		holders[i] = "?"
		args[i] = item.ID
	}
	posts, loadErr := s.loadPosts(c, "AND p.id IN ("+strings.Join(holders, ",")+")", args, len(page))
	if loadErr != nil {
		return 0, nil, loadErr
	}
	for _, post := range posts {
		byID[post.ID] = post
	}
	ordered := []Post{}
	for _, item := range page {
		post, ok := byID[item.ID]
		if ok {
			post.Reason = item.Reason
			ordered = append(ordered, post)
		}
	}
	// 快照固定次序，实时状态决定是否仍可展示，避免已删除或取消收藏内容复活。
	hydrated, hydrateErr := s.hydrate(c, ordered)
	if hydrateErr != nil {
		return 0, nil, hydrateErr
	}
	result := []Post{}
	for _, post := range hydrated {
		if c.url.Query().Get("tab") == "saved" && !post.Bookmarked {
			continue
		}
		if c.url.Query().Get("tab") == "verified" && post.ProofStatus != "verified" && !(s.Config.ExperienceMode && post.ProofStatus == "recorded") {
			continue
		}
		if !post.Hidden || c.url.Query().Get("tab") == "saved" {
			result = append(result, post)
		}
	}
	var next *string
	if end < len(items) {
		value := encodeCursor(parts[0], end)
		next = &value
	}
	return 200, map[string]any{"items": result, "next_cursor": next}, nil
}
