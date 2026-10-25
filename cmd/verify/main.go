// Command verify 通过有界 HTTP 请求验收站点身份、持久化与内容权限。
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/sha3"
	"usedit/internal/community"
	"usedit/internal/wallet"
)

const (
	inputLimit    = 64 << 10
	responseLimit = 2 << 20
	requestLimit  = 80
)

type configuration struct {
	URL          string `json:"url"`
	ServiceToken string `json:"service_token,omitempty"`
}

type verifier struct {
	base         *url.URL
	serviceToken string
	remaining    int
	postID       string
	postTitle    string
	postStarted  bool
	postDeleted  bool
}

type visitor struct {
	owner  *verifier
	client *http.Client
}

type sessionResult struct {
	User community.User `json:"user"`
}

type app struct {
	ID   string   `json:"id"`
	Tags []string `json:"tags"`
}

type postResult struct {
	Post community.Post `json:"post"`
}

type collectionResult struct {
	Collection community.Collection `json:"collection"`
}

type statusError struct{ got, want int }

// Error 只输出状态码，不输出可能包含凭据或账户资料的响应正文。
func (failure statusError) Error() string {
	return fmt.Sprintf("HTTP %d，期望 %d", failure.got, failure.want)
}

// main 只从标准输入读取配置，并以不含凭据的简短状态报告退出。
func main() {
	// 参数会进入进程列表，因此配置入口只保留标准输入。
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "FAIL 请通过标准输入提供配置，不接受命令行参数")
		os.Exit(1)
	}
	config, readErr := readConfiguration(os.Stdin)
	if readErr != nil {
		fmt.Fprintln(os.Stderr, "FAIL", readErr)
		os.Exit(1)
	}
	if verifyErr := verify(config); verifyErr != nil {
		fmt.Fprintln(os.Stderr, "FAIL", verifyErr)
		os.Exit(1)
	}
	fmt.Println("PASS 站点 HTTP 验收完成，本次发布记录已删除")
	fmt.Println("范围：外部 RPC 核对与浏览器钱包扩展交互不在本脚本验收范围内。")
}

// readConfiguration 限制输入大小和字段，禁止把访问凭据放进地址或日志。
func readConfiguration(source io.Reader) (configuration, error) {
	var config configuration
	// 多读取一个字节区分刚好达到上限和被截断的超大输入。
	input, readErr := io.ReadAll(io.LimitReader(source, inputLimit+1))
	if readErr != nil || len(input) > inputLimit {
		return config, errors.New("配置读取失败或超过大小限制")
	}
	defer clear(input)
	if !bytes.HasPrefix(bytes.TrimSpace(input), []byte("{")) {
		return config, errors.New("配置必须是 JSON 对象")
	}
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	if decodeErr := decoder.Decode(&config); decodeErr != nil {
		return configuration{}, errors.New("配置 JSON 格式或字段不正确")
	}
	var extra any
	if trailingErr := decoder.Decode(&extra); trailingErr != io.EOF {
		return configuration{}, errors.New("只能提交一份配置")
	}
	if strings.IndexFunc(config.ServiceToken, func(character rune) bool { return character < 32 || character == 127 }) >= 0 {
		return configuration{}, errors.New("访问凭据包含无效字符")
	}
	return config, nil
}

// newVerifier 只允许明确指定的站点根地址，本地回环服务可以使用 HTTP。
func newVerifier(config configuration) (*verifier, error) {
	// 禁止用户信息、路径和查询参数，避免凭据借地址进入诊断信息。
	base, parseErr := url.Parse(config.URL)
	if parseErr != nil || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Path != "" && base.Path != "/") {
		return nil, errors.New("请提供不带路径、查询参数或凭据的站点根地址")
	}
	local := strings.EqualFold(base.Hostname(), "localhost")
	if address := net.ParseIP(base.Hostname()); address != nil && address.IsLoopback() {
		local = true
	}
	if base.Scheme != "https" && !(base.Scheme == "http" && local) {
		return nil, errors.New("站点必须使用 HTTPS，本地回环地址除外")
	}
	base.Path, base.RawPath = "", ""
	return &verifier{base: base, serviceToken: config.ServiceToken, remaining: requestLimit}, nil
}

