package proof

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"usedit/internal/catalog"
)

var (
	testWallet = "0x" + strings.Repeat("2", 40)
	testHash   = "0x" + strings.Repeat("1", 64)
	testBlock  = "0x" + strings.Repeat("3", 64)
)

type responseEdit func(method string, value map[string]any) any

// startRPC 启动本地 HTTP 服务，以乱序响应检验批量关联，并允许针对指定字段设置边界值。
func startRPC(t *testing.T, edit responseEdit) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	app, _ := catalog.Get("ens")
	calls := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" {
			t.Error("请求协议不正确")
		}
		var requests []rpcRequest
		if decodeError := json.NewDecoder(request.Body).Decode(&requests); decodeError != nil {
			t.Error(decodeError)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(requests) != 4 && len(requests) != 1 {
			t.Errorf("批次大小不正确: %d", len(requests))
		}
		responses := make([]map[string]any, 0, len(requests))
		for _, rpc := range requests {
			var result any
			switch rpc.Method {
			case "eth_chainId":
				result = "0x1"
			case "eth_blockNumber":
				result = "0x6f"
			case "eth_getTransactionByHash":
				result = map[string]any{"hash": testHash, "from": testWallet, "to": app.Contracts[0], "chainId": "0x1", "blockNumber": "0x64", "blockHash": testBlock}
			case "eth_getTransactionReceipt":
				result = map[string]any{"transactionHash": testHash, "from": testWallet, "to": app.Contracts[0], "status": "0x1", "blockNumber": "0x64", "blockHash": testBlock}
			case "eth_getBlockByNumber":
				if len(requests) != 1 || len(rpc.Params) != 2 || rpc.Params[0] != "0x64" || rpc.Params[1] != false {
					t.Error("主链区块查询应仅查询对应高度且不展开交易")
				}
				result = map[string]any{"number": "0x64", "hash": testBlock}
			default:
				t.Errorf("出现合同外查询: %s", rpc.Method)
			}
			value := map[string]any{"jsonrpc": "2.0", "id": rpc.ID, "result": result}
			if edit != nil {
				result = edit(rpc.Method, value)
				if changed, isObject := result.(map[string]any); isObject {
					value = changed
				}
			}
			responses = append(responses, value)
		}
		// 服务端允许任意返回顺序，核对必须按 ID 关联。
		for left, right := 0, len(responses)-1; left < right; left, right = left+1, right-1 {
			responses[left], responses[right] = responses[right], responses[left]
		}
		writer.Header().Set("Content-Type", "application/json")
		if encodeError := json.NewEncoder(writer).Encode(responses); encodeError != nil {
			t.Error(encodeError)
		}
	}))
	t.Cleanup(server.Close)
	return server, calls
}

// TestVerifiedRequiresCanonicalBlockAndTwelveConfirmations 验证第十二个确认及主链哈希一致后才通过。
func TestVerifiedRequiresCanonicalBlockAndTwelveConfirmations(t *testing.T) {
	server, calls := startRPC(t, nil)
	app, _ := catalog.Get("ens")
	result := Verify(context.Background(), server.Client(), server.URL, testWallet, testHash, app)
	// 精确卡在第十二个确认，并要求返回已核对的高度和主链哈希。
	if result.Status != "verified" || result.Confirmations != 12 || result.BlockNumber != 100 || result.BlockHash != testBlock || result.CheckedAt == "" {
		t.Fatalf("结果不符合预期: %+v", result)
	}
	if calls.Load() != 2 {
		t.Fatalf("HTTP 调用数 = %d，应为两批", calls.Load())
	}
}

