// Package proof 核对主网交易的归属、直接目标、成功状态与确认深度。
package proof

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"usedit/internal/catalog"
)

const (
	RequiredConfirmations uint64 = 12
	requestTimeout               = 8 * time.Second
	maxResponseBytes             = 512 << 10
)

// Result 记录检查时的状态；确认深度不等于最终性或质量背书。
type Result struct {
	Status        string `json:"status"`
	Message       string `json:"message"`
	CheckedAt     string `json:"checked_at"`
	BlockNumber   uint64 `json:"block_number"`
	BlockHash     string `json:"block_hash"`
	Confirmations uint64 `json:"confirmations"`
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code int `json:"code"`
}

type transaction struct {
	Hash        string  `json:"hash"`
	From        string  `json:"from"`
	To          *string `json:"to"`
	ChainID     *string `json:"chainId"`
	BlockNumber *string `json:"blockNumber"`
	BlockHash   *string `json:"blockHash"`
}

type receipt struct {
	TransactionHash string  `json:"transactionHash"`
	From            string  `json:"from"`
	To              *string `json:"to"`
	Status          string  `json:"status"`
	BlockNumber     string  `json:"blockNumber"`
	BlockHash       string  `json:"blockHash"`
}

type block struct {
	Number string `json:"number"`
	Hash   string `json:"hash"`
}

