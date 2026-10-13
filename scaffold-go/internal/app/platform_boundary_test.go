package app

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestPlatformRegistrationCannotSetReadOnlyFields(t *testing.T) {
	username := "readonly_" + uniqueSuffix()
	cleanupAppUser(t, demoAppID, username)
	r := openRegisterResp(t, username, "secret1", map[string]any{"level": "gold"})
	if r.Status != 403 || r.Message() != "字段 level 不允许用户修改" {
		t.Errorf("register read-only field: status=%d", r.Status)
	}
	if n := queryInt64(t, "SELECT COUNT(*) FROM app_user WHERE app_id = ? AND username = ?", demoAppID, username); n != 0 {
		t.Fatalf("rejected registration persisted %d users", n)
	}

	// 同一个字段仍可由可信服务端设置，公开注册省略时使用默认值。
	user := sub(okData(t, openRegisterResp(t, username, "secret1", nil)), "user")
	if sub(user, "profile")["level"] != "bronze" {
		t.Errorf("default level = %v", sub(user, "profile")["level"])
	}
	path := "/open/v1/users/" + itoa(num(user["id"])) + "/profile"
	okData(t, pcall(t, http.MethodPut, path, demoSecretHdr(), map[string]any{"profile": map[string]any{"level": "gold"}}))
	trusted := username + "_server"
	cleanupAppUser(t, demoAppID, trusted)
	created := okData(t, pcall(t, http.MethodPost, "/open/v1/users", demoSecretHdr(), map[string]any{
		"username": trusted, "profile": map[string]any{"level": "gold"},
	}))
	if sub(created, "profile")["level"] != "gold" {
		t.Errorf("server-created level = %v", sub(created, "profile")["level"])
	}
}

func TestPlatformShorterTokenLifetimeKeepsRevocationIndex(t *testing.T) {
	admin := loginAsAdmin(t)
	appID, _ := adminCreateApp(t, admin, map[string]any{"tokenExpiresSeconds": 3600, "captchaEnabled": 0})
	appPath := "/api/platform/apps/" + itoa(appID)
	key := okData(t, post(t, appPath+"/keys", admin, map[string]any{"name": "撤销回归"}))
	headers := keyHdr(key["apiKey"].(string))
	body := map[string]any{"username": "revocation_user", "password": "secret1"}
	first := okData(t, pcall(t, http.MethodPost, "/open/v1/auth/register", headers, body))
	uid := num(sub(first, "user")["id"])
	setKey := "open:user:" + itoa(appID) + ":" + itoa(uid)
	t.Cleanup(func() { _ = testApp.Redis.Del(context.Background(), setKey).Err() })
	okData(t, put(t, appPath, admin, map[string]any{"tokenExpiresSeconds": 60}))
	second := okData(t, pcall(t, http.MethodPost, "/open/v1/auth/login", headers, body))
	ttl, err := testApp.Redis.TTL(context.Background(), setKey).Result()
	if err != nil || ttl < 3500*time.Second {
		t.Errorf("revocation index was shortened: ttl=%v err=%v", ttl, err)
	}
	okData(t, put(t, "/api/platform/users/"+itoa(uid)+"/password", admin, map[string]any{"password": "secret2"}))
	for i, token := range []string{first["token"].(string), second["token"].(string)} {
		if r := pcall(t, http.MethodGet, "/open/v1/me", bearerHdr(token), nil); r.Status != 401 {
			t.Errorf("token %d survived reset: status=%d", i, r.Status)
		}
	}
}

func TestPlatformConcurrentFirstCounterChange(t *testing.T) {
	username := "concurrent_" + uniqueSuffix()
	cleanupAppUser(t, demoAppID, username)
	user := okData(t, pcall(t, http.MethodPost, "/open/v1/users", demoSecretHdr(), map[string]any{"username": username}))
	uid := num(user["id"])
	path := "/open/v1/users/" + itoa(uid) + "/counters/coins/change"
	ref := "concurrent_" + uniqueSuffix()
	const workers = 12
	start := make(chan struct{})
	results := make(chan resp, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- pcall(t, http.MethodPost, path, demoSecretHdr(), map[string]any{"amount": "0.1", "reason": "并发首次入账", "refNo": ref})
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	created, count := 0, 0
	for r := range results {
		count++
		if r.Status != 200 {
			t.Errorf("concurrent request: status=%d message=%s", r.Status, r.Message())
			continue
		}
		if r.Data()["duplicated"] == false {
			created++
		}
		if fnum(r.Data()["balance"]) != 0.1 {
			t.Errorf("balance = %v", r.Data()["balance"])
		}
	}
	if count != workers || created != 1 {
		t.Errorf("responses=%d first writes=%d", count, created)
	}
	if n := queryInt64(t, "SELECT COUNT(*) FROM app_counter_log WHERE app_id = ? AND ref_no = ?", demoAppID, ref); n != 1 {
		t.Errorf("ledger rows=%d", n)
	}
	if b := queryString(t, "SELECT balance FROM app_counter WHERE app_id = ? AND user_id = ? AND field_key = 'coins'", demoAppID, uid); b != "0.1000" {
		t.Errorf("persisted balance=%s", b)
	}
}