// TestTransactionAndChainBoundaries 验证归属、目标、成功回执与区块字段异常不能获得通过状态。
func TestTransactionAndChainBoundaries(t *testing.T) {
	cases := []struct {
		name, method, field string
		value               any
		status              string
		calls               int32
	}{
		{name: "sender", method: "eth_getTransactionByHash", field: "from", value: "0x" + strings.Repeat("4", 40), status: "rejected", calls: 1},
		{name: "target", method: "eth_getTransactionByHash", field: "to", value: "0x" + strings.Repeat("4", 40), status: "rejected", calls: 1},
		{name: "contract_creation", method: "eth_getTransactionByHash", field: "to", value: nil, status: "rejected", calls: 1},
		{name: "failed_receipt", method: "eth_getTransactionReceipt", field: "status", value: "0x0", status: "rejected", calls: 1},
		{name: "receipt_sender", method: "eth_getTransactionReceipt", field: "from", value: "0x" + strings.Repeat("4", 40), status: "unavailable", calls: 1},
		{name: "receipt_target", method: "eth_getTransactionReceipt", field: "to", value: "0x" + strings.Repeat("4", 40), status: "unavailable", calls: 1},
		{name: "receipt_hash", method: "eth_getTransactionReceipt", field: "transactionHash", value: "0x" + strings.Repeat("4", 64), status: "unavailable", calls: 1},
		{name: "transaction_hash", method: "eth_getTransactionByHash", field: "hash", value: "0x" + strings.Repeat("4", 64), status: "unavailable", calls: 1},
		{name: "transaction_chain", method: "eth_getTransactionByHash", field: "chainId", value: "0x89", status: "unavailable", calls: 1},
		{name: "malformed_status", method: "eth_getTransactionReceipt", field: "status", value: "0x2", status: "unavailable", calls: 1},
		{name: "missing_status", method: "eth_getTransactionReceipt", field: "status", value: nil, status: "unavailable", calls: 1},
		{name: "invalid_quantity", method: "eth_getTransactionReceipt", field: "blockNumber", value: "0x064", status: "unavailable", calls: 1},
		{name: "block_disagreement", method: "eth_getTransactionByHash", field: "blockNumber", value: "0x63", status: "pending", calls: 1},
		{name: "missing_block", method: "eth_getTransactionByHash", field: "blockHash", value: nil, status: "unavailable", calls: 1},
		{name: "reorganized_block", method: "eth_getBlockByNumber", field: "hash", value: "0x" + strings.Repeat("4", 64), status: "pending", calls: 2},
		{name: "wrong_block_height", method: "eth_getBlockByNumber", field: "number", value: "0x65", status: "unavailable", calls: 2},
		{name: "short_block_hash", method: "eth_getBlockByNumber", field: "hash", value: "0x00", status: "unavailable", calls: 2},
	}
	// 每次仅破坏一个字段，调用次数同时约束失败后的查询是否及时停止。
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			server, calls := startRPC(t, func(method string, value map[string]any) any {
				if method == testCase.method {
					value["result"].(map[string]any)[testCase.field] = testCase.value
				}
				return value
			})
			app, _ := catalog.Get("ens")
			result := Verify(context.Background(), server.Client(), server.URL, testWallet, testHash, app)
			if result.Status != testCase.status || calls.Load() != testCase.calls {
				t.Fatalf("结果 = %+v，调用 = %d", result, calls.Load())
			}
		})
	}
}

