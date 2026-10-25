package community

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"
	"usedit/internal/catalog"
	"usedit/internal/collectionpage"
	"usedit/internal/database"
)

type collectionInput struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Instructions string   `json:"instructions"`
	Layout       string   `json:"layout"`
	Theme        string   `json:"theme"`
	AppIDs       []string `json:"app_ids"`
	PostIDs      []string `json:"post_ids"`
	Version      int      `json:"version"`
}

// rowCollection 恢复当前精选页及其可编辑参数。
func rowCollection(row database.Row) Collection {
	return Collection{ID: database.String(row, "id"), Title: database.String(row, "title"), Description: database.String(row, "description"), Instructions: database.String(row, "instructions"), Layout: database.String(row, "layout"), Theme: database.String(row, "theme"), AppIDs: list(database.String(row, "app_ids")), PostIDs: list(database.String(row, "post_ids")), Version: database.Int(row, "version"), HTML: database.String(row, "html"), CreatedAt: database.String(row, "created_at"), UpdatedAt: database.String(row, "updated_at")}
}

// ownedCollection 始终用服务端身份约束查询。
func (s *Service) ownedCollection(c *call, id string) (Collection, error) {
	rows, queryErr := s.query(c.ctx, "SELECT * FROM collections WHERE id=? AND user_id=?", id, c.user.ID)
	if queryErr != nil {
		return Collection{}, queryErr
	}
	// 不区分不存在与他人所有，避免通过错误信息枚举私人页面。
	if len(rows) == 0 {
		return Collection{}, fail(404, "collection_not_found", "精选页不存在或不属于当前账户")
	}
	return rowCollection(rows[0]), nil
}

// collectionRoute 分发精选页列表、历史和导出。
func (s *Service) collectionRoute(c *call, path string) (int, any, error) {
	if path == "/collections" {
		if c.req.Method == "GET" {
			rows, queryErr := s.query(c.ctx, "SELECT * FROM collections WHERE user_id=? ORDER BY updated_at DESC,id LIMIT 50", c.user.ID)
			if queryErr != nil {
				return 0, nil, queryErr
			}
			items := []Collection{}
			for _, row := range rows {
				items = append(items, rowCollection(row))
			}
			return 200, map[string]any{"items": items}, nil
		}
		if c.req.Method == "POST" {
			var input collectionInput
			if parseErr := decode(c, &input); parseErr != nil {
				return 0, nil, parseErr
			}
			return s.saveCollection(c, input, nil)
		}
		return 0, nil, fail(405, "method", "操作方式不支持")
	}
	// 导出和历史版本也必须先通过同一份所有权检查。
	parts := strings.Split(strings.TrimPrefix(path, "/collections/"), "/")
	if len(parts) > 3 {
		return 0, nil, fail(404, "not_found", "页面不存在")
	}
	current, loadErr := s.ownedCollection(c, parts[0])
	if loadErr != nil {
		return 0, nil, loadErr
	}
	if len(parts) == 1 && c.req.Method == "GET" {
		versions, queryErr := s.query(c.ctx, "SELECT version,created_at FROM collection_versions WHERE collection_id=? ORDER BY version DESC LIMIT 50", current.ID)
		return 200, map[string]any{"collection": current, "versions": versions}, queryErr
	}
	if len(parts) == 1 && c.req.Method == "PATCH" {
		var input collectionInput
		if parseErr := decode(c, &input); parseErr != nil {
			return 0, nil, parseErr
		}
		return s.saveCollection(c, input, &current)
	}
	if len(parts) == 2 && parts[1] == "export" && c.req.Method == "GET" {
		c.headers["content-type"] = "text/html; charset=utf-8"
		c.headers["content-disposition"] = "attachment; filename=usedit-collection.html"
		c.headers["content-security-policy"] = "sandbox allow-scripts; default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'"
		return 200, rawHTML(current.HTML), nil
	}
	if len(parts) == 3 && parts[1] == "versions" && c.req.Method == "GET" {
		version, parseErr := strconv.Atoi(parts[2])
		if parseErr != nil {
			return 0, nil, fail(400, "version", "版本号无效")
		}
		rows, queryErr := s.query(c.ctx, "SELECT data FROM collection_versions WHERE collection_id=? AND version=?", current.ID, version)
		if queryErr != nil {
			return 0, nil, queryErr
		}
		if len(rows) == 0 {
			return 0, nil, fail(404, "version", "版本不存在")
		}
		var saved Collection
		if parseErr := json.Unmarshal([]byte(database.String(rows[0], "data")), &saved); parseErr != nil {
			return 0, nil, fail(500, "version_data", "历史版本读取失败")
		}
		return 200, map[string]any{"collection": saved}, nil
	}
	if len(parts) == 2 && parts[1] == "compose" && c.req.Method == "POST" {
		return s.compose(c, current)
	}
	return 0, nil, fail(404, "not_found", "页面操作不存在")
}

