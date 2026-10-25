package collectionpage

import (
	"fmt"
	"html"
	"strings"
	"testing"
)

// sampleInput 提供包含应用和资料笔记的最小可编译页面输入。
func sampleInput() Input {
	return Input{
		Title: "链上工具阅读清单", Description: "记录实际使用感受与资料阅读笔记。", Layout: "grid", Theme: "sage",
		Apps: []App{{ID: "app-identity", Name: "身份工具", Category: "身份", Summary: "管理公开名称。", Tags: []string{"身份"}}},
		Posts: []Post{
			{ID: "review-one", AppID: "app-identity", Title: "名称设置记录", Body: "这次操作完成了名称设置。", Kind: "review", Sentiment: "neutral", Tags: []string{"身份"}},
			{ID: "note-one", AppID: "app-identity", Title: "官方文档阅读", Body: "文档说明了名称解析流程。", Kind: "reference", Sentiment: "positive"},
		},
	}
}

// TestCompileEscapesAllContentContexts 验证正文、标题、标签和属性中的内容均被转义。
func TestCompileEscapesAllContentContexts(t *testing.T) {
	// 同一段危险标记进入多个展示字段，既检查原串消失也检查安全文字保留。
	input := sampleInput()
	payload := `<img src=x onerror="alert(1)"><script>alert(2)</script>`
	input.Title = payload
	input.Description = payload
	input.Apps[0].Name = payload
	input.Apps[0].Summary = payload
	input.Apps[0].Category = `" onmouseover="alert(3)`
	input.Apps[0].Tags = []string{`<svg onload=alert(4)>`}
	input.Posts[0].Title = payload
	input.Posts[0].Body = payload
	document, compileErr := Compile(input)
	if compileErr != nil {
		t.Fatal(compileErr)
	}
	if strings.Contains(document, payload) || strings.Contains(document, `data-category="" onmouseover=`) {
		t.Fatal("unescaped user content entered markup")
	}
	if !strings.Contains(document, html.EscapeString(payload)) || strings.Count(document, "<script>") != 1 {
		t.Fatal("escaped text or isolated script boundary missing")
	}
}

// TestCompileLimitsAndPrivacy 验证非法选项、重复内容和私有记录不能编译。
func TestCompileLimitsAndPrivacy(t *testing.T) {
	// 每个分支从独立有效输入出发，只改变当前要验证的一项边界。
	cases := []struct {
		name   string
		change func(*Input)
	}{
		{"title-too-long", func(input *Input) { input.Title = strings.Repeat("字", 81) }},
		{"empty-title", func(input *Input) { input.Title = "  " }},
		{"description-too-long", func(input *Input) { input.Description = strings.Repeat("字", 501) }},
		{"invalid-layout", func(input *Input) { input.Layout = `grid" onclick="1` }},
		{"invalid-theme", func(input *Input) { input.Theme = "other" }},
		{"empty-items", func(input *Input) { input.Apps, input.Posts = nil, nil }},
		{"too-many-apps", func(input *Input) { input.Apps = make([]App, MaxItems+1) }},
		{"too-many-posts", func(input *Input) { input.Posts = make([]Post, MaxItems+1) }},
		{"duplicate-app", func(input *Input) { input.Apps = append(input.Apps, input.Apps[0]) }},
		{"duplicate-post", func(input *Input) { input.Posts = append(input.Posts, input.Posts[0]) }},
		{"hidden-post", func(input *Input) { input.Posts[0].Hidden = true }},
		{"private-post", func(input *Input) { input.Posts[0].Private = true }},
		{"unknown-kind", func(input *Input) { input.Posts[0].Kind = "other" }},
		{"invalid-sentiment", func(input *Input) { input.Posts[0].Sentiment = "other" }},
		{"long-body", func(input *Input) { input.Posts[0].Body = strings.Repeat("字", 6001) }},
		{"invalid-utf8", func(input *Input) { input.Title = string([]byte{0xff}) }},
		{"null-character", func(input *Input) { input.Title = "清单\x00" }},
		{"too-many-tags", func(input *Input) { input.Apps[0].Tags = make([]string, 11) }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			input := sampleInput()
			testCase.change(&input)
			document, compileErr := Compile(input)
			if !IsValidationError(compileErr) || document != "" {
				t.Fatalf("validation result: bytes=%d cause=%v", len(document), compileErr)
			}
		})
	}
}