// TestPendingAndUnavailableResponses 区分等待确认、节点落后、交易缺失和数据源格式失败。
func TestPendingAndUnavailableResponses(t *testing.T) {
	cases := []struct {
		name, method, status string
		value                any
		calls                int32
	}{
		{name: "pending_receipt", method: "eth_getTransactionReceipt", value: nil, status: "pending", calls: 1},
		{name: "missing_transaction_with_receipt", method: "eth_getTransactionByHash", value: nil, status: "unavailable", calls: 1},
		{name: "eleven_confirmations", method: "eth_blockNumber", value: "0x6e", status: "pending", calls: 1},
		{name: "node_behind", method: "eth_blockNumber", value: "0x63", status: "pending", calls: 1},
		{name: "wrong_network", method: "eth_chainId", value: "0x89", status: "unavailable", calls: 1},
		{name: "null_chain", method: "eth_chainId", value: nil, status: "unavailable", calls: 1},
		{name: "numeric_height", method: "eth_blockNumber", value: 111, status: "unavailable", calls: 1},
		{name: "noncanonical_quantity", method: "eth_blockNumber", value: "0x06f", status: "unavailable", calls: 1},
		{name: "negative_height", method: "eth_blockNumber", value: "0x-1", status: "unavailable", calls: 1},
		{name: "signed_height", method: "eth_blockNumber", value: "0x+6f", status: "unavailable", calls: 1},
		{name: "missing_canonical_block", method: "eth_getBlockByNumber", value: nil, status: "pending", calls: 2},
	}
	// 等待链上确认与无法信任数据源是两类状态，不能统一返回成功或拒绝。
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			server, calls := startRPC(t, func(method string, value map[string]any) any {
				if method == testCase.method {
					value["result"] = testCase.value
				}
				return value
			})
			app, _ := catalog.Get("ens")
			result := Verify(context.Background(), server.Client(), server.URL, testWallet, testHash, app)
			if result.Status != testCase.status || calls.Load() != testCase.calls {
				t.Fatalf("结果 = %+v，调用 = %d", result, calls.Load())
			}
		})
	}
	t.Run("unknown_transaction", func(t *testing.T) {
		server, calls := startRPC(t, func(method string, value map[string]any) any {
			if method == "eth_getTransactionByHash" || method == "eth_getTransactionReceipt" {
				value["result"] = nil
			}
			return value
		})
		app, _ := catalog.Get("ens")
		result := Verify(context.Background(), server.Client(), server.URL, testWallet, testHash, app)
		if result.Status != "pending" || calls.Load() != 1 {
			t.Fatalf("结果 = %+v", result)
		}
	})
}

// TestRPCEnvelopeFailures 验证错误、重复或缺失响应标识与版本异常只返回安全失败信息。
func TestRPCEnvelopeFailures(t *testing.T) {
	for _, failure := range []string{"rpc_error", "wrong_id", "duplicate_id", "wrong_version", "missing_result"} {
		t.Run(failure, func(t *testing.T) {
			server, calls := startRPC(t, func(method string, value map[string]any) any {
				if method != "eth_chainId" {
					return value
				}
				// 其他方法保持正常，仅破坏一个响应信封，检验整批是否被拒绝。
				switch failure {
				case "rpc_error":
					delete(value, "result")
					value["error"] = map[string]any{"code": -32000, "message": "private upstream detail"}
				case "wrong_id":
					value["id"] = 99
				case "duplicate_id":
					value["id"] = 2
				case "wrong_version":
					value["jsonrpc"] = "1.0"
				case "missing_result":
					delete(value, "result")
				}
				return value
			})
			app, _ := catalog.Get("ens")
			result := Verify(context.Background(), server.Client(), server.URL, testWallet, testHash, app)
			if result.Status != "unavailable" || calls.Load() != 1 || strings.Contains(result.Message, "private") {
				t.Fatalf("结果 = %+v，调用 = %d", result, calls.Load())
			}
		})
	}
}

// TestHTTPFailuresAreBoundedAndDoNotRetry 验证状态码、截断与超大响应被拒绝，且不追加重试请求。
func TestHTTPFailuresAreBoundedAndDoNotRetry(t *testing.T) {
	cases := []struct {
		name string
		code int
		body string
	}{
		{name: "upstream_failure", code: 503, body: "private upstream detail"},
		{name: "truncated_json", code: 200, body: `[{"jsonrpc":"2.0","id":1,`},
		{name: "wrong_shape", code: 200, body: `{"jsonrpc":"2.0","id":1,"result":"0x1"}`},
		{name: "oversized", code: 200, body: strings.Repeat(" ", maxResponseBytes+1)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				writer.WriteHeader(testCase.code)
				_, _ = io.WriteString(writer, testCase.body)
			}))
			defer server.Close()
			app, _ := catalog.Get("ens")
			result := Verify(context.Background(), server.Client(), server.URL, testWallet, testHash, app)
			if result.Status != "unavailable" || calls.Load() != 1 || strings.Contains(result.Message, "private") {
				t.Fatalf("结果 = %+v，调用 = %d", result, calls.Load())
			}
		})
	}
}