// compileCollection 批量读取被选内容后生成隔离运行的单文件页面。
func (s *Service) compileCollection(c *call, input collectionInput) (string, error) {
	if !validText(input.Title, 2, 80) || !validText(input.Description, 0, 500) || !validText(input.Instructions, 0, 1000) || len(input.AppIDs) > 12 || len(input.PostIDs) > 12 {
		return "", fail(400, "collection_content", "精选标题需为 2–80 字，说明最多 500 字，创作备注最多 1000 字，每类最多 12 项")
	}
	page := collectionpage.Input{Title: strings.TrimSpace(input.Title), Description: strings.TrimSpace(input.Description), Layout: input.Layout, Theme: input.Theme}
	seen := map[string]bool{}
	for _, id := range input.AppIDs {
		app, ok := catalog.Get(id)
		if !ok || seen[id] {
			return "", fail(400, "collection_apps", "应用不存在或重复")
		}
		seen[id] = true
		category := map[string]string{"identity": "链上身份", "exchange": "交易工具", "collectibles": "数字收藏"}[app.Category]
		if category == "" {
			category = "应用"
		}
		page.Apps = append(page.Apps, collectionpage.App{ID: app.ID, Name: app.Name, Category: category, Summary: app.Summary, Tags: app.Tags})
	}
	// 目录在内存中解析，文章 ID 统一汇成一条有界 SQL。
	byID := map[string]Post{}
	for _, post := range s.libraryPosts() {
		byID[post.ID] = post
	}
	seen = map[string]bool{}
	if len(input.PostIDs) > 0 {
		holders := make([]string, len(input.PostIDs))
		args := make([]any, len(input.PostIDs))
		for i, id := range input.PostIDs {
			holders[i] = "?"
			args[i] = id
		}
		posts, queryErr := s.loadPosts(c, "AND p.id IN ("+strings.Join(holders, ",")+")", args, 12)
		if queryErr != nil {
			return "", queryErr
		}
		for _, post := range posts {
			byID[post.ID] = post
		}
	}
	for _, id := range input.PostIDs {
		post, ok := byID[id]
		if !ok || seen[id] {
			return "", fail(400, "collection_posts", "选择的内容已删除或重复，请重新选择")
		}
		seen[id] = true
		page.Posts = append(page.Posts, collectionpage.Post{ID: post.ID, AppID: post.AppID, Title: post.Title, Body: post.Body, Kind: post.Kind, Sentiment: post.Sentiment, Tags: post.Tags})
	}
	html, compileErr := collectionpage.Compile(page)
	if compileErr != nil {
		return "", fail(400, "collection_layout", compileErr.Error())
	}
	return html, nil
}

// saveCollection 将当前页与不可变版本在同一批事务中保存。
func (s *Service) saveCollection(c *call, input collectionInput, current *Collection) (int, any, error) {
	if input.Layout == "" {
		input.Layout = "grid"
	}
	if input.Theme == "" {
		input.Theme = "sage"
	}
	html, compileErr := s.compileCollection(c, input)
	if compileErr != nil {
		return 0, nil, compileErr
	}
	now := c.now.Format(time.RFC3339Nano)
	id, idErr := randomID(18)
	if idErr != nil {
		return 0, nil, idErr
	}
	value := Collection{ID: id, Title: strings.TrimSpace(input.Title), Description: strings.TrimSpace(input.Description), Instructions: strings.TrimSpace(input.Instructions), Layout: input.Layout, Theme: input.Theme, AppIDs: input.AppIDs, PostIDs: input.PostIDs, HTML: html, Version: 1, CreatedAt: now, UpdatedAt: now}
	if value.AppIDs == nil {
		value.AppIDs = []string{}
	}
	if value.PostIDs == nil {
		value.PostIDs = []string{}
	}
	var statement database.Statement
	status := 201
	if current == nil {
		statement = database.Statement{SQL: "INSERT INTO collections(id,user_id,title,description,instructions,layout,theme,app_ids,post_ids,version,html,created_at,updated_at) SELECT ?,?,?,?,?,?,?,?,?,?,?,?,? WHERE (SELECT COUNT(*) FROM collections WHERE user_id=?)<50 RETURNING id", Args: []any{value.ID, c.user.ID, value.Title, value.Description, value.Instructions, value.Layout, value.Theme, encode(value.AppIDs), encode(value.PostIDs), 1, html, now, now, c.user.ID}}
	} else {
		if input.Version != current.Version {
			return 0, nil, fail(409, "version_conflict", "精选页已更新，请重新打开后保存")
		}
		if current.Version >= 50 {
			return 0, nil, fail(409, "version_limit", "本页已保存 50 个版本，请新建精选页继续整理")
		}
		value.ID = current.ID
		value.CreatedAt = current.CreatedAt
		value.Version = current.Version + 1
		status = 200
		statement = database.Statement{SQL: "UPDATE collections SET title=?,description=?,instructions=?,layout=?,theme=?,app_ids=?,post_ids=?,version=version+1,html=?,updated_at=? WHERE id=? AND user_id=? AND version=? RETURNING id", Args: []any{value.Title, value.Description, value.Instructions, value.Layout, value.Theme, encode(value.AppIDs), encode(value.PostIDs), html, now, value.ID, c.user.ID, current.Version}}
	}
	// 当前版本与历史快照同事务提交，版本竞争不能留下孤立历史。
	batch, batchErr := s.DB.Batch(c.ctx, []database.Statement{statement, {SQL: "INSERT OR IGNORE INTO collection_versions(collection_id,version,data,created_at) SELECT id,version,?,? FROM collections WHERE id=? AND user_id=? AND version=? AND updated_at=?", Args: []any{encode(value), now, value.ID, c.user.ID, value.Version, now}}})
	if batchErr != nil {
		return 0, nil, batchErr
	}
	if len(batch[0]) == 0 {
		if current == nil {
			return 0, nil, fail(409, "collection_limit", "最多保存 50 个精选页")
		}
		return 0, nil, fail(409, "version_conflict", "精选页已更新，请重新打开后保存")
	}
	return status, map[string]any{"collection": value}, nil
}

