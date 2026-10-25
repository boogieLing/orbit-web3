package community

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
	"usedit/internal/catalog"
	"usedit/internal/database"
)

type call struct {
	ctx     context.Context
	req     Request
	url     *url.URL
	user    User
	token   string
	headers map[string]string
	now     time.Time
}

// Handle 是原生服务器与 Worker 共用的请求入口。
func (s *Service) Handle(ctx context.Context, req Request) Response {
	started := time.Now()
	requestID, idErr := randomID(12)
	if idErr != nil {
		return Response{Status: 503, Body: `{"error":{"code":"entropy","message":"服务暂不可用"}}`}
	}
	headers := map[string]string{"content-type": "application/json; charset=utf-8", "cache-control": "no-store", "x-content-type-options": "nosniff", "x-request-id": requestID}
	parsed, parseErr := url.Parse(req.URL)
	if parseErr != nil || parsed.Host == "" {
		return Response{Status: 400, Headers: headers, Body: `{"error":{"code":"url","message":"请求地址无效"}}`}
	}
	c := &call{ctx: ctx, req: req, url: parsed, headers: headers, now: s.Now().UTC()}
	// 数据库错误只进入日志；响应正文统一收敛，避免泄漏底层语句。
	status, value, routeErr := s.route(c)
	if routeErr != nil {
		status = 500
		code, message := "storage_error", "服务暂时不可用，请稍后重试"
		var business *apiError
		if errors.As(routeErr, &business) {
			status = business.status
			code = business.code
			message = business.message
		} else {
			slog.Error("request_failed", "request_id", requestID, "error", routeErr)
		}
		value = map[string]any{"error": map[string]string{"code": code, "message": message}, "request_id": requestID}
	}
	body := encode(value)
	if text, ok := value.(rawHTML); ok {
		body = string(text)
	}
	slog.Info("request", "request_id", requestID, "method", req.Method, "path", parsed.Path, "status", status, "duration_ms", time.Since(started).Milliseconds())
	return Response{Status: status, Headers: headers, Body: body}
}

// route 先约束请求边界，再按公开的 v1 合同分发。
func (s *Service) route(c *call) (int, any, error) {
	if len(c.req.Body) > 65536 {
		return 0, nil, fail(413, "body_too_large", "请求内容过长")
	}
	// 浏览器写操作必须满足同源与自定义请求头两个条件。
	if c.req.Method != "GET" {
		origin := c.req.Headers["origin"]
		expected := c.url.Scheme + "://" + c.url.Host
		if c.req.Headers["x-usedit-request"] != "1" || (origin != "" && origin != expected) {
			return 0, nil, fail(403, "origin", "请从本站重新提交")
		}
		mediaType, _, mediaErr := mime.ParseMediaType(c.req.Headers["content-type"])
		if c.req.Method != "DELETE" && (mediaErr != nil || mediaType != "application/json") {
			return 0, nil, fail(415, "content_type", "请提交 JSON 内容")
		}
	}
	path := strings.TrimPrefix(c.url.Path, "/api/v1")
	if path == "/health" && c.req.Method == "GET" {
		_, healthErr := s.query(c.ctx, "SELECT 1 AS ready")
		return 200, map[string]string{"status": "ok", "runtime": "go-wasm", "storage": "sql"}, healthErr
	}
	if sessionErr := s.session(c); sessionErr != nil {
		return 0, nil, sessionErr
	}
	if c.req.Method != "GET" {
		if rateErr := s.limit(c, "write:"+c.user.ID, 60); rateErr != nil {
			return 0, nil, rateErr
		}
	}
	switch {
	case path == "/session" && c.req.Method == "GET":
		rows, statsErr := s.query(c.ctx, "SELECT COUNT(*) AS reviews, COALESCE(SUM(proof_status='verified'),0) AS verified FROM posts WHERE deleted=0")
		if statsErr != nil {
			return 0, nil, statsErr
		}
		return 200, map[string]any{"user": c.user, "capabilities": map[string]bool{"wallet": true, "proof": true, "freeform": s.Config.ComposeKey != "" && s.Config.ComposeURL != ""}, "stats": map[string]any{"reviews": database.Int(rows[0], "reviews"), "verified": database.Int(rows[0], "verified"), "apps": len(catalog.All())}}, nil
	case path == "/apps" && c.req.Method == "GET":
		return 200, map[string]any{"items": catalog.All()}, nil
	case path == "/auth/challenge" && c.req.Method == "POST":
		return s.challenge(c)
	case path == "/auth/verify" && c.req.Method == "POST":
		return s.verifySignature(c)
	case path == "/auth/logout" && c.req.Method == "POST":
		_, deleteErr := s.query(c.ctx, "DELETE FROM sessions WHERE token=?", c.token)
		c.headers["set-cookie"] = cookie("", c.url.Scheme == "https", 0)
		return 200, map[string]bool{"ok": true}, deleteErr
	case path == "/profile" && c.req.Method == "PATCH":
		return s.profile(c)
	case path == "/preferences" && c.req.Method == "PUT":
		return s.preferences(c)
	case path == "/feed" && c.req.Method == "GET":
		return s.feed(c)
	case path == "/me/posts" && c.req.Method == "GET":
		return s.myPosts(c)
	case path == "/posts" && c.req.Method == "POST":
		return s.createPost(c)
	case strings.HasPrefix(path, "/posts/"):
		return s.postRoute(c, strings.Split(strings.TrimPrefix(path, "/posts/"), "/"))
	case path == "/feedback" && c.req.Method == "POST":
		return s.feedback(c)
	case path == "/collections" || strings.HasPrefix(path, "/collections/"):
		return s.collectionRoute(c, path)
	default:
		return 0, nil, fail(404, "not_found", "内容不存在")
	}
}

