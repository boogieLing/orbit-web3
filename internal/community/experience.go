package community

import (
	"encoding/json"
	"log/slog"
	"strings"
	"time"
)

// attachInteraction 将站内白名单记录关联到作者自己的体验，保留版本竞争检查。
func (s *Service) attachInteraction(c *call, post Post, recordID string) (int, any, error) {
	var selected *InteractionRecord
	for _, record := range interactionRecords() {
		if record.ID == recordID && record.AppID == post.AppID {
			selected = &record
			break
		}
	}
	if selected == nil {
		return 0, nil, fail(400, "interaction_record", "请选择该应用的交互记录")
	}
	if rateErr := s.limit(c, "record:"+c.user.ID, 20); rateErr != nil {
		return 0, nil, rateErr
	}
	now := c.now.Format(time.RFC3339)
	// 不写入交易哈希或已核验状态；关联记录与内容版本一起提交。
	rows, updateErr := s.query(c.ctx, "UPDATE posts SET tx_hash='',proof_status='recorded',proof_message=?,proof_checked_at=?,proof_data=?,revision=revision+1,updated_at=? WHERE id=? AND user_id=? AND revision=? AND deleted=0 RETURNING id", "已关联「"+selected.Title+"」记录", now, encode(selected), now, post.ID, c.user.ID, post.Revision)
	if updateErr != nil {
		slog.Error("attach_interaction", "error", updateErr)
		return 0, nil, updateErr
	}
	if len(rows) == 0 {
		return 0, nil, fail(409, "revision_conflict", "内容已更新，请重新打开后关联")
	}
	updated, postErr := s.getPost(c, post.ID)
	return 200, map[string]any{"post": updated}, postErr
}

// storedInteraction 只解析记录关联字段，不将其他凭据结果误读为站内记录。
func storedInteraction(status, data string) *InteractionRecord {
	// 状态先限定字段含义，再解析数据，链上核验结果不会走此分支。
	if status != "recorded" {
		return nil
	}
	var record InteractionRecord
	if parseErr := json.Unmarshal([]byte(data), &record); parseErr != nil {
		slog.Error("interaction_decode", "error", parseErr)
		return nil
	}
	return &record
}

// applyPageInstruction 将明确的样式与文案要求变成有限的页面字段修改。
func applyPageInstruction(current Collection, instruction string) (collectionInput, error) {
	input := collectionInput{Title: current.Title, Description: current.Description, Instructions: instruction, Layout: current.Layout, Theme: current.Theme, AppIDs: current.AppIDs, PostIDs: current.PostIDs, Version: current.Version}
	matched := false
	styleText := instruction
	// 显式文案字段按行或分号读取，不让标题中的配色词意外改变主题。
	for _, clause := range strings.FieldsFunc(instruction, func(r rune) bool { return r == '\n' || r == ';' || r == '；' }) {
		pair := strings.FieldsFunc(clause, func(r rune) bool { return r == ':' || r == '：' })
		if len(pair) < 2 {
			continue
		}
		key := strings.TrimSpace(pair[0])
		value := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(clause), key+"："), key+":"))
		if key == "标题" {
			input.Title = value
			matched = true
			styleText = strings.ReplaceAll(styleText, clause, "")
		}
		if key == "简介" {
			input.Description = value
			matched = true
			styleText = strings.ReplaceAll(styleText, clause, "")
		}
	}
	for _, rule := range []struct {
		words  []string
		target *string
		value  string
	}{
		{[]string{"深色", "墨色", "暗色"}, &input.Theme, "ink"},
		{[]string{"清爽", "鼠尾草", "绿色"}, &input.Theme, "sage"},
		{[]string{"暖色", "暖沙", "米色"}, &input.Theme, "sand"},
		{[]string{"卡片", "网格", "清单"}, &input.Layout, "grid"},
		{[]string{"列表", "手记", "阅读"}, &input.Layout, "editorial"},
	} {
		for _, word := range rule.words {
			if strings.Contains(styleText, word) {
				*rule.target = rule.value
				matched = true
				break
			}
		}
	}
	if !matched {
		return collectionInput{}, fail(422, "page_instruction", "支持深色卡片、清爽列表、暖色手记，或按行填写「标题：…」「简介：…」")
	}
	return input, nil
}

// composeExperience 使用本地页面规则保存新版本，沿用相同事务和导出渲染。
func (s *Service) composeExperience(c *call, current Collection, instruction string) (int, any, error) {
	// 本地规则同样受限流与版本事务约束，失败不生成半个历史版本。
	if rateErr := s.limit(c, "compose:"+c.user.ID, 20); rateErr != nil {
		return 0, nil, rateErr
	}
	revised, instructionErr := applyPageInstruction(current, instruction)
	if instructionErr != nil {
		return 0, nil, instructionErr
	}
	return s.saveCollection(c, revised, &current)
}
