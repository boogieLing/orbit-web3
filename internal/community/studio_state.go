package community

import (
	"encoding/json"
	"usedit/internal/database"
)

// studioState 使用单独 revision 保存应用业务数据，代码回退不会倒退用户数据。
func (s *Service) studioState(c *call, p studioProject) (int, any, error) {
	if c.req.Method == "GET" {
		rows, queryErr := s.query(c.ctx, "SELECT state,state_revision FROM studio_projects WHERE id=? AND user_id=?", p.ID, c.user.ID)
		if queryErr != nil {
			return 0, nil, queryErr
		}
		var data map[string]string
		if parseErr := json.Unmarshal([]byte(database.String(rows[0], "state")), &data); parseErr != nil {
			return 0, nil, fail(500, "studio_state", "保存的应用数据暂时无法读取")
		}
		return 200, map[string]any{"data": data, "revision": database.Int(rows[0], "state_revision")}, nil
	}
	if c.req.Method != "PUT" {
		return 0, nil, fail(405, "studio_method", "操作方式不支持")
	}
	var input struct {
		Data     map[string]string `json:"data"`
		Revision int               `json:"revision"`
	}
	if parseErr := decode(c, &input); parseErr != nil {
		return 0, nil, parseErr
	}
	// 先约束快照，再用独立 revision 防止多标签页互相覆盖。
	encoded := encode(input.Data)
	if input.Data == nil || len(input.Data) > 100 || len(encoded) > 32768 {
		return 0, nil, fail(400, "studio_state_size", "应用数据超过32 KiB上限")
	}
	for key := range input.Data {
		if len(key) > 256 {
			return 0, nil, fail(400, "studio_state_key", "应用数据键过长")
		}
	}
	rows, updateErr := s.query(c.ctx, "UPDATE studio_projects SET state=?,state_revision=state_revision+1 WHERE id=? AND user_id=? AND state_revision=? RETURNING state_revision", encoded, p.ID, c.user.ID, input.Revision)
	if updateErr != nil {
		return 0, nil, updateErr
	}
	if len(rows) == 0 {
		return 0, nil, fail(409, "studio_state_conflict", "另一个页面已更新应用数据，请重新载入后继续")
	}
	return 200, map[string]any{"revision": database.Int(rows[0], "state_revision"), "data": input.Data}, nil
}
