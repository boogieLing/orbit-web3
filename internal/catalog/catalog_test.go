package catalog

import (
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestDirectoryContractsAndSources 核对分类、主网地址、来源链接与接口字段数量。
func TestDirectoryContractsAndSources(t *testing.T) {
	all := All()
	if len(all) != 3 {
		t.Fatalf("目录数量 = %d", len(all))
	}
	categories := map[string]bool{"identity": true, "exchange": true, "collectibles": true}
	// 每命中一个分类便移除，重复分类和合同外分类都会立即失败。
	for _, app := range all {
		if !categories[app.Category] || app.ChainID != 1 || len(app.Contracts) == 0 {
			t.Fatalf("目录范围无效: %s", app.ID)
		}
		delete(categories, app.Category)
		for _, address := range app.Contracts {
			_, decodeError := hex.DecodeString(strings.TrimPrefix(address, "0x"))
			if len(address) != 42 || !strings.HasPrefix(address, "0x") || decodeError != nil {
				t.Fatalf("合约地址格式无效: %s", app.ID)
			}
		}
		for _, link := range []string{app.URL, app.SourceURL} {
			parsed, parseError := url.Parse(link)
			if parseError != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
				t.Fatalf("来源链接无效: %s", app.ID)
			}
		}
		encoded, encodeError := json.Marshal(app)
		if encodeError != nil {
			t.Fatal(encodeError)
		}
		var fields map[string]any
		if decodeError := json.Unmarshal(encoded, &fields); decodeError != nil || len(fields) != 11 || fields["chain_id"] != float64(1) || fields["source_url"] == nil {
			t.Fatalf("目录 JSON 合同无效: %s", encoded)
		}
	}
}

// TestDirectoryCopiesDoNotChangeProofScope 验证外部修改读取结果不能改变可信合约范围。
func TestDirectoryCopiesDoNotChangeProofScope(t *testing.T) {
	all := All()
	originalContract := all[0].Contracts[0]
	// 同时改动合约和标签，再重新读取，证明隔离覆盖两个可变切片。
	all[0].Contracts[0] = "changed"
	all[0].Tags[0] = "changed"
	app, found := Get("ens")
	if !found || app.Contracts[0] != originalContract || app.Tags[0] == "changed" {
		t.Fatal("外部修改污染了目录")
	}
	app.Contracts[0] = "changed-again"
	second, _ := Get("ens")
	if second.Contracts[0] != originalContract {
		t.Fatal("单项读取共享了合约切片")
	}
	if _, found := Get("unknown"); found {
		t.Fatal("未知应用不应存在")
	}
}

// TestReferenceNotesAreSourcedAndIndependent 验证笔记明确标注来源、核对时间并隔离可变标签。
func TestReferenceNotesAreSourcedAndIndependent(t *testing.T) {
	list := References()
	if len(list) != 3 {
		t.Fatalf("资料笔记数量 = %d", len(list))
	}
	for _, reference := range list {
		// 资料必须能回溯到既有应用与来源，核对时间也不能来自未来。
		if _, found := Get(reference.AppID); !found || !strings.HasPrefix(reference.Body, "资料笔记｜") || reference.SourceName == "" || reference.SourceURL == "" {
			t.Fatalf("资料笔记缺少来源或归属: %s", reference.ID)
		}
		checked, parseError := time.Parse(time.RFC3339, reference.CheckedAt)
		if parseError != nil || checked.After(time.Now()) {
			t.Fatalf("核对时间无效: %s", reference.ID)
		}
	}
	list[0].Tags[0] = "changed"
	if References()[0].Tags[0] == "changed" {
		t.Fatal("资料笔记标签共享了切片")
	}
}
