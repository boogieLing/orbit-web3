// Package collectionpage 将经过权限检查的精选内容编译成独立网页。
package collectionpage

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"regexp"
	"strings"
	"unicode/utf8"
)

const MaxItems = 12

// Input 是结构化页面内容，调用方负责所有权和可公开范围校验。
type Input struct {
	Title       string
	Description string
	Layout      string
	Theme       string
	Apps        []App
	Posts       []Post
}

// App 只接收公开展示字段，不携带合约地址、外部链接或账户标识。
type App struct {
	ID       string
	Name     string
	Category string
	Summary  string
	Tags     []string
}

// Post 不接收作者地址或交易信息，隐藏和私有记录不得进入页面。
type Post struct {
	ID        string
	AppID     string
	Title     string
	Body      string
	Kind      string
	Sentiment string
	Tags      []string
	Hidden    bool
	Private   bool
}

// ValidationError 提供适合接口层转换的输入错误，不回显内容正文。
type ValidationError struct {
	Field   string
	Message string
}

// Error 仅返回可展示的约束说明，不附带用户原始内容。
func (validation *ValidationError) Error() string { return validation.Message }

type card struct {
	Title, Body, Label, Category, AppName, Sentiment, Search string
	Tags                                                     []string
}
type page struct {
	Title, Description, Layout, Theme string
	Apps, Posts                       []card
	Categories                        []string
}

var addressPattern = regexp.MustCompile(`(?i)\b0x(?:[a-f0-9]{64}|[a-f0-9]{40})\b`)

// Compile 只把实际精选内容编译成页面，不发起外部调用或扩写用户正文。
func Compile(input Input) (string, error) {
	// 输入校验先于模板执行，正文中的不可信内容由 html/template 按上下文转义。
	prepared, prepareErr := prepare(input)
	if prepareErr != nil {
		return "", prepareErr
	}
	parsed, parseErr := template.New("collection").Parse(pageTemplate)
	if parseErr != nil {
		slog.Error("精选页模板解析失败", "cause", parseErr)
		return "", fmt.Errorf("解析精选页模板: %w", parseErr)
	}
	var output bytes.Buffer
	if renderErr := parsed.Execute(&output, prepared); renderErr != nil {
		slog.Error("精选页内容编译失败", "cause", renderErr)
		return "", fmt.Errorf("编译精选页内容: %w", renderErr)
	}
	if output.Len() > 512*1024 {
		return "", invalid("content", "精选页内容超过大小限制")
	}
	return output.String(), nil
}

// invalid 记录固定字段标识并构造供接口层识别的输入错误。
func invalid(field, message string) error {
	slog.Warn("精选页输入不符合约束", "field", field)
	return &ValidationError{Field: field, Message: message}
}

// prepare 校验所选内容和展示选项，再组成模板需要的有限字段。
func prepare(input Input) (page, error) {
	// 先限制总体规模，再收集应用名称和分类供记录卡片展示使用。
	var result page
	if validateErr := bounded("title", input.Title, 1, 80); validateErr != nil {
		return result, validateErr
	}
	if validateErr := bounded("description", input.Description, 0, 500); validateErr != nil {
		return result, validateErr
	}
	if len(input.Apps) > MaxItems || len(input.Posts) > MaxItems || len(input.Apps)+len(input.Posts) == 0 {
		return result, invalid("items", "请选择至少一项内容，应用和记录各不超过 12 项")
	}
	result = page{Title: display(input.Title), Description: display(input.Description), Layout: input.Layout, Theme: input.Theme}
	if result.Layout == "" {
		result.Layout = "grid"
	}
	if result.Theme == "" {
		result.Theme = "sage"
	}
	if result.Layout != "grid" && result.Layout != "editorial" {
		return page{}, invalid("layout", "请选择网格或阅读布局")
	}
	if result.Theme != "ink" && result.Theme != "sage" && result.Theme != "sand" {
		return page{}, invalid("theme", "请选择有效主题")
	}
	appNames := make(map[string]string, len(input.Apps))
	appCategories := make(map[string]string, len(input.Apps))
	categories := make(map[string]bool)
	for _, app := range input.Apps {
		if validateErr := validateApp(app, appNames); validateErr != nil {
			return page{}, validateErr
		}
		appNames[app.ID] = display(app.Name)
		category := display(app.Category)
		if category == "" {
			category = "其他"
		}
		appCategories[app.ID] = category
		if !categories[category] {
			categories[category] = true
			result.Categories = append(result.Categories, category)
		}
		result.Apps = append(result.Apps, makeCard(app.Name, app.Summary, "应用", category, "", "", app.Tags))
	}
	postIDs := make(map[string]bool, len(input.Posts))
	for _, post := range input.Posts {
		if validateErr := validatePost(post, postIDs); validateErr != nil {
			return page{}, validateErr
		}
		postIDs[post.ID] = true
		label := "使用体验"
		if post.Kind == "reference" {
			label = "资料笔记"
		}
		result.Posts = append(result.Posts, makeCard(post.Title, post.Body, label, appCategories[post.AppID], appNames[post.AppID], sentimentLabel(post), post.Tags))
	}
	return result, nil
}

