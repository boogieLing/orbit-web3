package community

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"usedit/internal/database"
	"usedit/internal/studio"
)

// studioExpire 按当前项目清理已过期任务，不扫描全部用户的任务。
func (s *Service) studioExpire(c *call, p studioProject) error {
	// 未执行与执行中的超时统一收敛，仅释放匹配任务的占用。
	if p.ActiveRun == "" {
		return nil
	}
	_, batchErr := s.DB.Batch(c.ctx, []database.Statement{
		{SQL: "UPDATE studio_runs SET status='failed',message=?,updated_at=? WHERE id=? AND project_id=? AND status IN ('queued','running') AND deadline<=?", Args: []any{"本次创作已中断或超时，已有版本仍然保留。", c.now.Format(time.RFC3339Nano), p.ActiveRun, p.ID, c.now.Unix()}},
		{SQL: "UPDATE studio_projects SET active_run='' WHERE id=? AND active_run=? AND EXISTS(SELECT 1 FROM studio_runs WHERE id=? AND status='failed')", Args: []any{p.ID, p.ActiveRun, p.ActiveRun}},
	})
	if batchErr != nil {
		slog.Error("studio_expire", "project_id", p.ID, "error", batchErr)
	}
	return batchErr
}

// studioQueue 将用户需求与唯一任务同事务保存，重复请求不会再次计费。
func (s *Service) studioQueue(c *call, p studioProject) (int, any, error) {
	var input struct {
		Prompt   string `json:"prompt"`
		Revision int    `json:"revision"`
		Key      string `json:"request_key"`
	}
	if parseErr := decode(c, &input); parseErr != nil {
		return 0, nil, parseErr
	}
	input.Prompt = strings.TrimSpace(input.Prompt)
	if !validText(input.Prompt, 2, 2000) || !validText(input.Key, 16, 80) {
		return 0, nil, fail(400, "studio_prompt", "请填写2–2000字的需求")
	}
	requestHash := digest(encode(input))
	rows, readErr := s.query(c.ctx, "SELECT id,status,request_hash FROM studio_runs WHERE project_id=? AND request_key=?", p.ID, input.Key)
	if readErr != nil {
		return 0, nil, readErr
	}
	if len(rows) > 0 {
		if database.String(rows[0], "request_hash") != requestHash {
			return 0, nil, fail(409, "studio_idempotency", "同一请求标识的内容不能变更")
		}
		return 200, map[string]any{"run": rows[0]}, nil
	}
	if !s.Config.Studio.Ready() {
		return 0, nil, fail(503, "studio_unavailable", "创作服务尚未连接，需求草稿已保留；已有项目仍可查看")
	}
	if p.Revision != input.Revision || p.ActiveRun != "" {
		return 0, nil, fail(409, "studio_conflict", "项目已更新或正在创作，请先读取最新状态")
	}
	if limitErr := s.limit(c, "studio:"+c.user.ID, 2); limitErr != nil {
		return 0, nil, limitErr
	}
	// 全站额度以数据库原子日计数限制，先预留再调用，上游失败也计入尝试次数。
	budget, budgetErr := s.query(c.ctx, "INSERT INTO rates(key,count,expires) VALUES(?,1,?) ON CONFLICT(key) DO UPDATE SET count=count+1 RETURNING count", "studio-day:"+c.now.Format("2006-01-02"), c.now.Add(48*time.Hour).Unix())
	if budgetErr != nil {
		return 0, nil, budgetErr
	}
	if database.Int(budget[0], "count") > 50 {
		return 0, nil, fail(429, "studio_daily_limit", "今日创作次数已达上限，请稍后再来；已保存项目不受影响")
	}
	id, idErr := randomID(18)
	if idErr != nil {
		return 0, nil, idErr
	}
	now := c.now.Format(time.RFC3339Nano)
	result, batchErr := s.DB.Batch(c.ctx, []database.Statement{
		{SQL: "UPDATE studio_projects SET active_run=?,updated_at=? WHERE id=? AND user_id=? AND revision=? AND active_run='' AND (SELECT COUNT(*) FROM studio_runs WHERE project_id=?)<30 RETURNING id", Args: []any{id, now, p.ID, c.user.ID, input.Revision, p.ID}},
		{SQL: "INSERT INTO studio_runs(id,project_id,request_key,request_hash,prompt,base_revision,base_version,status,provider,model,deadline,created_at,updated_at) SELECT ?,id,?,?,?,?,?,'queued',?,?,?,?,? FROM studio_projects WHERE id=? AND active_run=?", Args: []any{id, input.Key, requestHash, input.Prompt, p.Revision, p.Version, s.Config.Studio.Provider, s.Config.Studio.Model, c.now.Add(150 * time.Second).Unix(), now, now, p.ID, id}},
	})
	if batchErr != nil {
		return 0, nil, batchErr
	}
	if len(result[0]) == 0 {
		return 0, nil, fail(409, "studio_conflict", "项目已更新、仍在创作或已达30次创作上限，请重新打开")
	}
	return 202, map[string]any{"run": map[string]any{"id": id, "status": "queued"}}, nil
}

