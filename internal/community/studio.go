package community

import (
	"strconv"
	"strings"
	"time"
	"usedit/internal/database"
)

// studioProject 是工作台项目的公开元信息，代码与应用状态分别读取。
type studioProject struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Version   int    `json:"version"`
	Revision  int    `json:"revision"`
	ActiveRun string `json:"active_run"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// studioRow 去除身份与内部状态字段后返回项目。
func studioRow(row database.Row) studioProject {
	return studioProject{ID: database.String(row, "id"), Title: database.String(row, "title"), Version: database.Int(row, "version"), Revision: database.Int(row, "revision"), ActiveRun: database.String(row, "active_run"), CreatedAt: database.String(row, "created_at"), UpdatedAt: database.String(row, "updated_at")}
}

// studioOwned 所有子资源先校验项目归属，隐藏他人项目是否存在。
func (s *Service) studioOwned(c *call, id string) (studioProject, error) {
	// 项目 ID 与身份共同过滤，子资源不能绕过归属条件。
	rows, queryErr := s.query(c.ctx, "SELECT * FROM studio_projects WHERE id=? AND user_id=?", id, c.user.ID)
	if queryErr != nil {
		return studioProject{}, queryErr
	}
	if len(rows) == 0 {
		return studioProject{}, fail(404, "studio_not_found", "项目不存在或不属于当前账户")
	}
	return studioRow(rows[0]), nil
}

// studioRoute 在既有身份和同源校验之后分发新增工作台接口。
func (s *Service) studioRoute(c *call, path string) (int, any, error) {
	if path == "/studio/config" && c.req.Method == "GET" {
		return 200, map[string]any{"ready": s.Config.Studio.Ready(), "provider": s.Config.Studio.Provider, "max_projects": 20, "max_runs": 30}, nil
	}
	if path == "/studio/projects" {
		return s.studioProjects(c)
	}
	// 公共配置和项目列表处理完毕后，所有子路由统一校验归属。
	parts := strings.Split(strings.TrimPrefix(path, "/studio/projects/"), "/")
	if !strings.HasPrefix(path, "/studio/projects/") || len(parts) > 4 {
		return 0, nil, fail(404, "studio_route", "页面不存在")
	}
	project, ownerErr := s.studioOwned(c, parts[0])
	if ownerErr != nil {
		return 0, nil, ownerErr
	}
	if expireErr := s.studioExpire(c, project); expireErr != nil {
		return 0, nil, expireErr
	}
	project, reloadErr := s.studioOwned(c, parts[0])
	if reloadErr != nil {
		return 0, nil, reloadErr
	}
	if len(parts) == 1 && c.req.Method == "GET" {
		return s.studioDetail(c, project)
	}
	if len(parts) == 2 {
		switch parts[1] {
		case "runs":
			if c.req.Method == "POST" {
				return s.studioQueue(c, project)
			}
		case "restore":
			if c.req.Method == "POST" {
				return s.studioRestore(c, project)
			}
		case "state":
			return s.studioState(c, project)
		case "export":
			// 列表只读取有限元信息，写入分支继续经过数量与幂等校验。
			if c.req.Method == "GET" {
				return s.studioExport(c, project)
			}
		}
	}
	if len(parts) == 3 && parts[1] == "versions" && c.req.Method == "GET" {
		return s.studioVersion(c, project, parts[2])
	}
	if len(parts) == 4 && parts[1] == "runs" && parts[3] == "execute" && c.req.Method == "POST" {
		return s.studioExecute(c, project, parts[2])
	}
	return 0, nil, fail(405, "studio_method", "此项目操作不支持")
}

// studioProjects 用客户端请求标识去重新建，列表只读取当前用户的有限项目。
func (s *Service) studioProjects(c *call) (int, any, error) {
	// 列表只读取有限元信息，写入分支继续经过数量与幂等校验。
	if c.req.Method == "GET" {
		rows, queryErr := s.query(c.ctx, "SELECT * FROM studio_projects WHERE user_id=? ORDER BY updated_at DESC,id LIMIT 20", c.user.ID)
		if queryErr != nil {
			return 0, nil, queryErr
		}
		items := []studioProject{}
		for _, row := range rows {
			items = append(items, studioRow(row))
		}
		return 200, map[string]any{"items": items}, nil
	}
	if c.req.Method != "POST" {
		return 0, nil, fail(405, "studio_method", "操作方式不支持")
	}
	var input struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if parseErr := decode(c, &input); parseErr != nil {
		return 0, nil, parseErr
	}
	if !validText(input.ID, 16, 80) || !validText(input.Title, 2, 80) {
		return 0, nil, fail(400, "studio_input", "请输入2–80字项目名称")
	}
	now := c.now.Format(time.RFC3339Nano)
	rows, createErr := s.query(c.ctx, "INSERT INTO studio_projects(id,user_id,title,created_at,updated_at) SELECT ?,?,?,?,? WHERE (SELECT COUNT(*) FROM studio_projects WHERE user_id=?)<20 ON CONFLICT(id) DO NOTHING RETURNING id", input.ID, c.user.ID, strings.TrimSpace(input.Title), now, now, c.user.ID)
	if createErr != nil {
		return 0, nil, createErr
	}
	current, loadErr := s.studioOwned(c, input.ID)
	if loadErr != nil {
		if len(rows) == 0 {
			return 0, nil, fail(409, "studio_limit", "项目数量已达上限，或此请求标识不可用")
		}
		return 0, nil, loadErr
	}
	if current.Title != strings.TrimSpace(input.Title) {
		return 0, nil, fail(409, "studio_idempotency", "此请求标识已用于另一个项目")
	}
	return 201, map[string]any{"project": current}, nil
}

// studioDetail 一次批事务读取有界对话与版本索引，不逐项加载历史源码。
func (s *Service) studioDetail(c *call, p studioProject) (int, any, error) {
	result, batchErr := s.DB.Batch(c.ctx, []database.Statement{
		{SQL: "SELECT id,prompt,status,message,provider,model,base_version,result_version,created_at FROM studio_runs WHERE project_id=? ORDER BY created_at,id LIMIT 30", Args: []any{p.ID}},
		{SQL: "SELECT version,run_id,created_at FROM studio_versions WHERE project_id=? ORDER BY version DESC LIMIT 30", Args: []any{p.ID}},
	})
	if batchErr != nil {
		return 0, nil, batchErr
	}
	return 200, map[string]any{"project": p, "runs": result[0], "versions": result[1]}, nil
}

// studioVersion 按当前用户已校验项目和具体编号读取不可变代码。
func (s *Service) studioVersion(c *call, p studioProject, number string) (int, any, error) {
	// 仅接受已保存的正整数版本，不从外部拼接文件路径。
	version, parseErr := strconv.Atoi(number)
	if parseErr != nil || version < 1 {
		return 0, nil, fail(400, "studio_version", "版本编号无效")
	}
	rows, queryErr := s.query(c.ctx, "SELECT version,html,created_at FROM studio_versions WHERE project_id=? AND version=?", p.ID, version)
	if queryErr != nil {
		return 0, nil, queryErr
	}
	if len(rows) == 0 {
		return 0, nil, fail(404, "studio_version", "版本不存在")
	}
	return 200, rows[0], nil
}

// studioRestore 只切换代码指针，应用业务数据保留，revision 阻止旧任务覆盖。
func (s *Service) studioRestore(c *call, p studioProject) (int, any, error) {
	// 恢复仅移动代码指针，独立状态与历史记录保持原样。
	var input struct {
		Version  int `json:"version"`
		Revision int `json:"revision"`
	}
	if parseErr := decode(c, &input); parseErr != nil {
		return 0, nil, parseErr
	}
	rows, updateErr := s.query(c.ctx, "UPDATE studio_projects SET version=?,revision=revision+1,updated_at=? WHERE id=? AND user_id=? AND revision=? AND active_run='' AND EXISTS(SELECT 1 FROM studio_versions WHERE project_id=? AND version=?) RETURNING id", input.Version, c.now.Format(time.RFC3339Nano), p.ID, c.user.ID, input.Revision, p.ID, input.Version)
	if updateErr != nil {
		return 0, nil, updateErr
	}
	if len(rows) == 0 {
		return 0, nil, fail(409, "studio_conflict", "项目已更新、仍在创作中或版本不存在，请重新打开")
	}
	updated, loadErr := s.studioOwned(c, p.ID)
	return 200, map[string]any{"project": updated}, loadErr
}

// studioExport 以下载附件交付源码，响应禁止在本站拥有完整执行权限。
func (s *Service) studioExport(c *call, p studioProject) (int, any, error) {
	// 按已授权项目的当前指针读取，附件响应不在站点源内执行。
	rows, queryErr := s.query(c.ctx, "SELECT html FROM studio_versions WHERE project_id=? AND version=?", p.ID, p.Version)
	if queryErr != nil {
		return 0, nil, queryErr
	}
	if len(rows) == 0 {
		return 0, nil, fail(404, "studio_version", "项目还没有可导出的版本")
	}
	c.headers["content-type"] = "text/html; charset=utf-8"
	c.headers["content-disposition"] = "attachment; filename=orbit-app.html"
	c.headers["content-security-policy"] = "sandbox allow-scripts; default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; connect-src 'none'"
	return 200, rawHTML(database.String(rows[0], "html")), nil
}
