package community

import (
	"fmt"
	"net/http"
	"strings"
	"time"
	"usedit/internal/database"
	"usedit/internal/wallet"
)

// cookie 设置仅同站请求携带的会话，HTTPS 下强制安全传输。
func cookie(value string, secure bool, maxAge int) string {
	return (&http.Cookie{Name: "usedit_session", Value: value, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge}).String()
}

// userFromRow 只导出公开身份字段。
func userFromRow(row database.Row) User {
	var address *string
	if value := database.String(row, "wallet"); value != "" {
		address = &value
	}
	return User{ID: database.String(row, "id"), DisplayName: database.String(row, "display_name"), Wallet: address, Preferences: list(database.String(row, "preferences"))}
}

// session 验证会话及平台身份绑定；新访客以游客身份保存自己的内容。
func (s *Service) session(c *call) error {
	request := &http.Request{Header: http.Header{"Cookie": []string{c.req.Headers["cookie"]}}}
	existing, cookieErr := request.Cookie("usedit_session")
	if cookieErr == nil && len(existing.Value) <= 128 {
		c.token = digest(existing.Value)
		rows, sessionErr := s.query(c.ctx, "SELECT u.* FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token=? AND s.expires>? AND s.platform_id=?", c.token, c.now.Unix(), c.req.PlatformID)
		if sessionErr != nil {
			return sessionErr
		}
		if len(rows) == 1 {
			c.user = userFromRow(rows[0])
			return nil
		}
	}
	// 新会话入口另有总量上限，避免清除 Cookie 绕过用户限流。
	if rateErr := s.limit(c, "new-session", 120); rateErr != nil {
		return rateErr
	}
	id, idErr := randomID(18)
	if idErr != nil {
		return idErr
	}
	token, tokenErr := randomID(32)
	if tokenErr != nil {
		return tokenErr
	}
	c.token = digest(token)
	_, createErr := s.DB.Batch(c.ctx, []database.Statement{{SQL: "INSERT INTO users(id,display_name,preferences,created_at) VALUES(?,?,?,?)", Args: []any{id, "新朋友", "[]", c.now.Format(time.RFC3339)}}, {SQL: "INSERT INTO sessions(token,user_id,platform_id,expires) VALUES(?,?,?,?)", Args: []any{c.token, id, c.req.PlatformID, c.now.Add(30 * 24 * time.Hour).Unix()}}})
	if createErr != nil {
		return createErr
	}
	c.user = User{ID: id, DisplayName: "新朋友", Preferences: []string{}}
	c.headers["set-cookie"] = cookie(token, c.url.Scheme == "https", 30*86400)
	return nil
}

// challenge 将签名内容与域名、当前会话、地址和有效期绑定。
func (s *Service) challenge(c *call) (int, any, error) {
	var input struct {
		Address string `json:"address"`
	}
	if parseErr := decode(c, &input); parseErr != nil {
		return 0, nil, parseErr
	}
	if !wallet.ValidAddress(input.Address) {
		return 0, nil, fail(400, "address", "请输入完整的钱包地址")
	}
	if rateErr := s.limit(c, "challenge:"+c.user.ID, 6); rateErr != nil {
		return 0, nil, rateErr
	}
	id, idErr := randomID(18)
	if idErr != nil {
		return 0, nil, idErr
	}
	nonce, nonceErr := randomID(18)
	if nonceErr != nil {
		return 0, nil, nonceErr
	}
	// 有效期与随机数写进待签正文，客户端不能单独替换这些字段。
	expires := c.now.Add(5 * time.Minute)
	nonce = digest(nonce)[:32]
	message := fmt.Sprintf("%s wants you to sign in with your Ethereum account:\n%s\n\nSign in to UsedIt. This message does not authorize transactions.\n\nURI: %s://%s\nVersion: 1\nChain ID: 1\nNonce: %s\nIssued At: %s\nExpiration Time: %s", c.url.Host, input.Address, c.url.Scheme, c.url.Host, nonce, c.now.Format(time.RFC3339), expires.Format(time.RFC3339))
	_, insertErr := s.query(c.ctx, "INSERT INTO challenges(id,session_token,address,message,expires) VALUES(?,?,?,?,?)", id, c.token, strings.ToLower(input.Address), message, expires.Unix())
	return 200, map[string]string{"challenge_id": id, "message": message}, insertErr
}

