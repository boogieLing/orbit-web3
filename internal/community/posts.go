package community

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"usedit/internal/catalog"
	"usedit/internal/database"
	"usedit/internal/proof"
)

var txPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)
var keyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{8,100}$`)

type postInput struct {
	AppID     string   `json:"app_id"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Sentiment string   `json:"sentiment"`
	Tags      []string `json:"tags"`
	TxHash    string   `json:"tx_hash"`
	Revision  int      `json:"revision"`
}

// validatePost 统一创建和编辑的内容限制。
func validatePost(input *postInput) error {
	input.Title = strings.TrimSpace(input.Title)
	input.Body = strings.TrimSpace(input.Body)
	if !validText(input.Title, 2, 120) || !validText(input.Body, 20, 5000) {
		return fail(400, "post_content", "标题需为 2–120 字，正文需为 20–5000 字")
	}
	if input.Sentiment != "positive" && input.Sentiment != "neutral" && input.Sentiment != "negative" {
		return fail(400, "sentiment", "请选择体验感受")
	}
	if len(input.Tags) > 5 {
		return fail(400, "tags", "最多添加 5 个标签")
	}
	// 标签归一化后去重，防止重复词放大推荐权重。
	seen := map[string]bool{}
	tags := []string{}
	for _, tag := range input.Tags {
		tag = strings.TrimSpace(tag)
		if !validText(tag, 1, 20) || strings.Contains(tag, ":") {
			return fail(400, "tags", "标签需为 1–20 个字符")
		}
		if !seen[tag] {
			tags = append(tags, tag)
			seen[tag] = true
		}
	}
	input.Tags = tags
	if input.TxHash != "" && !txPattern.MatchString(input.TxHash) {
		return fail(400, "tx_hash", "交易哈希格式不正确")
	}
	return nil
}

// createPost 使用账户内幂等键避免重试产生重复文章。
func (s *Service) createPost(c *call) (int, any, error) {
	if c.user.Wallet == nil {
		return 0, nil, fail(401, "wallet_required", "连接钱包并完成签名后即可发布，当前草稿会保留")
	}
	var input postInput
	if parseErr := decode(c, &input); parseErr != nil {
		return 0, nil, parseErr
	}
	if validateErr := validatePost(&input); validateErr != nil {
		return 0, nil, validateErr
	}
	if _, ok := catalog.Get(input.AppID); !ok {
		return 0, nil, fail(400, "app", "请选择目录中的应用")
	}
	key := c.req.Headers["idempotency-key"]
	if !keyPattern.MatchString(key) {
		return 0, nil, fail(400, "idempotency_key", "缺少有效的提交标识")
	}
	// 唯一键保证一次写入，摘要区分重试与复用旧键提交新内容。
	hash := digest(encode(input))
	id, idErr := randomID(18)
	if idErr != nil {
		return 0, nil, idErr
	}
	now := c.now.Format(time.RFC3339)
	_, insertErr := s.query(c.ctx, "INSERT INTO posts(id,user_id,app_id,title,body,sentiment,tags,tx_hash,created_at,updated_at,idempotency_key,content_hash) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(user_id,idempotency_key) DO NOTHING", id, c.user.ID, input.AppID, input.Title, input.Body, input.Sentiment, encode(input.Tags), input.TxHash, now, now, key, hash)
	if insertErr != nil {
		return 0, nil, insertErr
	}
	rows, queryErr := s.query(c.ctx, "SELECT id,content_hash FROM posts WHERE user_id=? AND idempotency_key=?", c.user.ID, key)
	if queryErr != nil {
		return 0, nil, queryErr
	}
	if database.String(rows[0], "content_hash") != hash {
		return 0, nil, fail(409, "idempotency_conflict", "提交标识已用于其他内容，请重新发布")
	}
	post, postErr := s.getPost(c, database.String(rows[0], "id"))
	return 201, map[string]any{"post": post}, postErr
}