// TestRedirectIsNotFollowed 验证跳转目标没有收到请求，避免查询被转交其他主机。
func TestRedirectIsNotFollowed(t *testing.T) {
	var destinationCalls atomic.Int32
	// 独立目标服务只统计访问，零次访问才证明请求没有随跳转泄出。
	destination := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		destinationCalls.Add(1)
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	app, _ := catalog.Get("ens")
	result := Verify(context.Background(), source.Client(), source.URL, testWallet, testHash, app)
	if result.Status != "unavailable" || destinationCalls.Load() != 0 {
		t.Fatalf("发生了重定向跟随: %+v", result)
	}
}

// TestDeadlineCoversBothBatches 验证第二批查询继承剩余时限，而不是重新获得完整等待窗口。
func TestDeadlineCoversBothBatches(t *testing.T) {
	var calls atomic.Int32
	// 第一批消耗部分预算，第二批延迟超过剩余预算，验证共用截止时间。
	server, _ := startRPC(t, func(method string, value map[string]any) any {
		if method == "eth_chainId" {
			calls.Add(1)
			time.Sleep(25 * time.Millisecond)
		}
		if method == "eth_getBlockByNumber" {
			calls.Add(1)
			time.Sleep(100 * time.Millisecond)
		}
		return value
	})
	app, _ := catalog.Get("ens")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	started := time.Now()
	result := Verify(ctx, server.Client(), server.URL, testWallet, testHash, app)
	if result.Status != "unavailable" || !strings.Contains(result.Message, "超时") || time.Since(started) > 250*time.Millisecond || calls.Load() != 2 {
		t.Fatalf("总时限未生效: %+v", result)
	}
	if requestTimeout != 8*time.Second {
		t.Fatal("默认总超时应为 8 秒")
	}
}

// TestInputAndDirectoryCannotExpandScope 验证非法输入、非安全端点与改写后的合约切片无法扩展核对范围。
func TestInputAndDirectoryCannotExpandScope(t *testing.T) {
	app, _ := catalog.Get("ens")
	for _, testCase := range []struct {
		name, endpoint, wallet, hash, status string
		app                                  catalog.App
	}{
		{name: "bad_wallet", endpoint: "https://example.com", wallet: "0x1", hash: testHash, app: app, status: "rejected"},
		{name: "bad_hash", endpoint: "https://example.com", wallet: testWallet, hash: "0x2", app: app, status: "rejected"},
		{name: "missing_endpoint", wallet: testWallet, hash: testHash, app: app, status: "unavailable"},
		{name: "insecure_endpoint", endpoint: "http://example.com", wallet: testWallet, hash: testHash, app: app, status: "unavailable"},
		{name: "userinfo_endpoint", endpoint: "https://name:secret@example.com", wallet: testWallet, hash: testHash, app: app, status: "unavailable"},
		{name: "unknown_app", endpoint: "https://example.com", wallet: testWallet, hash: testHash, app: catalog.App{ID: "unknown", ChainID: 1, Contracts: app.Contracts}, status: "rejected"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			result := Verify(context.Background(), nil, testCase.endpoint, testCase.wallet, testCase.hash, testCase.app)
			if result.Status != testCase.status {
				t.Fatalf("结果 = %+v", result)
			}
		})
	}
	t.Run("modified_contracts", func(t *testing.T) {
		// 调用方和上游同时使用目录外目标，也不能覆盖内部可信目录。
		foreignTarget := "0x" + strings.Repeat("5", 40)
		server, _ := startRPC(t, func(method string, value map[string]any) any {
			if method == "eth_getTransactionByHash" || method == "eth_getTransactionReceipt" {
				value["result"].(map[string]any)["to"] = foreignTarget
			}
			return value
		})
		app.Contracts = []string{foreignTarget}
		result := Verify(context.Background(), server.Client(), server.URL, testWallet, testHash, app)
		if result.Status != "rejected" {
			t.Fatalf("目录范围被扩展: %+v", result)
		}
	})
}
