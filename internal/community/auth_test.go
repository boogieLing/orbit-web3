package community_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"usedit/internal/community"
)

// TestWalletChallengeRotationReplayAndExpiry 验证登录轮换、旧令牌失效、挑战过期与退出。
func TestWalletChallengeRotationReplayAndExpiry(t *testing.T) {
	// 保留登录前令牌，后续独立请求必须无法恢复原游客或钱包身份。
	store := openStore(t)
	client := newBrowser(store)
	guest := client.session(t)
	oldCookie := client.cookie
	account := newSigner(t)
	issued := client.challenge(t, account)
	if !strings.Contains(issued.Message, "URI: https://usedit.test") || !strings.Contains(issued.Message, account.address) || !strings.Contains(issued.Message, "Expiration Time:") {
		t.Fatalf("挑战缺少站点、地址或有效期: %q", issued.Message)
	}
	body := map[string]string{"challenge_id": issued.ID, "signature": account.sign(issued.Message)}
	response := client.request(t, "POST", "/auth/verify", body)
	requireStatus(t, response, 200)
	user := readJSON[struct{ User community.User }](t, response).User
	if user.ID == guest.ID || user.Wallet == nil || *user.Wallet != account.address || client.cookie == oldCookie {
		t.Fatalf("身份或会话未轮换: %+v", user)
	}
	parsed := (&http.Response{Header: http.Header{"Set-Cookie": {response.Headers["set-cookie"]}}}).Cookies()
	if len(parsed) != 1 || !parsed[0].Secure || !parsed[0].HttpOnly {
		t.Fatalf("轮换会话缺少安全标志: %+v", parsed)
	}
	requireStatus(t, client.request(t, "POST", "/auth/verify", body), 401)
	stale := *client
	stale.cookie = oldCookie
	staleUser := stale.session(t)
	if staleUser.ID == user.ID || staleUser.ID == guest.ID || staleUser.Wallet != nil {
		t.Fatalf("旧令牌仍可恢复已迁移身份: %+v", staleUser)
	}
	expiring := client.challenge(t, account)
	store.now.Add(int64(5*time.Minute + time.Second))
	requireStatus(t, client.request(t, "POST", "/auth/verify", map[string]string{"challenge_id": expiring.ID, "signature": account.sign(expiring.Message)}), 401)
	newChallenge := client.challenge(t, account)
	if newChallenge.ID == expiring.ID || newChallenge.Message == expiring.Message {
		t.Fatal("刷新挑战未更换唯一内容")
	}
	activeCookie := client.cookie
	requireStatus(t, client.request(t, "POST", "/auth/logout", map[string]any{}), 200)
	stale.cookie = activeCookie
	if loggedOut := stale.session(t); loggedOut.Wallet != nil || loggedOut.ID == user.ID {
		t.Fatal("退出后旧会话仍有效")
	}
}

// TestWalletChallengeBindsOriginSessionAndAddress 阻止跨站点、协议、端口、会话和钱包复用签名。
func TestWalletChallengeBindsOriginSessionAndAddress(t *testing.T) {
	// 域名、协议和端口分别变化，任一来源差异都不能消费原挑战。
	for _, target := range []struct {
		name   string
		origin string
	}{
		{name: "host", origin: "https://different.test"},
		{name: "scheme", origin: "http://usedit.test"},
		{name: "port", origin: "https://usedit.test:8443"},
	} {
		t.Run(target.name, func(t *testing.T) {
			store := openStore(t)
			client := newBrowser(store)
			account := newSigner(t)
			issued := client.challenge(t, account)
			body := map[string]string{"challenge_id": issued.ID, "signature": account.sign(issued.Message)}
			foreign := *client
			foreign.origin = target.origin
			requireStatus(t, foreign.request(t, "POST", "/auth/verify", body), 401)
			requireStatus(t, client.request(t, "POST", "/auth/verify", body), 200)
		})
	}
	t.Run("session", func(t *testing.T) {
		store := openStore(t)
		client := newBrowser(store)
		account := newSigner(t)
		issued := client.challenge(t, account)
		body := map[string]string{"challenge_id": issued.ID, "signature": account.sign(issued.Message)}
		other := newBrowser(store)
		requireStatus(t, other.request(t, "POST", "/auth/verify", body), 401)
		wrong := newSigner(t)
		requireStatus(t, client.request(t, "POST", "/auth/verify", map[string]string{"challenge_id": issued.ID, "signature": wrong.sign(issued.Message)}), 401)
		requireStatus(t, client.request(t, "POST", "/auth/verify", body), 200)
	})
}