// rowPost 映射持久化内容，不向浏览器泄漏内部作者标识。
func rowPost(row database.Row, userID string) Post {
	var address *string
	if value := database.String(row, "wallet"); value != "" {
		address = &value
	}
	return Post{ID: database.String(row, "id"), Kind: "review", AppID: database.String(row, "app_id"), Title: database.String(row, "title"), Body: database.String(row, "body"), Sentiment: database.String(row, "sentiment"), Tags: list(database.String(row, "tags")), Author: Author{Name: database.String(row, "display_name"), Wallet: address}, ProofStatus: database.String(row, "proof_status"), ProofMessage: database.String(row, "proof_message"), TxHash: database.String(row, "tx_hash"), ProofCheckedAt: database.String(row, "proof_checked_at"), CreatedAt: database.String(row, "created_at"), UpdatedAt: database.String(row, "updated_at"), Revision: database.Int(row, "revision"), Owned: database.String(row, "user_id") == userID}
}

// references 返回来源明确的资料条目，绝不伪装成用户体验。
func references() []Post {
	result := []Post{}
	for _, note := range catalog.References() {
		result = append(result, Post{ID: note.ID, Kind: "reference", AppID: note.AppID, Title: note.Title, Body: note.Body, Sentiment: "neutral", Tags: note.Tags, Author: Author{Name: note.SourceName}, ProofStatus: "none", CreatedAt: note.CheckedAt, UpdatedAt: note.CheckedAt, Revision: 1, SourceURL: note.SourceURL})
	}
	return result
}

// hydrate 用一次聚合查询填充一批文章的互动状态。
func (s *Service) hydrate(c *call, posts []Post) ([]Post, error) {
	if len(posts) == 0 {
		return posts, nil
	}
	// 循环只构建参数；全部互动状态一次聚合读取。
	placeholders := make([]string, len(posts))
	args := []any{c.user.ID, c.user.ID, c.user.ID}
	for i, post := range posts {
		placeholders[i] = "?"
		args = append(args, post.ID)
	}
	query := "SELECT post_id, SUM(kind='helpful') AS helpful_count, MAX(user_id=? AND kind='bookmark') AS bookmarked, MAX(user_id=? AND kind='helpful') AS helpful, MAX(user_id=? AND kind='hide') AS hidden FROM actions WHERE post_id IN (" + strings.Join(placeholders, ",") + ") GROUP BY post_id"
	rows, queryErr := s.query(c.ctx, query, args...)
	if queryErr != nil {
		return nil, queryErr
	}
	byID := map[string]database.Row{}
	for _, row := range rows {
		byID[database.String(row, "post_id")] = row
	}
	for i := range posts {
		row := byID[posts[i].ID]
		posts[i].HelpfulCount = database.Int(row, "helpful_count")
		posts[i].Bookmarked = database.Int(row, "bookmarked") != 0
		posts[i].Helpful = database.Int(row, "helpful") != 0
		posts[i].Hidden = database.Int(row, "hidden") != 0
	}
	return posts, nil
}

// loadPosts 限制候选数量并一次查询作者关联。
func (s *Service) loadPosts(c *call, where string, args []any, limit int) ([]Post, error) {
	// where 仅由服务端拼装，用户输入进入绑定参数，limit 限制单次候选集。
	args = append(args, limit)
	rows, queryErr := s.query(c.ctx, "SELECT p.*,u.display_name,u.wallet FROM posts p JOIN users u ON u.id=p.user_id WHERE p.deleted=0 "+where+" ORDER BY p.created_at DESC,p.id LIMIT ?", args...)
	if queryErr != nil {
		return nil, queryErr
	}
	result := []Post{}
	for _, row := range rows {
		result = append(result, rowPost(row, c.user.ID))
	}
	return result, nil
}

// getPost 获取一个可见条目，删除后的内容不会从旧推荐快照复活。
func (s *Service) getPost(c *call, id string) (Post, error) {
	// 资料条目与用户内容共用互动层，但资料来源始终独立标记。
	for _, post := range references() {
		if post.ID == id {
			hydrated, hydrateErr := s.hydrate(c, []Post{post})
			if hydrateErr != nil {
				return Post{}, hydrateErr
			}
			return hydrated[0], nil
		}
	}
	posts, loadErr := s.loadPosts(c, "AND p.id=?", []any{id}, 1)
	if loadErr != nil {
		return Post{}, loadErr
	}
	if len(posts) == 0 {
		return Post{}, fail(404, "post_not_found", "这篇内容已不存在")
	}
	hydrated, hydrateErr := s.hydrate(c, posts)
	if hydrateErr != nil {
		return Post{}, hydrateErr
	}
	return hydrated[0], nil
}

