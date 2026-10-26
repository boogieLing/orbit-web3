package studio

import (
	"strings"
	"testing"
)

// TestLocalResourceContext 区分交互变量和标签资源，避免合法的委托点击逻辑被拒绝。
func TestLocalResourceContext(t *testing.T) {
	document := strings.Replace(validDocument, "console.log('ready')", "const action = 'delete'; const src = 'article'; console.log(action, src)", 1)
	if _, err := Validate(document); err != nil {
		t.Fatalf("合法变量不应被识别成资源: %v", err)
	}
	// 正常脚本通过后，逐一核对常见标签的外部资源仍被拒绝。
	for _, tag := range []string{`<img src="https://example.com/a.png">`, `<form action="https://example.com">`, `<script src="https://example.com/a.js">`, `<a href="https://example.com">`} {
		if localResources(tag) {
			t.Errorf("外部资源不得通过: %s", tag)
		}
	}
}