// TestWalletChallengeConcurrentConsumptionHasOneWinner 验证同一有效挑战只能被一个并发请求消费。
func TestWalletChallengeConcurrentConsumptionHasOneWinner(t *testing.T) {
	// 两个请求在同一屏障后发送，检查数据库原子消费而非顺序请求的结果。
	store := openStore(t)
	client := newBrowser(store)
	account := newSigner(t)
	issued := client.challenge(t, account)
	body := `{"challenge_id":"` + issued.ID + `","signature":"` + account.sign(issued.Message) + `"}`
	start := make(chan struct{})
	responses := make(chan community.Response, 2)
	for index := 0; index < 2; index++ {
		concurrent := *client
		go func() {
			<-start
			responses <- concurrent.raw("POST", "/auth/verify", body, nil)
		}()
	}
	close(start)
	winners := 0
	for index := 0; index < 2; index++ {
		response := <-responses
		if response.Status == 200 {
			winners++
		} else if response.Status != 401 && response.Status != 409 {
			t.Errorf("并发消费返回非预期状态: %d %s", response.Status, response.Body)
		}
	}
	if winners != 1 {
		t.Fatalf("同一挑战成功次数为 %d", winners)
	}
	if count := store.count(t, "SELECT count(*) FROM challenges WHERE id=? AND consumed=1", issued.ID); count != 1 {
		t.Fatalf("挑战消费状态不正确: %d", count)
	}
}

// TestGuestCollectionsAndActionsMigrateWithoutDoubleCounting 检查游客内容迁移、互动去重和重启恢复。
func TestGuestCollectionsAndActionsMigrateWithoutDoubleCounting(t *testing.T) {
	store := openStore(t)
	client := newBrowser(store)
	guest := client.session(t)
	oldCookie := client.cookie
	requireStatus(t, client.request(t, "PATCH", "/profile", map[string]string{"display_name": "认真使用者"}), 200)
	requireStatus(t, client.request(t, "PUT", "/preferences", map[string]any{"tags": []string{"链上身份"}}), 200)
	for _, kind := range []string{"bookmark", "helpful"} {
		requireStatus(t, client.request(t, "PUT", "/posts/reference-ens/"+kind, map[string]bool{"active": true}), 200)
	}
	collection := client.createCollection(t, "我的身份工具")
	account := newSigner(t)
	user := client.login(t, account)
	if user.DisplayName != "认真使用者" || len(user.Preferences) != 1 || user.Preferences[0] != "链上身份" {
		t.Fatalf("游客资料未迁移: %+v", user)
	}
	requireStatus(t, client.request(t, "GET", "/collections/"+collection.ID, nil), 200)
	post := client.getPost(t, "reference-ens")
	if !post.Bookmarked || !post.Helpful || post.HelpfulCount != 1 {
		t.Errorf("迁移后互动状态或计数不正确: %+v", post)
	}
	if remaining := store.count(t, "SELECT count(*) FROM actions WHERE user_id=?", guest.ID); remaining != 0 {
		t.Errorf("迁移后保留了 %d 条旧游客互动", remaining)
	}
	stale := *client
	stale.cookie = oldCookie
	requireStatus(t, stale.request(t, "GET", "/collections/"+collection.ID, nil), 404)
	store.restart(t)
	if restored := client.session(t); restored.ID != user.ID || restored.Wallet == nil {
		t.Fatal("钱包身份未在重启后保留")
	}
	// 已有钱包再次接收游客内容时，相同互动仍只能保留一条。
	anotherGuest := newBrowser(store)
	requireStatus(t, anotherGuest.request(t, "PUT", "/posts/reference-ens/helpful", map[string]bool{"active": true}), 200)
	if joined := anotherGuest.login(t, account); joined.ID != user.ID {
		t.Fatal("同一钱包创建了多个账户")
	}
	if count := anotherGuest.getPost(t, "reference-ens").HelpfulCount; count != 1 {
		t.Fatalf("重复归并造成互动累计: %d", count)
	}
}

// TestWalletSwitchDoesNotTransferPreviousAccountCollections 验证钱包切换不会转移已有账户的私有精选。
func TestWalletSwitchDoesNotTransferPreviousAccountCollections(t *testing.T) {
	// 先建钱包私有页再切换身份，切回原钱包后才能重新读取。
	store := openStore(t)
	client := newBrowser(store)
	first := newSigner(t)
	firstUser := client.login(t, first)
	collection := client.createCollection(t, "账户一的精选")
	secondUser := client.login(t, newSigner(t))
	if firstUser.ID == secondUser.ID {
		t.Fatal("切换钱包未更换身份")
	}
	requireStatus(t, client.request(t, "GET", "/collections/"+collection.ID, nil), 404)
	client.login(t, first)
	requireStatus(t, client.request(t, "GET", "/collections/"+collection.ID, nil), 200)
}