// decode 拒绝空值、未知字段和多段 JSON，避免写入半有效内容。
func decode(c *call, value any) error {
	if strings.TrimSpace(c.req.Body) == "null" {
		return fail(400, "invalid_json", "内容不能为空")
	}
	decoder := json.NewDecoder(strings.NewReader(c.req.Body))
	decoder.DisallowUnknownFields()
	if decodeErr := decoder.Decode(value); decodeErr != nil {
		return fail(400, "invalid_json", "请求内容格式不正确")
	}
	// 第二次读取必须抵达 EOF，阻止一份请求携带两段不同数据。
	var extra any
	if trailingErr := decoder.Decode(&extra); trailingErr != io.EOF {
		return fail(400, "invalid_json", "请求只能包含一份内容")
	}
	return nil
}

// randomID 只使用系统随机源创建身份与挑战。
func randomID(size int) (string, error) {
	data := make([]byte, size)
	if _, readErr := rand.Read(data); readErr != nil {
		slog.Error("random_id", "error", readErr)
		return "", readErr
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

// digest 只保存会话令牌摘要。
func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// validText 按用户可见字符数检查边界。
func validText(value string, min, max int) bool {
	length := utf8.RuneCountInString(strings.TrimSpace(value))
	return length >= min && length <= max
}

// limit 以原子计数约束每分钟变更，避免并发绕过。
func (s *Service) limit(c *call, key string, max int) error {
	// 计数在数据库内递增，并发请求共享同一个分钟桶。
	bucket := fmt.Sprintf("%s:%d", key, c.now.Unix()/60)
	rows, rateErr := s.query(c.ctx, "INSERT INTO rates(key,count,expires) VALUES(?,1,?) ON CONFLICT(key) DO UPDATE SET count=count+1 RETURNING count", bucket, c.now.Unix()+120)
	if rateErr != nil {
		return rateErr
	}
	if database.Int(rows[0], "count") > max {
		c.headers["retry-after"] = "60"
		return fail(429, "rate_limit", "操作较频繁，请稍后重试")
	}
	return nil
}

type rawHTML string