// validateApp 拒绝缺失或重复应用标识，并检查每个公开展示字段的边界。
func validateApp(app App, seen map[string]string) error {
	// 重复判断使用内部标识，不能用可能重复的展示名称作为唯一键。
	if app.ID == "" || len(app.ID) > 128 || seen[app.ID] != "" {
		return invalid("apps", "应用标识缺失或重复")
	}
	if validateErr := bounded("app.name", app.Name, 1, 80); validateErr != nil {
		return validateErr
	}
	if validateErr := bounded("app.summary", app.Summary, 0, 500); validateErr != nil {
		return validateErr
	}
	if validateErr := bounded("app.category", app.Category, 0, 40); validateErr != nil {
		return validateErr
	}
	return validateTags(app.Tags)
}

// validatePost 排除私有或隐藏记录，同时约束类型、感受和正文大小。
func validatePost(post Post, seen map[string]bool) error {
	// 私有范围优先检查，后续字段合法也不能让被排除内容进入导出页面。
	if post.Private || post.Hidden {
		return invalid("posts", "隐藏或私有记录不能加入精选页")
	}
	if post.ID == "" || len(post.ID) > 128 || len(post.AppID) > 128 || seen[post.ID] {
		return invalid("posts", "记录标识缺失或重复")
	}
	if post.Kind != "review" && post.Kind != "reference" {
		return invalid("post.kind", "记录类型无效")
	}
	if post.Sentiment != "" && post.Sentiment != "positive" && post.Sentiment != "neutral" && post.Sentiment != "negative" {
		return invalid("post.sentiment", "体验感受无效")
	}
	if validateErr := bounded("post.title", post.Title, 1, 160); validateErr != nil {
		return validateErr
	}
	if validateErr := bounded("post.body", post.Body, 1, 6000); validateErr != nil {
		return validateErr
	}
	return validateTags(post.Tags)
}

// validateTags 限制标签数量和单项长度，避免无界页面内容。
func validateTags(tags []string) error {
	// 数量和单项长度分别限制，避免许多短标签或单个长标签撑大页面。
	if len(tags) > 10 {
		return invalid("tags", "标签不能超过 10 个")
	}
	for _, tag := range tags {
		if validateErr := bounded("tag", tag, 1, 40); validateErr != nil {
			return validateErr
		}
	}
	return nil
}

// bounded 按可见字符数检查长度，并拒绝无效编码及空字节。
func bounded(field, value string, minimum, maximum int) error {
	length := utf8.RuneCountInString(strings.TrimSpace(value))
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') || length < minimum || length > maximum {
		return invalid(field, fmt.Sprintf("%s 长度须为 %d 到 %d 个字", field, minimum, maximum))
	}
	return nil
}

// display 清理首尾空白并遮蔽正文中完整的账户地址与交易哈希。
func display(value string) string {
	return addressPattern.ReplaceAllString(strings.TrimSpace(value), "[地址已隐藏]")
}

// makeCard 组合已限制的卡片展示字段和本地搜索索引。
func makeCard(title, body, label, category, appName, sentiment string, tags []string) card {
	item := card{Title: display(title), Body: display(body), Label: label, Category: category, AppName: appName, Sentiment: sentiment}
	for _, tag := range tags {
		item.Tags = append(item.Tags, display(tag))
	}
	item.Search = strings.ToLower(strings.Join(append([]string{item.Title, item.Body, item.Label, item.Category, item.AppName}, item.Tags...), " "))
	return item
}

// sentimentLabel 只给使用体验显示感受，资料笔记不携带体验评价。
func sentimentLabel(post Post) string {
	// 资料笔记没有使用感受，必须在进入感受映射前直接排除。
	if post.Kind != "review" {
		return ""
	}
	switch post.Sentiment {
	case "positive":
		return "值得一试"
	case "neutral":
		return "各有取舍"
	case "negative":
		return "仍需改进"
	default:
		return ""
	}
}

// IsValidationError 让接口层区分用户输入问题与编译异常。
func IsValidationError(cause error) bool {
	var validation *ValidationError
	return errors.As(cause, &validation)
}