// compose 调用显式配置的外部创作服务；返回内容仍受既有结构和数据范围约束。
func (s *Service) compose(c *call, current Collection) (int, any, error) {
	var input struct {
		Instruction string `json:"instruction"`
	}
	if parseErr := decode(c, &input); parseErr != nil {
		return 0, nil, parseErr
	}
	if !validText(input.Instruction, 2, 1000) {
		return 0, nil, fail(400, "instruction", "请填写 2–1000 字的调整要求")
	}
	if s.Config.ExperienceMode {
		return s.composeExperience(c, current, input.Instruction)
	}
	if s.Config.ComposeURL == "" || s.Config.ComposeKey == "" {
		return 0, nil, fail(503, "composition_not_configured", "自由创作服务尚未接入；你仍可选择内容、布局和主题创建精选页")
	}
	if rateErr := s.limit(c, "compose:"+c.user.ID, 3); rateErr != nil {
		return 0, nil, rateErr
	}
	target, parseErr := url.Parse(s.Config.ComposeURL)
	if parseErr != nil || target.Scheme != "https" || target.Host == "" {
		return 0, nil, fail(503, "composition_config", "创作服务配置无效")
	}
	base := collectionInput{Title: current.Title, Description: current.Description, Instructions: input.Instruction, Layout: current.Layout, Theme: current.Theme, AppIDs: current.AppIDs, PostIDs: current.PostIDs, Version: current.Version}
	prompt := "Return only JSON with title, description, instructions, layout (editorial|grid), theme (ink|sage|sand), app_ids, post_ids, version. Preserve selected IDs and version. Adjust the page to the user's request; no unsupported data or external URLs. Title 2-80 characters, description <=500. Current: " + encode(base)
	payload := map[string]any{"model": s.Config.ComposeModel, "temperature": 0.4, "max_tokens": 1600, "messages": []map[string]string{{"role": "system", "content": prompt}, {"role": "user", "content": input.Instruction}}, "response_format": map[string]string{"type": "json_object"}}
	request, requestErr := http.NewRequestWithContext(c.ctx, "POST", target.String(), bytes.NewBufferString(encode(payload)))
	if requestErr != nil {
		return 0, nil, fail(503, "composition_request", "无法创建创作请求")
	}
	request.Header.Set("content-type", "application/json")
	request.Header.Set("authorization", "Bearer "+s.Config.ComposeKey)
	if runtime.GOOS == "js" {
		request.Header.Set("js.fetch:redirect", "error")
		request.Header.Set("js.fetch:credentials", "omit")
	}
	// 外部响应不受信任；限制大小且失败时不改当前版本。
	response, callErr := s.HTTP.Do(request)
	if callErr != nil {
		return 0, nil, fail(502, "composition_unavailable", "创作服务暂不可用，原版本已保留")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return 0, nil, fail(502, "composition_rejected", "创作服务未完成请求，原版本已保留")
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, 65537))
	if readErr != nil || len(data) > 65536 {
		return 0, nil, fail(502, "composition_size", "创作响应无法读取")
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if decodeErr := json.Unmarshal(data, &result); decodeErr != nil || len(result.Choices) == 0 {
		return 0, nil, fail(502, "composition_response", "创作服务返回格式不正确")
	}
	var revised collectionInput
	if parseErr := json.Unmarshal([]byte(result.Choices[0].Message.Content), &revised); parseErr != nil {
		return 0, nil, fail(502, "composition_content", "创作结果未通过格式检查")
	}
	// 外部结果只能调整展示，不能扩展用户选中的内容或覆盖新版本。
	revised.AppIDs = current.AppIDs
	revised.PostIDs = current.PostIDs
	revised.Version = current.Version
	return s.saveCollection(c, revised, &current)
}