// TestAddressesAndInternalIdentifiersNotExported 验证导出只包含展示内容且遮蔽地址与内部标识。
func TestAddressesAndInternalIdentifiersNotExported(t *testing.T) {
	// 地址、哈希和内部标识分别进入内容与关联字段，导出不能暴露其中任何一项。
	input := sampleInput()
	address := "0x" + strings.Repeat("a", 40)
	transaction := "0x" + strings.Repeat("b", 64)
	input.Title = address
	input.Description = transaction
	input.Posts[0].Body = "操作记录 " + address + "，交易 " + transaction
	document, compileErr := Compile(input)
	if compileErr != nil {
		t.Fatal(compileErr)
	}
	for _, privateValue := range []string{address, transaction, input.Apps[0].ID, input.Posts[0].ID, input.Posts[1].ID} {
		if strings.Contains(document, privateValue) {
			t.Fatalf("non-display value exported: %s", privateValue)
		}
	}
	if !strings.Contains(document, "[地址已隐藏]") {
		t.Fatal("address display was not redacted")
	}
}

// TestStandalonePageContract 检查单文件页面的隔离策略、搜索筛选和资料标识。
func TestStandalonePageContract(t *testing.T) {
	// 检查必要交互与隔离策略同时存在，且页面不依赖外部脚本或资源。
	document, compileErr := Compile(sampleInput())
	if compileErr != nil {
		t.Fatal(compileErr)
	}
	for _, required := range []string{
		`default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'`,
		`id="search"`, `id="kind"`, `id="category"`, `id="reset"`,
		`addEventListener('input',filter)`, `addEventListener('change',filter)`,
		`card.hidden=!visible`, `textContent=`, `role="status"`,
		`data-kind="资料笔记"`, `data-kind="使用体验"`, `data-category="身份"`,
	} {
		if !strings.Contains(document, required) {
			t.Errorf("missing page behavior: %s", required)
		}
	}
	for _, forbidden := range []string{"http://", "https://", "fetch(", "XMLHttpRequest", "<iframe", "<link", "src="} {
		if strings.Contains(document, forbidden) {
			t.Errorf("unexpected external surface: %s", forbidden)
		}
	}
	if strings.Count(document, "值得一试") != 0 {
		t.Fatal("reference note was labelled as personal sentiment")
	}
	if !strings.HasPrefix(document, "<!DOCTYPE html>") || !strings.HasSuffix(document, "</html>") {
		t.Fatal("page is not a complete document")
	}
}

// TestAllLayoutsAndThemes 验证每种允许的布局与主题组合都能生成对应页面。
func TestAllLayoutsAndThemes(t *testing.T) {
	// 遍历允许选项的笛卡尔积，确保配置能进入实际页面而非被静默忽略。
	for _, layout := range []string{"", "grid", "editorial"} {
		for _, theme := range []string{"", "ink", "sage", "sand"} {
			t.Run(layout+"-"+theme, func(t *testing.T) {
				input := sampleInput()
				input.Layout, input.Theme = layout, theme
				document, compileErr := Compile(input)
				if compileErr != nil || !strings.Contains(document, "theme-") {
					t.Fatalf("layout/theme: %v", compileErr)
				}
			})
		}
	}
}

// TestMaximumContentCountAndTextBounds 验证允许的最大条目数与文字长度边界。
func TestMaximumContentCountAndTextBounds(t *testing.T) {
	// 在合法上限处构造完整内容，避免只验证超限拒绝而漏掉边界误拒绝。
	input := sampleInput()
	input.Title = strings.Repeat("字", 80)
	input.Description = strings.Repeat("字", 500)
	input.Apps = nil
	input.Posts = nil
	for index := 0; index < MaxItems; index++ {
		input.Apps = append(input.Apps, App{ID: fmt.Sprintf("app-%d", index), Name: "应用", Summary: strings.Repeat("字", 500)})
		input.Posts = append(input.Posts, Post{ID: fmt.Sprintf("post-%d", index), Title: "记录", Body: strings.Repeat("字", 6000), Kind: "review"})
	}
	document, compileErr := Compile(input)
	if compileErr != nil || len(document) > 512*1024 {
		t.Fatalf("maximum valid page: bytes=%d cause=%v", len(document), compileErr)
	}
	if strings.Count(document, "<article ") != 24 {
		t.Fatal("valid item boundary lost content")
	}
}