// studioExecute 原子领取持久任务；重发执行请求只返回原任务，不重复调用服务。
func (s *Service) studioExecute(c *call, p studioProject, id string) (int, any, error) {
	rows, loadErr := s.query(c.ctx, "SELECT * FROM studio_runs WHERE id=? AND project_id=?", id, p.ID)
	if loadErr != nil {
		return 0, nil, loadErr
	}
	if len(rows) == 0 {
		return 0, nil, fail(404, "studio_run", "任务不存在")
	}
	run := rows[0]
	if database.String(run, "status") != "queued" {
		return s.studioDetail(c, p)
	}
	if !s.Config.Studio.Ready() {
		return 0, nil, fail(503, "studio_unavailable", "创作服务尚未连接")
	}
	// 原子领取是唯一调用入口，并发与重放只会读取已存在任务。
	claimed, claimErr := s.query(c.ctx, "UPDATE studio_runs SET status='running',updated_at=?,deadline=? WHERE id=? AND project_id=? AND status='queued' AND deadline>? AND EXISTS(SELECT 1 FROM studio_projects WHERE id=? AND active_run=? AND revision=?) RETURNING id", c.now.Format(time.RFC3339Nano), c.now.Add(110*time.Second).Unix(), id, p.ID, c.now.Unix(), p.ID, id, database.Int(run, "base_revision"))
	if claimErr != nil {
		return 0, nil, claimErr
	}
	if len(claimed) == 0 {
		return s.studioDetail(c, p)
	}
	return s.studioBuild(c, p, run)
}

// studioBuild 在数据库事务外调用服务，用户断开不把旧版本标成新版本。
func (s *Service) studioBuild(c *call, p studioProject, run database.Row) (int, any, error) {
	id := database.String(run, "id")
	// 沿当前版本祖先恢复需求，回退后不会复活另一分支的功能。
	data, batchErr := s.DB.Batch(c.ctx, []database.Statement{
		{SQL: "SELECT html FROM studio_versions WHERE project_id=? AND version=?", Args: []any{p.ID, database.Int(run, "base_version")}},
		{SQL: "WITH RECURSIVE lineage(version,depth) AS (SELECT ?,0 UNION ALL SELECT r.base_version,l.depth+1 FROM lineage l JOIN studio_versions v ON v.project_id=? AND v.version=l.version JOIN studio_runs r ON r.id=v.run_id WHERE l.version>0 AND l.depth<29) SELECT r.prompt FROM lineage l JOIN studio_versions v ON v.project_id=? AND v.version=l.version JOIN studio_runs r ON r.id=v.run_id ORDER BY l.depth DESC", Args: []any{database.Int(run, "base_version"), p.ID, p.ID}},
	})
	if batchErr != nil {
		return s.studioFailed(c, p, id, "读取历史版本失败，原版本仍保留")
	}
	html := ""
	if len(data[0]) > 0 {
		html = database.String(data[0][0], "html")
	}
	previous := []string{}
	for _, row := range data[1] {
		previous = append(previous, database.String(row, "prompt"))
	}
	output, buildErr := studio.Build(c.ctx, s.HTTP, s.Config.Studio, studio.Context(html, previous, database.String(run, "prompt")))
	if buildErr != nil {
		return s.studioFailed(c, p, id, buildErr.Error())
	}
	return s.studioCommit(c, p, run, output)
}