// myPosts 只返回当前身份自己的最新内容。
func (s *Service) myPosts(c *call) (int, any, error) {
	posts, loadErr := s.loadPosts(c, "AND p.user_id=?", []any{c.user.ID}, 50)
	if loadErr != nil {
		return 0, nil, loadErr
	}
	items, hydrateErr := s.hydrate(c, posts)
	return 200, map[string]any{"items": items}, hydrateErr
}

// postRoute 统一所有权校验，阅读与互动不要求作者身份。
func (s *Service) postRoute(c *call, parts []string) (int, any, error) {
	if len(parts) > 2 || parts[0] == "" {
		return 0, nil, fail(404, "not_found", "内容不存在")
	}
	post, loadErr := s.getPost(c, parts[0])
	if loadErr != nil {
		return 0, nil, loadErr
	}
	if len(parts) == 2 && (parts[1] == "bookmark" || parts[1] == "helpful") && c.req.Method == "PUT" {
		return s.action(c, post, parts[1])
	}
	if len(parts) == 1 && c.req.Method == "GET" {
		return 200, map[string]any{"post": post}, nil
	}
	// 阅读和互动已分发；以下分支全部属于作者专属操作。
	if !post.Owned {
		return 0, nil, fail(403, "ownership", "只能管理自己发布的体验")
	}
	if len(parts) == 2 && parts[1] == "verify" && c.req.Method == "POST" {
		return s.verifyPost(c, post)
	}
	if len(parts) != 1 {
		return 0, nil, fail(404, "not_found", "操作不存在")
	}
	if c.req.Method == "DELETE" {
		_, deleteErr := s.query(c.ctx, "UPDATE posts SET deleted=1,updated_at=?,revision=revision+1 WHERE id=? AND user_id=? AND deleted=0", c.now.Format(time.RFC3339), post.ID, c.user.ID)
		return 200, map[string]bool{"ok": true}, deleteErr
	}
	if c.req.Method != "PATCH" {
		return 0, nil, fail(405, "method", "操作方式不支持")
	}
	var input postInput
	if parseErr := decode(c, &input); parseErr != nil {
		return 0, nil, parseErr
	}
	if validateErr := validatePost(&input); validateErr != nil {
		return 0, nil, validateErr
	}
	rows, updateErr := s.query(c.ctx, "UPDATE posts SET title=?,body=?,sentiment=?,tags=?,updated_at=?,revision=revision+1 WHERE id=? AND user_id=? AND revision=? AND deleted=0 RETURNING id", input.Title, input.Body, input.Sentiment, encode(input.Tags), c.now.Format(time.RFC3339), post.ID, c.user.ID, input.Revision)
	if updateErr != nil {
		return 0, nil, updateErr
	}
	if len(rows) == 0 {
		return 0, nil, fail(409, "revision_conflict", "内容已更新，请重新打开后编辑")
	}
	updated, postErr := s.getPost(c, post.ID)
	return 200, map[string]any{"post": updated}, postErr
}

// action 用目标状态更新互动，重复请求不会累计。
func (s *Service) action(c *call, post Post, kind string) (int, any, error) {
	var input struct {
		Active *bool `json:"active"`
	}
	if parseErr := decode(c, &input); parseErr != nil {
		return 0, nil, parseErr
	}
	if input.Active == nil {
		return 0, nil, fail(400, "active", "请指定互动状态")
	}
	// 写入目标状态而非翻转状态，网络重试不会反转用户选择。
	if *input.Active {
		_, insertErr := s.query(c.ctx, "INSERT OR IGNORE INTO actions(user_id,post_id,kind,created_at) VALUES(?,?,?,?)", c.user.ID, post.ID, kind, c.now.Format(time.RFC3339))
		return 200, map[string]bool{"active": true}, insertErr
	}
	_, deleteErr := s.query(c.ctx, "DELETE FROM actions WHERE user_id=? AND post_id=? AND kind=?", c.user.ID, post.ID, kind)
	return 200, map[string]bool{"active": false}, deleteErr
}