// newVisitor 使用独立 CookieJar；所有请求禁止跳转以防站点凭据发往其他地址。
func (run *verifier) newVisitor() (*visitor, error) {
	jar, jarErr := cookiejar.New(nil)
	if jarErr != nil {
		return nil, errors.New("无法创建临时会话容器")
	}
	return &visitor{owner: run, client: &http.Client{Jar: jar, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// request 在请求次数、时间和响应体积限制内调用唯一目标站点，不记录请求正文或凭据。
func (browser *visitor) request(ctx context.Context, method, path string, input any, expected int, output any, key string) ([]byte, error) {
	// 所有访问端共享次数预算，失败清理也计入相同上限。
	if browser.owner.remaining <= 0 {
		return nil, errors.New("已达到验收请求次数上限")
	}
	browser.owner.remaining--
	var body []byte
	if input != nil {
		encoded, encodeErr := json.Marshal(input)
		if encodeErr != nil {
			return nil, errors.New("请求内容编码失败")
		}
		body = encoded
	}
	request, requestErr := http.NewRequestWithContext(ctx, method, browser.owner.base.String()+"/api/v1"+path, bytes.NewReader(body))
	if requestErr != nil {
		return nil, errors.New("请求地址构建失败")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Origin", browser.owner.base.String())
	request.Header.Set("X-UsedIt-Request", "1")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if browser.owner.serviceToken != "" {
		// 仅在明确选定站点的请求头中携带访问凭据，不复制到 URL 或正文。
		request.Header.Set("OAI-Sites-Authorization", browser.owner.serviceToken)
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	response, transportErr := browser.client.Do(request)
	if transportErr != nil {
		return nil, errors.New("站点请求失败或超时")
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(response.Body, responseLimit+1))
	if readErr != nil || len(data) > responseLimit {
		return nil, errors.New("响应读取失败或超过大小限制")
	}
	if response.StatusCode != expected {
		return nil, statusError{got: response.StatusCode, want: expected}
	}
	if output != nil {
		if decodeErr := json.Unmarshal(data, output); decodeErr != nil {
			return nil, errors.New("响应 JSON 格式不正确")
		}
	}
	return data, nil
}

// check 为一次业务断言附上固定步骤名，不传播原始请求或响应内容。
func check(label string, cause error) error {
	if cause != nil {
		return fmt.Errorf("%s：%w", label, cause)
	}
	return nil
}

// sameCookie 检查登录是否轮换站点会话，令牌始终只保留在内存中。
func sameCookie(before, after []*http.Cookie) bool {
	// 只比较业务会话 Cookie，站点网关的其他 Cookie 不影响轮换断言。
	for _, previous := range before {
		if previous.Name != "usedit_session" {
			continue
		}
		for _, current := range after {
			if current.Name == previous.Name && current.Value != "" && current.Value == previous.Value {
				return true
			}
		}
	}
	return false
}

// identifier 拒绝缺失和路径控制字符，服务端标识不能改变后续请求范围。
func identifier(value string) bool {
	// 标识进入 URL 路径前限定字符集，拒绝斜线、百分号和查询分隔符。
	if len(value) < 1 || len(value) > 100 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_') {
			return false
		}
	}
	return true
}

// collectionInput 只选择目录资料，不向个人页面写入账户地址或交易信息。
func collectionInput(title, appID, referenceID string, version int) map[string]any {
	return map[string]any{"title": title, "description": "服务验收记录：检查内容选择、页面版本和重新请求后的持久化。", "instructions": "", "layout": "editorial", "theme": "sage", "app_ids": []string{appID}, "post_ids": []string{referenceID}, "version": version}
}

// publicationInput 明确标识服务验收用途，不声称发生过应用或链上交互。
func publicationInput(title, appID string, tags []string, revision int) map[string]any {
	return map[string]any{"title": title, "app_id": appID, "body": "服务验收记录：此条目仅用于检查发布、重复提交、内容编辑与权限控制。验收结束后删除，不代表对该应用的实际使用评价或链上交互。", "sentiment": "neutral", "tags": tags, "revision": revision}
}

// verify 串联完整业务流程，临时私钥不落盘，失败时仍尝试清理刚发布的记录。
func verify(config configuration) (runErr error) {
	run, setupErr := newVerifier(config)
	if setupErr != nil {
		return setupErr
	}
	browser, browserErr := run.newVisitor()
	if browserErr != nil {
		return browserErr
	}
	interrupted, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	ctx, cancel := context.WithTimeout(interrupted, 3*time.Minute)
	defer cancel()
	// 清理使用独立的短时上下文，主流程超时后仍能删除本次记录。
	defer func() {
		if run.postStarted && !run.postDeleted {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cleanupCancel()
			if cleanupErr := run.cleanup(cleanupCtx, browser); cleanupErr != nil {
				runErr = errors.Join(runErr, errors.New("本次发布记录清理未完成，需再次核对临时验收账户"))
			}
		}
	}()
	key, keyErr := secp256k1.GeneratePrivateKey()
	if keyErr != nil {
		return errors.New("临时签名密钥创建失败")
	}
	defer key.Zero()
	// 一次运行使用唯一标题与幂等键，清理不能碰到其他账户或其他内容。
	random := make([]byte, 16)
	if _, randomErr := rand.Read(random); randomErr != nil {
		return errors.New("验收标识创建失败")
	}
	random[6] = random[6]&0x0f | 0x40
	random[8] = random[8]&0x3f | 0x80
	runID := fmt.Sprintf("%x-%x-%x-%x-%x", random[:4], random[4:6], random[6:8], random[8:10], random[10:])
	run.postTitle = "服务验收记录 " + runID[:8]
	var guest sessionResult
	// 先建立无钱包身份，后续才能验证收藏和精选确实从游客迁移。
	if _, requestErr := browser.request(ctx, "GET", "/session", nil, 200, &guest, ""); requestErr != nil {
		return check("游客会话", requestErr)
	}
	if guest.User.ID == "" || guest.User.Wallet != nil {
		return errors.New("游客会话未创建独立身份")
	}
	var apps struct{ Items []app }
	if _, requestErr := browser.request(ctx, "GET", "/apps", nil, 200, &apps, ""); requestErr != nil {
		return check("应用目录", requestErr)
	}
	if len(apps.Items) == 0 || !identifier(apps.Items[0].ID) || len(apps.Items[0].Tags) == 0 {
		return errors.New("应用目录缺少可验收条目")
	}
	selected := apps.Items[0]
	referenceID := "reference-" + selected.ID
	var reference postResult
	if _, requestErr := browser.request(ctx, "GET", "/posts/"+referenceID, nil, 200, &reference, ""); requestErr != nil {
		return check("目录资料", requestErr)
	}
	if reference.Post.Kind != "reference" || reference.Post.SourceURL == "" {
		return errors.New("目录资料缺少类型或来源")
	}
	var feed struct{ Items []community.Post }
	if _, requestErr := browser.request(ctx, "GET", "/feed?tab=for-you", nil, 200, &feed, ""); requestErr != nil {
		return check("推荐列表", requestErr)
	}
	for _, active := range []bool{true, true} {
		if _, requestErr := browser.request(ctx, "PUT", "/posts/"+referenceID+"/bookmark", map[string]bool{"active": active}, 200, nil, ""); requestErr != nil {
			return check("重复收藏", requestErr)
		}
	}
	if _, requestErr := browser.request(ctx, "GET", "/feed?tab=saved", nil, 200, &feed, ""); requestErr != nil {
		return check("收藏列表", requestErr)
	}
	if len(feed.Items) != 1 || feed.Items[0].ID != referenceID || !feed.Items[0].Bookmarked {
		return errors.New("重复收藏未保持唯一记录")
	}
	if _, requestErr := browser.request(ctx, "PUT", "/posts/"+referenceID+"/bookmark", map[string]bool{"active": false}, 200, nil, ""); requestErr != nil {
		return check("取消收藏", requestErr)
	}
	if _, requestErr := browser.request(ctx, "GET", "/feed?tab=saved", nil, 200, &feed, ""); requestErr != nil || len(feed.Items) != 0 {
		return errors.New("取消收藏后列表未清空")
	}
	var preferences sessionResult
	tags := []string{selected.Tags[0]}
	if _, requestErr := browser.request(ctx, "PUT", "/preferences", map[string]any{"tags": tags}, 200, &preferences, ""); requestErr != nil {
		return check("兴趣设置", requestErr)
	}
	if len(preferences.User.Preferences) != 1 || preferences.User.Preferences[0] != tags[0] {
		return errors.New("兴趣设置未保存")
	}
	if _, requestErr := browser.request(ctx, "PUT", "/posts/"+referenceID+"/bookmark", map[string]bool{"active": true}, 200, nil, ""); requestErr != nil {
		return check("迁移前收藏", requestErr)
	}
	var initial collectionResult
	title := "服务验收精选 " + runID[:8]
	if _, requestErr := browser.request(ctx, "POST", "/collections", collectionInput(title, selected.ID, referenceID, 0), 201, &initial, ""); requestErr != nil {
		return check("游客精选保存", requestErr)
	}
	if !identifier(initial.Collection.ID) || initial.Collection.Version != 1 || initial.Collection.HTML == "" {
		return errors.New("游客精选首版内容不完整")
	}
	fmt.Println("PASS 游客会话、目录、推荐、收藏和兴趣")
	// 登录只验证个人消息签名，不请求交易授权，也不提交链上凭据。
	account, loginErr := browser.login(ctx, key)
	if loginErr != nil {
		return loginErr
	}
	if account.ID == guest.User.ID || len(account.Preferences) != 1 || account.Preferences[0] != tags[0] {
		return errors.New("钱包身份未接收游客偏好")
	}
	if _, requestErr := browser.request(ctx, "GET", "/posts/"+referenceID, nil, 200, &reference, ""); requestErr != nil || !reference.Post.Bookmarked {
		return errors.New("游客收藏未迁移到钱包身份")
	}
	collectionPath := "/collections/" + initial.Collection.ID
	var migrated collectionResult
	if _, requestErr := browser.request(ctx, "GET", collectionPath, nil, 200, &migrated, ""); requestErr != nil || migrated.Collection.HTML != initial.Collection.HTML {
		return errors.New("游客精选未完整迁移到钱包身份")
	}
	fmt.Println("PASS 真实钱包签名、会话轮换和游客内容迁移")
	run.postStarted = true
	// 在发送创建请求前登记清理意图，响应丢失时仍可按本次标题查找。
	publishedInput := publicationInput(run.postTitle, selected.ID, tags, 0)
	var published postResult
	if _, requestErr := browser.request(ctx, "POST", "/posts", publishedInput, 201, &published, runID); requestErr != nil {
		return check("发布验收记录", requestErr)
	}
	if !identifier(published.Post.ID) || published.Post.ProofStatus != "none" || published.Post.TxHash != "" || !published.Post.Owned || published.Post.Title != run.postTitle || published.Post.AppID != selected.ID || published.Post.Revision != 1 {
		return errors.New("新记录的身份或凭据状态不正确")
	}
	run.postID = published.Post.ID
	var repeated postResult
	if _, requestErr := browser.request(ctx, "POST", "/posts", publishedInput, 201, &repeated, runID); requestErr != nil || repeated.Post.ID != run.postID {
		return errors.New("重复发布未返回同一记录")
	}
	if _, requestErr := browser.request(ctx, "POST", "/posts", publicationInput(run.postTitle+"冲突", selected.ID, tags, 0), 409, nil, runID); requestErr != nil {
		return check("发布幂等冲突", requestErr)
	}
	postPath := "/posts/" + run.postID
	updatedInput := publicationInput(run.postTitle+" 已编辑", selected.ID, tags, published.Post.Revision)
	var updated postResult
	if _, requestErr := browser.request(ctx, "PATCH", postPath, updatedInput, 200, &updated, ""); requestErr != nil {
		return check("记录编辑", requestErr)
	}
	if updated.Post.Revision != published.Post.Revision+1 || updated.Post.Title != run.postTitle+" 已编辑" {
		return errors.New("编辑未保存正文或增加修订号")
	}
	if _, requestErr := browser.request(ctx, "PATCH", postPath, updatedInput, 409, nil, ""); requestErr != nil {
		return check("过期修订拒绝", requestErr)
	}
	other, otherErr := run.newVisitor()
	// 第二个独立会话只进行拒绝检查，不修改其他身份的数据。
	if otherErr != nil {
		return otherErr
	}
	if _, requestErr := other.request(ctx, "PATCH", postPath, updatedInput, 403, nil, ""); requestErr != nil {
		return check("非作者编辑拒绝", requestErr)
	}
	if _, requestErr := other.request(ctx, "DELETE", postPath, nil, 403, nil, ""); requestErr != nil {
		return check("非作者删除拒绝", requestErr)
	}
	if _, requestErr := other.request(ctx, "GET", collectionPath, nil, 404, nil, ""); requestErr != nil {
		return check("其他账户精选隔离", requestErr)
	}
	fmt.Println("PASS 发布幂等、内容修订和非作者权限")
	var second collectionResult
	if _, requestErr := browser.request(ctx, "PATCH", collectionPath, collectionInput(title+" 第二版", selected.ID, referenceID, 1), 200, &second, ""); requestErr != nil {
		return check("精选版本保存", requestErr)
	}
	if second.Collection.Version != 2 || second.Collection.Title != title+" 第二版" || second.Collection.HTML == initial.Collection.HTML {
		return errors.New("精选页未生成第二版")
	}
	if _, requestErr := browser.request(ctx, "PATCH", collectionPath, collectionInput(title, selected.ID, referenceID, 1), 409, nil, ""); requestErr != nil {
		return check("精选版本冲突", requestErr)
	}
	var history collectionResult
	if _, requestErr := browser.request(ctx, "GET", collectionPath+"/versions/1", nil, 200, &history, ""); requestErr != nil || history.Collection.HTML != initial.Collection.HTML {
		return errors.New("保存第二版改变了第一版历史")
	}
	exported, exportErr := browser.request(ctx, "GET", collectionPath+"/export", nil, 200, nil, "")
	if exportErr != nil || string(exported) != second.Collection.HTML {
		return errors.New("精选导出与当前版本不一致")
	}
	restored, restoreErr := run.newVisitor()
	if restoreErr != nil {
		return restoreErr
	}
	restored.client.Jar.SetCookies(run.base, browser.client.Jar.Cookies(run.base))
	// 新 HTTP 客户端仅复用会话 Cookie，正文和版本必须重新从服务端读取。
	var restoredSession sessionResult
	if _, requestErr := restored.request(ctx, "GET", "/session", nil, 200, &restoredSession, ""); requestErr != nil || restoredSession.User.ID != account.ID {
		return errors.New("独立请求未恢复同一钱包会话")
	}
	var restoredPage collectionResult
	if _, requestErr := restored.request(ctx, "GET", collectionPath, nil, 200, &restoredPage, ""); requestErr != nil || restoredPage.Collection.Version != 2 || restoredPage.Collection.HTML != second.Collection.HTML {
		return errors.New("独立请求未恢复已保存精选版本")
	}
	var restoredPost postResult
	if _, requestErr := restored.request(ctx, "GET", postPath, nil, 200, &restoredPost, ""); requestErr != nil || restoredPost.Post.Revision != updated.Post.Revision || restoredPost.Post.Title != updated.Post.Title {
		return errors.New("独立请求未恢复已编辑内容")
	}
	fmt.Println("PASS 精选版本、历史、导出和独立请求恢复")
	if cleanupErr := run.cleanup(ctx, browser); cleanupErr != nil {
		return cleanupErr
	}
	// 若实现错误导致删除后的重放重新创建记录，失败清理仍须再次执行。
	run.postDeleted = false
	if _, requestErr := browser.request(ctx, "POST", "/posts", publishedInput, 404, nil, runID); requestErr != nil {
		return check("删除后幂等重放", requestErr)
	}
	run.postDeleted = true
	if _, requestErr := browser.request(ctx, "PUT", "/posts/"+referenceID+"/bookmark", map[string]bool{"active": false}, 200, nil, ""); requestErr != nil {
		return check("清理本次收藏", requestErr)
	}
	return nil
}

// login 只对服务端挑战进行真实签名，验证站点、钱包归属及会话令牌轮换。
func (browser *visitor) login(ctx context.Context, key *secp256k1.PrivateKey) (community.User, error) {
	// 从实际公钥推导账户地址，签名结果需由服务端恢复为同一账户。
	hash := sha3.NewLegacyKeccak256()
	hash.Write(key.PubKey().SerializeUncompressed()[1:])
	address := "0x" + hex.EncodeToString(hash.Sum(nil)[12:])
	var challenge struct {
		ID      string `json:"challenge_id"`
		Message string `json:"message"`
	}
	if _, requestErr := browser.request(ctx, "POST", "/auth/challenge", map[string]string{"address": address}, 200, &challenge, ""); requestErr != nil {
		return community.User{}, check("钱包挑战", requestErr)
	}
	if challenge.ID == "" || len(challenge.Message) > 4096 || !strings.HasPrefix(challenge.Message, browser.owner.base.Host+" wants you to sign in") || !strings.Contains(challenge.Message, "\nURI: "+browser.owner.base.String()+"\n") || !strings.Contains(challenge.Message, "\n"+address+"\n") {
		return community.User{}, errors.New("签名挑战与目标站点或临时钱包不一致")
	}
	compact := ecdsa.SignCompact(key, wallet.Digest(challenge.Message), false)
	// 把曲线库的前置恢复位转换为个人消息协议的末尾恢复位。
	personal := append(append([]byte{}, compact[1:]...), compact[0])
	before := browser.client.Jar.Cookies(browser.owner.base)
	var verified sessionResult
	if _, requestErr := browser.request(ctx, "POST", "/auth/verify", map[string]string{"challenge_id": challenge.ID, "signature": "0x" + hex.EncodeToString(personal)}, 200, &verified, ""); requestErr != nil {
		return community.User{}, check("钱包签名验证", requestErr)
	}
	if verified.User.Wallet == nil || *verified.User.Wallet != address || sameCookie(before, browser.client.Jar.Cookies(browser.owner.base)) {
		return community.User{}, errors.New("钱包登录未确认账户或未轮换会话")
	}
	return verified.User, nil
}

// cleanup 仅查找当前临时账户且标题属于本次运行的记录，并核对软删除结果。
func (run *verifier) cleanup(ctx context.Context, browser *visitor) error {
	// 只从当前账户列表查找本次唯一标题，不能依据公共信息流执行删除。
	var mine struct{ Items []community.Post }
	if _, requestErr := browser.request(ctx, "GET", "/me/posts", nil, 200, &mine, ""); requestErr != nil {
		return check("查找本次验收记录", requestErr)
	}
	for _, post := range mine.Items {
		if (post.Title != run.postTitle && post.Title != run.postTitle+" 已编辑") || !identifier(post.ID) {
			continue
		}
		if _, requestErr := browser.request(ctx, "DELETE", "/posts/"+post.ID, nil, 200, nil, ""); requestErr != nil {
			return check("清理本次验收记录", requestErr)
		}
		if _, requestErr := browser.request(ctx, "GET", "/posts/"+post.ID, nil, 404, nil, ""); requestErr != nil {
			return check("检查记录删除结果", requestErr)
		}
	}
	if run.postID != "" {
		if _, requestErr := browser.request(ctx, "GET", "/posts/"+run.postID, nil, 404, nil, ""); requestErr != nil {
			return check("确认本次记录不可读取", requestErr)
		}
	}
	run.postDeleted = true
	return nil
}
