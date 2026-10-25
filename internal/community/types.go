// Package community 实现内容、身份、反馈和个人精选页业务。
package community

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
	"usedit/internal/database"
)

type Request struct {
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	PlatformID string            `json:"platform_id"`
}
type Response struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}
type Config struct {
	ExperienceMode bool
	RPCURL         string
	ComposeURL     string
	ComposeKey     string
	ComposeModel   string
}
type Service struct {
	DB     database.DB
	Config Config
	HTTP   *http.Client
	Now    func() time.Time
}
type User struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name"`
	Wallet      *string  `json:"wallet"`
	Preferences []string `json:"preferences"`
	CanPublish  bool     `json:"can_publish"`
}
type Author struct {
	Name   string  `json:"name"`
	Wallet *string `json:"wallet"`
}
type Post struct {
	ID             string             `json:"id"`
	Kind           string             `json:"kind"`
	AppID          string             `json:"app_id"`
	Title          string             `json:"title"`
	Body           string             `json:"body"`
	Sentiment      string             `json:"sentiment"`
	Tags           []string           `json:"tags"`
	Author         Author             `json:"author"`
	ProofStatus    string             `json:"proof_status"`
	ProofMessage   string             `json:"proof_message"`
	TxHash         string             `json:"tx_hash"`
	ProofCheckedAt string             `json:"proof_checked_at"`
	CreatedAt      string             `json:"created_at"`
	UpdatedAt      string             `json:"updated_at"`
	Revision       int                `json:"revision"`
	HelpfulCount   int                `json:"helpful_count"`
	Bookmarked     bool               `json:"bookmarked"`
	Helpful        bool               `json:"helpful"`
	Owned          bool               `json:"owned"`
	SourceURL      string             `json:"source_url,omitempty"`
	Reason         string             `json:"reason,omitempty"`
	Hidden         bool               `json:"-"`
	Interaction    *InteractionRecord `json:"interaction,omitempty"`
}

// InteractionRecord 描述站内可关联的操作记录，不包含资产或链上认证声明。
type InteractionRecord struct {
	ID         string `json:"id"`
	AppID      string `json:"app_id"`
	Title      string `json:"title"`
	Detail     string `json:"detail"`
	OccurredAt string `json:"occurred_at"`
}
type Collection struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Instructions string   `json:"instructions"`
	Layout       string   `json:"layout"`
	Theme        string   `json:"theme"`
	AppIDs       []string `json:"app_ids"`
	PostIDs      []string `json:"post_ids"`
	Version      int      `json:"version"`
	HTML         string   `json:"html"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
}
type apiError struct {
	status        int
	code, message string
}

// Error 允许统一出口识别业务错误。
func (e *apiError) Error() string { return e.code + ": " + e.message }

// fail 记录可预期的业务失败，避免输出输入正文与凭据。
func fail(status int, code, message string) error {
	slog.Warn("request_rejected", "code", code, "status", status)
	return &apiError{status, code, message}
}

// New 创建与运行平台无关的领域服务。
func New(db database.DB, config Config) *Service {
	return &Service{DB: db, Config: config, HTTP: &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, Now: time.Now}
}

// encode 对固定业务结构编码，异常会被记录并交由统一出口处理。
func encode(value any) string {
	data, marshalErr := json.Marshal(value)
	if marshalErr != nil {
		slog.Error("json_encode", "error", marshalErr)
		return "null"
	}
	return string(data)
}

// list 解码数据库中受服务端约束的字符串数组。
func list(value string) []string {
	items := []string{}
	// 空列视为空集合；格式损坏单独记录，不能变成包含空字符串的偏好。
	if value == "" {
		return items
	}
	if decodeErr := json.Unmarshal([]byte(value), &items); decodeErr != nil {
		slog.Error("stored_list_decode", "error", decodeErr)
		return []string{}
	}
	return items
}

// query 让上下文贯穿适配层。
func (s *Service) query(ctx context.Context, query string, args ...any) ([]database.Row, error) {
	return s.DB.Query(ctx, query, args...)
}