// verifyPost 在事务外查询链，再以内容版本阻止迟到结果覆盖新编辑。
func (s *Service) verifyPost(c *call, post Post) (int, any, error) {
	var input struct {
		TxHash string `json:"tx_hash"`
	}
	if parseErr := decode(c, &input); parseErr != nil {
		return 0, nil, parseErr
	}
	if !txPattern.MatchString(input.TxHash) {
		return 0, nil, fail(400, "tx_hash", "交易哈希格式不正确")
	}
	if rateErr := s.limit(c, "proof:"+c.user.ID, 4); rateErr != nil {
		return 0, nil, rateErr
	}
	app, _ := catalog.Get(post.AppID)
	// 先在事务外等待上游，再用文章版本拒绝迟到结果。
	result := proof.Verify(c.ctx, s.HTTP, s.Config.RPCURL, *c.user.Wallet, input.TxHash, app)
	rows, updateErr := s.query(c.ctx, "UPDATE posts SET tx_hash=?,proof_status=?,proof_message=?,proof_checked_at=?,proof_data=?,revision=revision+1,updated_at=? WHERE id=? AND user_id=? AND revision=? AND deleted=0 RETURNING id", strings.ToLower(input.TxHash), result.Status, result.Message, result.CheckedAt, encode(result), c.now.Format(time.RFC3339), post.ID, c.user.ID, post.Revision)
	if updateErr != nil {
		return 0, nil, updateErr
	}
	if len(rows) == 0 {
		return 0, nil, fail(409, "revision_conflict", "内容已更新，请重新核验")
	}
	updated, postErr := s.getPost(c, post.ID)
	return 200, map[string]any{"post": updated}, postErr
}

// feedback 保存唯一事件并原子更新明确排除状态。
func (s *Service) feedback(c *call) (int, any, error) {
	var input struct {
		PostID  string `json:"post_id"`
		Kind    string `json:"kind"`
		EventID string `json:"event_id"`
		Active  *bool  `json:"active"`
	}
	if parseErr := decode(c, &input); parseErr != nil {
		return 0, nil, parseErr
	}
	if !keyPattern.MatchString(input.EventID) || (input.Kind != "open" && input.Kind != "hide" && input.Kind != "impression") {
		return 0, nil, fail(400, "feedback", "反馈格式不正确")
	}
	if _, postErr := s.getPost(c, input.PostID); postErr != nil {
		return 0, nil, postErr
	}
	kind := input.Kind
	if input.Kind == "hide" && input.Active != nil && !*input.Active {
		kind = "unhide"
	}
	statements := []database.Statement{{SQL: "INSERT OR IGNORE INTO events(user_id,event_id,post_id,kind,created_at) VALUES(?,?,?,?,?)", Args: []any{c.user.ID, input.EventID, input.PostID, kind, c.now.Format(time.RFC3339)}}}
	// 只允许刚插入的新事件改变状态，旧事件重放不能撤销后来操作。
	condition := "changes()=1 AND EXISTS(SELECT 1 FROM events WHERE user_id=? AND event_id=? AND post_id=? AND kind=?)"
	args := []any{c.user.ID, input.EventID, input.PostID, kind}
	if kind == "hide" || kind == "open" {
		statements = append(statements, database.Statement{SQL: "INSERT OR IGNORE INTO actions(user_id,post_id,kind,created_at) SELECT ?,?,?,? WHERE " + condition, Args: append([]any{c.user.ID, input.PostID, kind, c.now.Format(time.RFC3339)}, args...)})
	}
	if kind == "unhide" {
		statements = append(statements, database.Statement{SQL: "DELETE FROM actions WHERE user_id=? AND post_id=? AND kind='hide' AND " + condition, Args: append([]any{c.user.ID, input.PostID}, args...)})
	}
	if _, batchErr := s.DB.Batch(c.ctx, statements); batchErr != nil {
		return 0, nil, batchErr
	}
	rows, eventErr := s.query(c.ctx, "SELECT post_id,kind FROM events WHERE user_id=? AND event_id=?", c.user.ID, input.EventID)
	if eventErr != nil {
		return 0, nil, eventErr
	}
	if database.String(rows[0], "post_id") != input.PostID || database.String(rows[0], "kind") != kind {
		return 0, nil, fail(409, "event_conflict", fmt.Sprintf("反馈标识已使用，请重新%s", "操作"))
	}
	return 200, map[string]bool{"ok": true}, nil
}