// studioCommit 将代码、任务结果和项目指针在同一事务写入，过期结果不能覆盖。
func (s *Service) studioCommit(c *call, p studioProject, run database.Row, html string) (int, any, error) {
	id := database.String(run, "id")
	revision := database.Int(run, "base_revision")
	version := revision + 1
	now := s.Now().UTC()
	// 条件写入版本后才推进指针，三次写入共享事务。
	result, batchErr := s.DB.Batch(c.ctx, []database.Statement{
		{SQL: "INSERT INTO studio_versions(project_id,version,html,run_id,created_at) SELECT id,?,?,?,? FROM studio_projects WHERE id=? AND user_id=? AND revision=? AND active_run=? AND EXISTS(SELECT 1 FROM studio_runs WHERE id=? AND status='running' AND deadline>?) RETURNING version", Args: []any{version, html, id, now.Format(time.RFC3339Nano), p.ID, c.user.ID, revision, id, id, now.Unix()}},
		{SQL: "UPDATE studio_projects SET version=?,revision=revision+1,active_run='',updated_at=? WHERE id=? AND revision=? AND active_run=? AND EXISTS(SELECT 1 FROM studio_versions WHERE run_id=?)", Args: []any{version, now.Format(time.RFC3339Nano), p.ID, revision, id, id}},
		{SQL: "UPDATE studio_runs SET status='succeeded',message=?,result_version=?,updated_at=? WHERE id=? AND EXISTS(SELECT 1 FROM studio_versions WHERE run_id=?)", Args: []any{"应用代码已保存。请在预览中检查功能，可继续描述修改要求。", version, now.Format(time.RFC3339Nano), id, id}},
	})
	if batchErr != nil {
		return s.studioFailed(c, p, id, "保存新版本失败，旧版本仍然保留")
	}
	if len(result[0]) == 0 {
		return s.studioFailed(c, p, id, "项目状态已变化，本次结果未覆盖当前版本")
	}
	updated, loadErr := s.studioOwned(c, p.ID)
	if loadErr != nil {
		return 0, nil, loadErr
	}
	return s.studioDetail(c, updated)
}

// studioFailed 使用短独立上下文保存失败状态，保证断开请求也有可恢复记录。
func (s *Service) studioFailed(c *call, p studioProject, id, message string) (int, any, error) {
	// 脱离请求取消信号仍设短截止时间，确保失败记录有界完成。
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.ctx), 5*time.Second)
	defer cancel()
	_, batchErr := s.DB.Batch(ctx, []database.Statement{
		{SQL: "UPDATE studio_runs SET status='failed',message=?,updated_at=? WHERE id=? AND project_id=? AND status IN ('queued','running')", Args: []any{message, s.Now().UTC().Format(time.RFC3339Nano), id, p.ID}},
		{SQL: "UPDATE studio_projects SET active_run='' WHERE id=? AND active_run=? AND EXISTS(SELECT 1 FROM studio_runs WHERE id=? AND status='failed')", Args: []any{p.ID, id, id}},
	})
	if batchErr != nil {
		slog.Error("studio_failure_save", "run_id", id, "error", batchErr)
		return 0, nil, batchErr
	}
	return 0, nil, fail(502, "studio_failed", message)
}
