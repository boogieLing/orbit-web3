package studio

import (
	"html"
	"regexp"
	"strings"
)

const MaxHTMLBytes = 256 << 10

var (
	documentStart = regexp.MustCompile(`(?i)^<!doctype\s+html\s*>`)
	openingHTML   = regexp.MustCompile(`(?i)<html(?:\s[^>]*)?>`)
	openingHead   = regexp.MustCompile(`(?i)<head(?:\s[^>]*)?>`)
	openingBody   = regexp.MustCompile(`(?i)<body(?:\s[^>]*)?>`)
	inlineScript  = regexp.MustCompile(`(?i)<script(?:\s[^>]*)?>`)
	blockedTags   = regexp.MustCompile(`(?i)<\s*(?:iframe|frame|frameset|object|embed|base)\b`)
	markupTag     = regexp.MustCompile(`(?s)<[A-Za-z][^>]*>`)
	resourceAttr  = regexp.MustCompile(`(?i)\b(?:src|href|action|formaction|poster|srcset)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	cssURL        = regexp.MustCompile(`(?i)url\s*\(\s*['"]?\s*([^)'"\s]+)`)
	cssImport     = regexp.MustCompile(`(?i)@import\b`)
	refreshMeta   = regexp.MustCompile(`(?i)<meta\b[^>]*http-equiv\s*=\s*["']?\s*refresh\b`)
	nativeDialog  = regexp.MustCompile(`\b(?:alert|confirm|prompt)\s*\(`)
	networkCall   = regexp.MustCompile(`(?i)\b(?:fetch|XMLHttpRequest|WebSocket|EventSource|sendBeacon|importScripts)\s*\(|\bimport\s*(?:\(|[^;\n]*\bfrom\b)|\bnavigator\s*\.\s*serviceWorker\b`)
)

// Validate 校验可交付文档结构与资源边界；脚本执行权限由预览沙箱约束。
func Validate(raw string) (string, error) {
	if len(raw) > MaxHTMLBytes {
		return "", rejected("应用文件超过 256 KiB，请缩小需求范围。")
	}
	document := trimFence(strings.TrimSpace(raw))
	lower := strings.ToLower(document)
	if !validStructure(document, lower) {
		return "", rejected("应用文件结构不完整，请重新构建。")
	}
	// 解码实体后检查资源边界，实际执行仍依赖独立沙箱。
	decoded := html.UnescapeString(document)
	if blockedTags.MatchString(decoded) || refreshMeta.MatchString(decoded) || cssImport.MatchString(decoded) || networkCall.MatchString(decoded) || !localResources(decoded) {
		return "", rejected("应用包含外部资源或不支持的执行方式，请改用自包含页面。")
	}
	// 原生确认框在隔离预览中被浏览器禁用，避免保存无法完成关键操作的版本。
	if nativeDialog.MatchString(decoded) {
		return "", rejected("应用使用了预览不支持的系统弹窗。请要求改为页面内确认或撤销按钮后重新制作。")
	}
	return document, nil
}

// validStructure 检查单文档结构，排除截断输出。
func validStructure(document, lower string) bool {
	if !documentStart.MatchString(document) || !strings.HasSuffix(lower, "</html>") {
		return false
	}
	// 核对标签顺序与闭合次数，拒绝截断或嵌套完整文档。
	htmlStart, headStart, bodyStart := openingHTML.FindStringIndex(document), openingHead.FindStringIndex(document), openingBody.FindStringIndex(document)
	if htmlStart == nil || headStart == nil || bodyStart == nil || !inlineScript.MatchString(document) {
		return false
	}
	headEnd, bodyEnd := strings.Index(lower, "</head>"), strings.LastIndex(lower, "</body>")
	return htmlStart[0] < headStart[0] && headStart[1] <= headEnd && headEnd < bodyStart[0] && bodyStart[1] <= bodyEnd &&
		strings.Count(lower, "</html>") == 1 && strings.Count(lower, "</head>") == 1 && strings.Count(lower, "</body>") == 1 && strings.Contains(lower, "</script>")
}

// trimFence 兼容服务偶尔返回的完整 Markdown 包装。
func trimFence(document string) string {
	if !strings.HasPrefix(document, "```") || !strings.HasSuffix(document, "```") {
		return document
	}
	// 只剥离完整且明确的 HTML 包装，不修补任意损坏输出。
	firstLine := strings.IndexByte(document, '\n')
	if firstLine < 0 || firstLine > 10 {
		return document
	}
	language := strings.ToLower(strings.TrimSpace(document[3:firstLine]))
	if language != "" && language != "html" {
		return document
	}
	return strings.TrimSpace(document[firstLine+1 : len(document)-3])
}

// localResources 只允许内嵌资源和页内锚点。
func localResources(document string) bool {
	// 属性与 CSS 两条资源通道使用相同的本地资源约束。
	for _, tag := range markupTag.FindAllString(document, -1) {
		if !localAttributes(tag) {
			return false
		}
	}
	for _, resource := range cssURL.FindAllStringSubmatch(document, -1) {
		if !strings.HasPrefix(resource[1], "data:") && !strings.HasPrefix(resource[1], "blob:") && !strings.HasPrefix(resource[1], "#") {
			return false
		}
	}
	return true
}

// localAttributes 仅把标签内的属性当作资源，避免将脚本中的 action 变量误判为表单地址。
func localAttributes(tag string) bool {
	// 解码后的属性只允许空值、页内锚点和内嵌资源。
	for _, attribute := range resourceAttr.FindAllStringSubmatch(tag, -1) {
		value := strings.TrimSpace(attribute[1] + attribute[2] + attribute[3])
		if value != "" && !strings.HasPrefix(value, "#") && !strings.HasPrefix(value, "data:") && !strings.HasPrefix(value, "blob:") {
			return false
		}
	}
	return true
}