// verifySignature 原子消费挑战，迁移游客收藏并轮换会话。
func (s *Service) verifySignature(c *call) (int, any, error) {
	var input struct {
		ChallengeID string `json:"challenge_id"`
		Signature   string `json:"signature"`
	}
	if parseErr := decode(c, &input); parseErr != nil {
		return 0, nil, parseErr
	}
	if rateErr := s.limit(c, "signature:"+c.user.ID, 10); rateErr != nil {
		return 0, nil, rateErr
	}
	rows, queryErr := s.query(c.ctx, "SELECT * FROM challenges WHERE id=? AND session_token=? AND expires>? AND consumed=0", input.ChallengeID, c.token, c.now.Unix())
	if queryErr != nil {
		return 0, nil, queryErr
	}
	if len(rows) != 1 {
		return 0, nil, fail(401, "challenge_expired", "签名请求已过期或已使用，请重新连接")
	}
	message := database.String(rows[0], "message")
	if !strings.HasPrefix(message, c.url.Host+" wants you to sign in") || !strings.Contains(message, "\nURI: "+c.url.Scheme+"://"+c.url.Host+"\n") {
		return 0, nil, fail(401, "challenge_domain", "签名请求来自不同站点")
	}
	address, signatureErr := wallet.Recover(message, input.Signature)
	if signatureErr != nil || address != database.String(rows[0], "address") {
		return 0, nil, fail(401, "signature", "签名与当前钱包不匹配")
	}
	consumed, consumeErr := s.query(c.ctx, "UPDATE challenges SET consumed=1 WHERE id=? AND session_token=? AND consumed=0 AND expires>? RETURNING id", input.ChallengeID, c.token, c.now.Unix())
	if consumeErr != nil {
		return 0, nil, consumeErr
	}
	if len(consumed) != 1 {
		return 0, nil, fail(409, "challenge_used", "签名请求已使用")
	}
	target := "w_" + digest(address)[:32]
	token, tokenErr := randomID(32)
	if tokenErr != nil {
		return 0, nil, tokenErr
	}
	now := c.now.Format(time.RFC3339)
	statements := []database.Statement{
		{SQL: "INSERT INTO users(id,wallet,display_name,preferences,created_at) VALUES(?,?,?,?,?) ON CONFLICT(id) DO NOTHING", Args: []any{target, address, c.user.DisplayName, encode(c.user.Preferences), now}},
	}
	// 已登录的钱包切换时不迁移其数据；只有游客可以合并到新身份。
	if c.user.Wallet == nil {
		statements = append(statements,
			database.Statement{SQL: "INSERT OR IGNORE INTO actions(user_id,post_id,kind,created_at) SELECT ?,post_id,kind,created_at FROM actions WHERE user_id=?", Args: []any{target, c.user.ID}},
			database.Statement{SQL: "UPDATE collections SET user_id=? WHERE user_id=?", Args: []any{target, c.user.ID}},
			database.Statement{SQL: "DELETE FROM actions WHERE user_id=?", Args: []any{c.user.ID}},
		)
	}
	statements = append(statements,
		database.Statement{SQL: "INSERT INTO sessions(token,user_id,platform_id,expires) VALUES(?,?,?,?)", Args: []any{digest(token), target, c.req.PlatformID, c.now.Add(30 * 24 * time.Hour).Unix()}},
		database.Statement{SQL: "DELETE FROM sessions WHERE token=?", Args: []any{c.token}},
	)
	if _, batchErr := s.DB.Batch(c.ctx, statements); batchErr != nil {
		return 0, nil, batchErr
	}
	users, userErr := s.query(c.ctx, "SELECT * FROM users WHERE id=?", target)
	if userErr != nil {
		return 0, nil, userErr
	}
	c.headers["set-cookie"] = cookie(token, c.url.Scheme == "https", 30*86400)
	return 200, map[string]any{"user": userFromRow(users[0])}, nil
}

// profile 允许访客维护显示名称，不改变身份归属。
func (s *Service) profile(c *call) (int, any, error) {
	var input struct {
		Name string `json:"display_name"`
	}
	if parseErr := decode(c, &input); parseErr != nil {
		return 0, nil, parseErr
	}
	// 显示名称仅作为资料，不接受客户端指定目标用户。
	input.Name = strings.TrimSpace(input.Name)
	if !validText(input.Name, 2, 30) {
		return 0, nil, fail(400, "display_name", "昵称需为 2–30 个字符")
	}
	_, updateErr := s.query(c.ctx, "UPDATE users SET display_name=? WHERE id=?", input.Name, c.user.ID)
	c.user.DisplayName = input.Name
	return 200, map[string]any{"user": c.user}, updateErr
}

// preferences 保存显式标签，内部应用偏好不能由客户端伪造。
func (s *Service) preferences(c *call) (int, any, error) {
	var input struct {
		Tags []string `json:"tags"`
	}
	if parseErr := decode(c, &input); parseErr != nil {
		return 0, nil, parseErr
	}
	if len(input.Tags) > 12 {
		return 0, nil, fail(400, "preferences", "最多选择 12 个兴趣")
	}
	// 保留标签集合的唯一性，冒号预留给服务端派生信号。
	tags := []string{}
	seen := map[string]bool{}
	for _, tag := range input.Tags {
		tag = strings.TrimSpace(tag)
		if !validText(tag, 1, 20) || strings.Contains(tag, ":") {
			return 0, nil, fail(400, "preferences", "兴趣标签格式不正确")
		}
		if !seen[tag] {
			tags = append(tags, tag)
			seen[tag] = true
		}
	}
	_, updateErr := s.query(c.ctx, "UPDATE users SET preferences=? WHERE id=?", encode(tags), c.user.ID)
	c.user.Preferences = tags
	return 200, map[string]any{"user": c.user}, updateErr
}