// Verify 只接受服务端提供的数据源配置。第一批固定四次查询，确认充分后再查一次主链区块；不扫描历史或自动重试。
func Verify(parent context.Context, client *http.Client, rpcURL, address, txHash string, app catalog.App) Result {
	result := Result{CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	if !validHex(address, 20) || !validHex(txHash, 32) {
		return result.finish("rejected", "钱包地址或交易哈希格式不正确。", "invalid_input")
	}
	// 凭据范围以内部目录为准，调用方不能扩展链或合约集合。
	knownApp, found := catalog.Get(app.ID)
	if !found || knownApp.ChainID != 1 {
		return result.finish("rejected", "该应用暂不在凭据核对范围内。", "unsupported_app")
	}
	if !validEndpoint(rpcURL) {
		return result.finish("unavailable", "链上数据源尚未配置或配置无效，请稍后重试。", "invalid_endpoint")
	}
	ctx, cancel := context.WithTimeout(parent, requestTimeout)
	defer cancel()
	requestClient := http.Client{}
	if client != nil {
		requestClient = *client
	}
	// 不跟随跳转，避免将带鉴权路径的请求转交给其他主机。
	requestClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	responses, callError := call(ctx, &requestClient, rpcURL, []rpcRequest{
		{JSONRPC: "2.0", ID: 1, Method: "eth_chainId", Params: []any{}},
		{JSONRPC: "2.0", ID: 2, Method: "eth_getTransactionByHash", Params: []any{txHash}},
		{JSONRPC: "2.0", ID: 3, Method: "eth_getTransactionReceipt", Params: []any{txHash}},
		{JSONRPC: "2.0", ID: 4, Method: "eth_blockNumber", Params: []any{}},
	})
	if callError != nil {
		return unavailable(result, ctx, callError)
	}
	var chainText, latestText string
	if json.Unmarshal(responses[1], &chainText) != nil || json.Unmarshal(responses[4], &latestText) != nil {
		return result.finish("unavailable", "链上数据源返回格式异常，请稍后重试。", "quantity_type")
	}
	chainID, chainValid := quantity(chainText)
	latestNumber, latestValid := quantity(latestText)
	if !chainValid || !latestValid || chainID != knownApp.ChainID {
		return result.finish("unavailable", "链上数据源的网络或区块信息不符合要求。", "network_or_height")
	}
	var tx *transaction
	var txReceipt *receipt
	txError := json.Unmarshal(responses[2], &tx)
	receiptError := json.Unmarshal(responses[3], &txReceipt)
	if txError != nil || receiptError != nil {
		return result.finish("unavailable", "交易资料暂时无法读取，请稍后重试。", "transaction_format")
	}
	if tx == nil {
		if txReceipt != nil {
			return result.finish("unavailable", "链上数据源的交易资料不一致，请稍后重试。", "receipt_without_transaction")
		}
		return result.finish("pending", "尚未查询到交易，请确认使用 Ethereum 主网后稍后重试。", "transaction_missing")
	}
	if !validHex(tx.Hash, 32) || !same(tx.Hash, txHash) || !validHex(tx.From, 20) || (tx.To != nil && !validHex(*tx.To, 20)) {
		return result.finish("unavailable", "链上数据源返回的交易字段不完整或不一致。", "transaction_fields")
	}
	if tx.ChainID != nil {
		txChain, validChain := quantity(*tx.ChainID)
		if !validChain || txChain != knownApp.ChainID {
			return result.finish("unavailable", "链上数据源的交易网络不符合要求。", "transaction_chain")
		}
	}
	if !same(tx.From, address) {
		return result.finish("rejected", "这笔交易的发送者不是当前钱包。", "sender_mismatch")
	}
	if tx.To == nil || !containsContract(knownApp.Contracts, *tx.To) {
		return result.finish("rejected", "这笔交易没有直接调用该应用已收录的主网合约。", "target_mismatch")
	}
	if txReceipt == nil {
		return result.finish("pending", "交易尚未取得回执，请稍后重试。", "receipt_pending")
	}
	if !validHex(txReceipt.TransactionHash, 32) || !same(txReceipt.TransactionHash, txHash) ||
		!validHex(txReceipt.From, 20) || !same(txReceipt.From, tx.From) || txReceipt.To == nil ||
		!validHex(*txReceipt.To, 20) || !same(*txReceipt.To, *tx.To) || !validHex(txReceipt.BlockHash, 32) {
		return result.finish("unavailable", "链上数据源的交易与回执不一致，请稍后重试。", "receipt_fields")
	}
	status, validStatus := quantity(txReceipt.Status)
	blockNumber, validBlock := quantity(txReceipt.BlockNumber)
	if !validStatus || status > 1 || !validBlock || tx.BlockNumber == nil || tx.BlockHash == nil || !validHex(*tx.BlockHash, 32) {
		return result.finish("unavailable", "链上数据源的回执字段不完整，请稍后重试。", "receipt_status_or_block")
	}
	txBlock, validTxBlock := quantity(*tx.BlockNumber)
	if !validTxBlock || txBlock != blockNumber || !same(*tx.BlockHash, txReceipt.BlockHash) {
		return result.finish("pending", "交易的区块资料正在变化，请稍后重试。", "transaction_block_changed")
	}
	result.BlockNumber, result.BlockHash = blockNumber, strings.ToLower(txReceipt.BlockHash)
	if status == 0 {
		return result.finish("rejected", "这笔交易执行失败，不能作为成功交互凭据。", "execution_failed")
	}
	if latestNumber < blockNumber {
		return result.finish("pending", "链上数据源尚未同步到交易区块，请稍后重试。", "node_behind")
	}
	// 以包含交易的区块计为第一个确认；差值上限检查避免异常高度溢出。
	distance := latestNumber - blockNumber
	if distance == ^uint64(0) {
		return result.finish("unavailable", "链上数据源的区块高度异常，请稍后重试。", "height_overflow")
	}
	result.Confirmations = distance + 1
	if result.Confirmations < RequiredConfirmations {
		return result.finish("pending", fmt.Sprintf("已取得 %d/%d 个区块确认，请稍后重试。", result.Confirmations, RequiredConfirmations), "confirmation_pending")
	}
	canonical, canonicalError := call(ctx, &requestClient, rpcURL, []rpcRequest{
		{JSONRPC: "2.0", ID: 5, Method: "eth_getBlockByNumber", Params: []any{txReceipt.BlockNumber, false}},
	})
	if canonicalError != nil {
		return unavailable(result, ctx, canonicalError)
	}
	var canonicalBlock *block
	if decodeError := json.Unmarshal(canonical[5], &canonicalBlock); decodeError != nil {
		return result.finish("unavailable", "链上数据源返回的区块格式异常，请稍后重试。", "block_format")
	}
	if canonicalBlock == nil {
		return result.finish("pending", "交易区块暂时无法确认，请稍后重试。", "block_missing")
	}
	canonicalNumber, validCanonical := quantity(canonicalBlock.Number)
	if !validCanonical || canonicalNumber != blockNumber || !validHex(canonicalBlock.Hash, 32) {
		return result.finish("unavailable", "链上数据源返回的区块字段不一致，请稍后重试。", "block_fields")
	}
	if !same(canonicalBlock.Hash, txReceipt.BlockHash) {
		return result.finish("pending", "交易区块已发生变化，请稍后重新核对。", "block_changed")
	}
	result.Status = "verified"
	result.Message = "已核对当前钱包对目录合约的成功直接交互；不代表评价质量或应用安全。"
	return result
}

// finish 只记录固定原因码，不记录地址、交易正文或带密钥的数据源地址。
func (result Result) finish(status, message, reason string) Result {
	result.Status, result.Message = status, message
	slog.Warn("凭据核对未完成", "status", status, "reason", reason)
	return result
}

// unavailable 区分超时、取消与其他查询失败，对外仅返回固定的安全提示。
func unavailable(result Result, ctx context.Context, callError error) Result {
	// 客户端自身的超时可能早于父上下文，因此同时检查传输错误与上下文。
	if errors.Is(callError, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return result.finish("unavailable", "链上查询超时，请稍后手动重试。", "rpc_timeout")
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return result.finish("unavailable", "本次链上查询已取消，可以稍后重试。", "rpc_canceled")
	}
	return result.finish("unavailable", "链上数据源暂时不可用，请稍后手动重试。", "rpc_unavailable")
}

// call 执行一次有界批量请求，按唯一响应标识关联结果并拒绝上游错误与异常结构。
func call(ctx context.Context, client *http.Client, endpoint string, requests []rpcRequest) (map[int]json.RawMessage, error) {
	requestBody, encodeError := json.Marshal(requests)
	if encodeError != nil {
		return nil, rpcFailure("request_encode")
	}
	request, requestError := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if requestError != nil {
		return nil, rpcFailure("request_create")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if runtime.GOOS == "js" {
		// Go 的 Fetch 适配在传输层处理跳转，需显式阻止并关闭环境凭据。
		request.Header.Set("js.fetch:redirect", "error")
		request.Header.Set("js.fetch:credentials", "omit")
	}
	response, transportError := client.Do(request)
	if transportError != nil {
		slog.Warn("链上查询传输失败", "reason", "transport")
		// 保留超时语义，同时不传播可能含有敏感路径的原始传输错误。
		if errors.Is(transportError, context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		return nil, errors.New("rpc_transport")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, rpcFailure("http_status")
	}
	if response.ContentLength > maxResponseBytes {
		return nil, rpcFailure("body_limit")
	}
	responseBody, readError := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if readError != nil || len(responseBody) > maxResponseBytes {
		return nil, rpcFailure("body_read_or_limit")
	}
	var responses []rpcResponse
	if decodeError := json.Unmarshal(responseBody, &responses); decodeError != nil || len(responses) != len(requests) {
		return nil, rpcFailure("batch_format")
	}
	expected := make(map[int]bool, len(requests))
	for _, request := range requests {
		expected[request.ID] = true
	}
	result := make(map[int]json.RawMessage, len(requests))
	for _, response := range responses {
		if response.JSONRPC != "2.0" || !expected[response.ID] || result[response.ID] != nil || response.Error != nil || len(response.Result) == 0 {
			return nil, rpcFailure("batch_fields")
		}
		result[response.ID] = response.Result
	}
	return result, nil
}

// rpcFailure 在发现协议异常的位置记录固定原因码，避免泄漏数据源正文。
func rpcFailure(reason string) error {
	slog.Warn("链上查询响应无效", "reason", reason)
	return errors.New(reason)
}

// validEndpoint 仅接受 HTTPS 或本地回环 HTTP，并拒绝用户信息与片段标记。
func validEndpoint(endpoint string) bool {
	parsed, parseError := url.Parse(endpoint)
	if parseError != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	// 明文传输只对本机协议测试开放，普通域名和非回环地址不能走此分支。
	if parsed.Scheme != "http" {
		return false
	}
	if strings.EqualFold(parsed.Hostname(), "localhost") {
		return true
	}
	parsedIP := net.ParseIP(parsed.Hostname())
	return parsedIP != nil && parsedIP.IsLoopback()
}

// validHex 校验带 0x 前缀的定长十六进制字段，避免截短地址或哈希参与比较。
func validHex(value string, byteLength int) bool {
	if len(value) != 2+byteLength*2 || !strings.HasPrefix(value, "0x") {
		return false
	}
	_, decodeError := hex.DecodeString(value[2:])
	return decodeError == nil
}

// quantity 解析无符号 RPC 数量，拒绝多余前导零、符号、非法字符及溢出。
func quantity(value string) (uint64, bool) {
	if len(value) < 3 || !strings.HasPrefix(value, "0x") || (len(value) > 3 && value[2] == '0') {
		return 0, false
	}
	// 数量字段必须完全由十六进制数字构成，再交给定宽解析器检查溢出。
	for _, digit := range value[2:] {
		if !((digit >= '0' && digit <= '9') || (digit >= 'a' && digit <= 'f') || (digit >= 'A' && digit <= 'F')) {
			return 0, false
		}
	}
	number, parseError := strconv.ParseUint(value[2:], 16, 64)
	return number, parseError == nil
}

// same 比较已校验的地址或哈希时忽略十六进制字母大小写。
func same(left, right string) bool { return strings.EqualFold(left, right) }

// containsContract 将交易的直接目标与有限可信合约集合逐一比较。
func containsContract(contracts []string, target string) bool {
	// 只比较直接目标；地址大小写差异不会扩大或缩小目录中的允许集合。
	for _, contract := range contracts {
		if same(contract, target) {
			return true
		}
	}
	return false
}
