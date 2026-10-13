package app

// 用户中心平台：开放接口 /open/v1 集成测试（契约见 docs/platform-api.md）。

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"scaffold/internal/framework"
)

const (
	demoAppID  = int64(1)
	demoKey    = "demo_pk_local"
	demoSecret = "demo-secret-change-me-0123456789abcdef0123456789"
)

// pcall 带自定义请求头的调用（开放接口用 X-App-Key / X-App-Secret / Bearer）。
func pcall(t testing.TB, method, path string, headers map[string]string, body any) resp {
	t.Helper()
	var rd io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
		rd = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, testServer.URL+path, rd)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := testServer.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{Status: res.StatusCode, Header: res.Header, Raw: string(raw)}
	if strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") {
		_ = json.Unmarshal(raw, &out.Body)
	}
	return out
}

func keyHdr(apiKey string) map[string]string { return map[string]string{"X-App-Key": apiKey} }
func secretHdr(apiKey, secret string) map[string]string {
	return map[string]string{"X-App-Key": apiKey, "X-App-Secret": secret}
}
func bearerHdr(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func demoKeyHdr() map[string]string    { return keyHdr(demoKey) }
func demoSecretHdr() map[string]string { return secretHdr(demoKey, demoSecret) }

// openCaptcha 取开放接口验证码。
func openCaptcha(t testing.TB, apiKey string) (uuid, code string) {
	t.Helper()
	r := pcall(t, http.MethodGet, "/open/v1/captcha", keyHdr(apiKey), nil)
	d := okData(t, r)
	return d["uuid"].(string), d["code"].(string)
}

func openRegisterResp(t testing.TB, username, password string, profile map[string]any) resp {
	t.Helper()
	uuid, code := openCaptcha(t, demoKey)
	body := map[string]any{"username": username, "password": password, "captcha": code, "captchaUuid": uuid}
	if profile != nil {
		body["profile"] = profile
	}
	return pcall(t, http.MethodPost, "/open/v1/auth/register", demoKeyHdr(), body)
}

func openLoginResp(t testing.TB, username, password string) resp {
	t.Helper()
	uuid, code := openCaptcha(t, demoKey)
	return pcall(t, http.MethodPost, "/open/v1/auth/login", demoKeyHdr(), map[string]any{
		"username": username, "password": password, "captcha": code, "captchaUuid": uuid,
	})
}

func openLogin(t testing.TB, username, password string) string {
	t.Helper()
	r := openLoginResp(t, username, password)
	token, _ := okData(t, r)["token"].(string)
	if token == "" {
		t.Fatalf("login %s: no token: %s", username, r.Raw)
	}
	return token
}

// cleanupAppUser 注册清理：按应用 + 账号硬删除用户及其流水 / 日志。
func cleanupAppUser(t testing.TB, appID int64, username string) {
	t.Cleanup(func() {
		exec(t, "DELETE FROM app_counter_log WHERE app_id = ? AND user_id IN (SELECT id FROM app_user WHERE app_id = ? AND username = ?)", appID, appID, username)
		exec(t, "DELETE FROM app_counter WHERE app_id = ? AND user_id IN (SELECT id FROM app_user WHERE app_id = ? AND username = ?)", appID, appID, username)
		exec(t, "DELETE FROM app_user_identity WHERE app_id = ? AND user_id IN (SELECT id FROM app_user WHERE app_id = ? AND username = ?)", appID, appID, username)
		exec(t, "DELETE FROM app_auth_log WHERE app_id = ? AND username = ?", appID, username)
		exec(t, "DELETE FROM app_user WHERE app_id = ? AND username = ?", appID, username)
	})
}

func sub(m map[string]any, key string) map[string]any {
	s, _ := m[key].(map[string]any)
	return s
}

func fnum(v any) float64 { return framework.AsFloat64(v) }

func TestPlatformOpenRegisterLoginMeAndPasswordFlow(t *testing.T) {
	username := "pu_" + uniqueSuffix()
	cleanupAppUser(t, demoAppID, username)

	reg := openRegisterResp(t, username, "secret1", map[string]any{"inviteCode": "WELCOME"})
	data := okData(t, reg)
	if data["token"] == nil || num(data["expiresIn"]) != 604800 {
		t.Fatalf("register response: %s", reg.Raw)
	}
	user := sub(data, "user")
	if user["username"] != username || user["nickname"] != username {
		t.Errorf("user = %v", user)
	}
	if p := sub(user, "profile"); p["inviteCode"] != "WELCOME" || p["level"] != "bronze" || p["birthday"] != nil {
		t.Errorf("profile = %v", p)
	}
	if c := sub(user, "counters"); fnum(c["coins"]) != 0 {
		t.Errorf("counters = %v", c)
	}
	if user["createTime"] == nil || user["lastLoginTime"] == nil {
		t.Errorf("times missing: %v", user)
	}

	token := openLogin(t, username, "secret1")
	me := okData(t, pcall(t, http.MethodGet, "/open/v1/me", bearerHdr(token), nil))
	if me["username"] != username || num(me["status"]) != 1 {
		t.Errorf("me = %v", me)
	}

	upd := pcall(t, http.MethodPut, "/open/v1/me", bearerHdr(token), map[string]any{"nickname": "小明", "profile": map[string]any{"birthday": "2000-01-02"}})
	okData(t, upd)
	me = okData(t, pcall(t, http.MethodGet, "/open/v1/me", bearerHdr(token), nil))
	if me["nickname"] != "小明" || sub(me, "profile")["birthday"] != "2000-01-02" || sub(me, "profile")["inviteCode"] != "WELCOME" {
		t.Errorf("me after update = %v", me)
	}

	forbidden := pcall(t, http.MethodPut, "/open/v1/me", bearerHdr(token), map[string]any{"profile": map[string]any{"level": "gold"}})
	if forbidden.Status != 403 || forbidden.Message() != "字段 level 不允许用户修改" {
		t.Errorf("non-editable field: %d %s", forbidden.Status, forbidden.Raw)
	}
	badDate := pcall(t, http.MethodPut, "/open/v1/me", bearerHdr(token), map[string]any{"profile": map[string]any{"birthday": "2000/01/02"}})
	if badDate.Status != 400 || badDate.Message() != "字段 birthday 格式不正确" {
		t.Errorf("bad date: %d %s", badDate.Status, badDate.Raw)
	}
	unknown := pcall(t, http.MethodPut, "/open/v1/me", bearerHdr(token), map[string]any{"profile": map[string]any{"nope": 1}})
	if unknown.Status != 400 || unknown.Message() != "未定义的字段: nope" {
		t.Errorf("unknown field: %d %s", unknown.Status, unknown.Raw)
	}

	wrongOld := pcall(t, http.MethodPut, "/open/v1/me/password", bearerHdr(token), map[string]any{"oldPassword": "bad", "newPassword": "secret2"})
	if wrongOld.Status != 400 || wrongOld.Message() != "旧密码不正确" {
		t.Errorf("wrong old password: %d %s", wrongOld.Status, wrongOld.Raw)
	}
	same := pcall(t, http.MethodPut, "/open/v1/me/password", bearerHdr(token), map[string]any{"oldPassword": "secret1", "newPassword": "secret1"})
	if same.Status != 400 || same.Message() != "新密码不能与旧密码一致" {
		t.Errorf("same password: %d %s", same.Status, same.Raw)
	}
	okData(t, pcall(t, http.MethodPut, "/open/v1/me/password", bearerHdr(token), map[string]any{"oldPassword": "secret1", "newPassword": "secret2"}))
	if r := pcall(t, http.MethodGet, "/open/v1/me", bearerHdr(token), nil); r.Status != 401 || r.Message() != "未认证或登录已过期" {
		t.Errorf("old token after password change: %d %s", r.Status, r.Raw)
	}
	if r := openLoginResp(t, username, "secret1"); r.Status != 400 || r.Message() != "账号或密码错误" {
		t.Errorf("old password login: %d %s", r.Status, r.Raw)
	}
	token = openLogin(t, username, "secret2")

	// 刷新：旧 token 失效，新 token 可用；登出后新 token 也失效
	refresh := okData(t, pcall(t, http.MethodPost, "/open/v1/auth/refresh", bearerHdr(token), nil))
	newToken, _ := refresh["token"].(string)
	if newToken == "" || newToken == token {
		t.Fatalf("refresh: %v", refresh)
	}
	if r := pcall(t, http.MethodGet, "/open/v1/me", bearerHdr(token), nil); r.Status != 401 {
		t.Errorf("old token after refresh: %d", r.Status)
	}
	okData(t, pcall(t, http.MethodGet, "/open/v1/me", bearerHdr(newToken), nil))
	okData(t, pcall(t, http.MethodPost, "/open/v1/auth/logout", bearerHdr(newToken), nil))
	if r := pcall(t, http.MethodGet, "/open/v1/me", bearerHdr(newToken), nil); r.Status != 401 {
		t.Errorf("token after logout: %d", r.Status)
	}

	events := map[string]int64{}
	for _, e := range []string{"register", "login", "login_failed", "refresh", "logout"} {
		events[e] = queryInt64(t, "SELECT COUNT(*) FROM app_auth_log WHERE app_id = ? AND username = ? AND event = ?", demoAppID, username, e)
	}
	if events["register"] != 1 || events["login"] < 2 || events["login_failed"] != 1 || events["refresh"] != 1 || events["logout"] != 1 {
		t.Errorf("auth log events = %v", events)
	}
	if ip := queryString(t, "SELECT last_login_ip FROM app_user WHERE app_id = ? AND username = ?", demoAppID, username); ip == "" {
		t.Errorf("last_login_ip not set")
	}
}

func TestPlatformOpenRegisterValidation(t *testing.T) {
	username := "pv_" + uniqueSuffix()
	cleanupAppUser(t, demoAppID, username)

	bad := pcall(t, http.MethodPost, "/open/v1/auth/register", demoKeyHdr(), map[string]any{"username": username, "password": "secret1", "captcha": "XXXX", "captchaUuid": "nope"})
	if bad.Status != 400 || bad.Message() != "验证码错误或已过期" {
		t.Errorf("captcha failure: %d %s", bad.Status, bad.Raw)
	}
	if n := queryInt64(t, "SELECT COUNT(*) FROM app_auth_log WHERE app_id = ? AND username = ? AND event = 'register_failed'", demoAppID, username); n != 1 {
		t.Errorf("register_failed logs = %d", n)
	}

	if r := openRegisterResp(t, username, "12345", nil); r.Status != 400 {
		t.Errorf("short password: %d %s", r.Status, r.Raw)
	}
	if r := openRegisterResp(t, "a b", "secret1", nil); r.Status != 400 {
		t.Errorf("bad username: %d %s", r.Status, r.Raw)
	}
	if r := pcall(t, http.MethodPost, "/open/v1/users", demoSecretHdr(), map[string]any{"username": username, "password": "secret1", "profile": map[string]any{"level": 5}}); r.Status != 400 || r.Message() != "字段 level 格式不正确" {
		t.Errorf("bad enum: %d %s", r.Status, r.Raw)
	}
	okData(t, openRegisterResp(t, username, "secret1", nil))
	if r := openRegisterResp(t, username, "secret1", nil); r.Status != 400 || r.Message() != "账号已存在" {
		t.Errorf("duplicate: %d %s", r.Status, r.Raw)
	}
}

func TestPlatformOpenAuthRejections(t *testing.T) {
	if r := pcall(t, http.MethodGet, "/open/v1/captcha", nil, nil); r.Status != 401 || r.Message() != "应用或密钥无效" {
		t.Errorf("missing key: %d %s", r.Status, r.Raw)
	}
	if r := pcall(t, http.MethodGet, "/open/v1/captcha", keyHdr("nope_pk"), nil); r.Status != 401 || r.Message() != "应用或密钥无效" {
		t.Errorf("unknown key: %d %s", r.Status, r.Raw)
	}
	if r := pcall(t, http.MethodGet, "/open/v1/users", secretHdr(demoKey, "wrong"), nil); r.Status != 401 || r.Message() != "应用密钥错误" {
		t.Errorf("wrong secret: %d %s", r.Status, r.Raw)
	}
	if r := pcall(t, http.MethodGet, "/open/v1/users", demoKeyHdr(), nil); r.Status != 401 || r.Message() != "应用密钥错误" {
		t.Errorf("missing secret: %d %s", r.Status, r.Raw)
	}
	if r := pcall(t, http.MethodGet, "/open/v1/me", nil, nil); r.Status != 401 || r.Message() != "未认证或登录已过期" {
		t.Errorf("missing bearer: %d %s", r.Status, r.Raw)
	}
	if r := pcall(t, http.MethodGet, "/open/v1/me", bearerHdr("garbage"), nil); r.Status != 401 || r.Message() != "未认证或登录已过期" {
		t.Errorf("bad bearer: %d %s", r.Status, r.Raw)
	}
	if r := pcall(t, http.MethodGet, "/open/v1/nope", demoKeyHdr(), nil); r.Status != 404 {
		t.Errorf("unknown route: %d %s", r.Status, r.Raw)
	}

	// 停用的应用：任何密钥都被拒绝
	token := loginAsAdmin(t)
	appID, code := adminCreateApp(t, token, map[string]any{"captchaEnabled": 0})
	key := okData(t, post(t, "/api/platform/apps/"+itoa(appID)+"/keys", token, map[string]any{"name": "t"}))
	apiKey, _ := key["apiKey"].(string)
	if !strings.HasPrefix(apiKey, code+"_") || len(apiKey) != len(code)+17 {
		t.Errorf("apiKey = %q", apiKey)
	}
	if secret, _ := key["apiSecret"].(string); len(secret) != 48 {
		t.Errorf("apiSecret = %q", secret)
	}
	okData(t, pcall(t, http.MethodGet, "/open/v1/fields", keyHdr(apiKey), nil))
	okData(t, put(t, "/api/platform/apps/"+itoa(appID)+"/status", token, map[string]any{"status": 0}))
	if r := pcall(t, http.MethodGet, "/open/v1/fields", keyHdr(apiKey), nil); r.Status != 401 || r.Message() != "应用或密钥无效" {
		t.Errorf("disabled app: %d %s", r.Status, r.Raw)
	}
}

func TestPlatformOpenServerUserLifecycle(t *testing.T) {
	username := "ps_" + uniqueSuffix()
	cleanupAppUser(t, demoAppID, username)

	created := okData(t, pcall(t, http.MethodPost, "/open/v1/users", demoSecretHdr(), map[string]any{
		"username": username, "password": "secret1", "nickname": "服务端用户", "profile": map[string]any{"level": "silver"},
	}))
	id := itoa(num(created["id"]))
	if sub(created, "profile")["level"] != "silver" {
		t.Errorf("created = %v", created)
	}
	if n := queryString(t, "SELECT create_by FROM app_user WHERE id = ?", num(created["id"])); n != "api:"+demoKey {
		t.Errorf("create_by = %q", n)
	}

	okData(t, pcall(t, http.MethodPut, "/open/v1/users/"+id+"/profile", demoSecretHdr(), map[string]any{"profile": map[string]any{"level": "gold", "inviteCode": "ABC"}}))
	detail := okData(t, pcall(t, http.MethodGet, "/open/v1/users/"+id, demoSecretHdr(), nil))
	if p := sub(detail, "profile"); p["level"] != "gold" || p["inviteCode"] != "ABC" {
		t.Errorf("detail profile = %v", p)
	}
	if r := pcall(t, http.MethodPut, "/open/v1/users/"+id+"/profile", demoSecretHdr(), map[string]any{"profile": map[string]any{"coins": 1}}); r.Status != 400 || r.Message() != "未定义的字段: coins" {
		t.Errorf("counter in profile: %d %s", r.Status, r.Raw)
	}

	rows := okRows(t, pcall(t, http.MethodGet, "/open/v1/users?keyword="+username, demoSecretHdr(), nil))
	if len(rows) != 1 || rows[0]["username"] != username {
		t.Errorf("list = %v", rows)
	}

	token := openLogin(t, username, "secret1")
	intro := okData(t, pcall(t, http.MethodPost, "/open/v1/auth/introspect", demoSecretHdr(), map[string]any{"token": token}))
	if intro["active"] != true || intro["username"] != username || itoa(num(intro["userId"])) != id || intro["expiresAt"] == nil {
		t.Errorf("introspect = %v", intro)
	}

	okData(t, pcall(t, http.MethodPut, "/open/v1/users/"+id+"/status", demoSecretHdr(), map[string]any{"status": 0}))
	if r := pcall(t, http.MethodGet, "/open/v1/me", bearerHdr(token), nil); r.Status != 401 || r.Message() != "未认证或登录已过期" {
		t.Errorf("token after disable: %d %s", r.Status, r.Raw)
	}
	if r := openLoginResp(t, username, "secret1"); r.Status != 400 || r.Message() != "账号已停用" {
		t.Errorf("disabled login: %d %s", r.Status, r.Raw)
	}
	intro = okData(t, pcall(t, http.MethodPost, "/open/v1/auth/introspect", demoSecretHdr(), map[string]any{"token": token}))
	if intro["active"] != false || intro["userId"] != nil {
		t.Errorf("introspect inactive = %v", intro)
	}
	okData(t, pcall(t, http.MethodPut, "/open/v1/users/"+id+"/status", demoSecretHdr(), map[string]any{"status": 1}))
	okData(t, pcall(t, http.MethodGet, "/open/v1/me", bearerHdr(openLogin(t, username, "secret1")), nil))

	if r := pcall(t, http.MethodGet, "/open/v1/users/999999999", demoSecretHdr(), nil); r.Status != 400 || r.Message() != "用户不存在" {
		t.Errorf("missing user: %d %s", r.Status, r.Raw)
	}
}

func TestPlatformOpenCounterChangeAndLogs(t *testing.T) {
	username := "pc_" + uniqueSuffix()
	cleanupAppUser(t, demoAppID, username)
	created := okData(t, pcall(t, http.MethodPost, "/open/v1/users", demoSecretHdr(), map[string]any{"username": username}))
	id := itoa(num(created["id"]))
	base := "/open/v1/users/" + id + "/counters/coins/"
	refA, refB := "a-"+uniqueSuffix(), "b-"+uniqueSuffix()

	change := func(body map[string]any) resp {
		return pcall(t, http.MethodPost, base+"change", demoSecretHdr(), body)
	}
	first := okData(t, change(map[string]any{"amount": 100, "reason": "签到", "refNo": refA}))
	if fnum(first["balance"]) != 100 || first["duplicated"] != false || num(first["logId"]) == 0 {
		t.Errorf("first = %v", first)
	}
	second := okData(t, change(map[string]any{"amount": -30, "reason": "兑换", "refNo": refB}))
	if fnum(second["balance"]) != 70 {
		t.Errorf("second = %v", second)
	}
	if r := change(map[string]any{"amount": -100, "reason": "超扣", "refNo": "c-" + uniqueSuffix()}); r.Status != 400 || r.Message() != "余额不足" {
		t.Errorf("insufficient: %d %s", r.Status, r.Raw)
	}
	dup := okData(t, change(map[string]any{"amount": 100, "reason": "签到", "refNo": refA}))
	if fnum(dup["balance"]) != 100 || dup["duplicated"] != true || num(dup["logId"]) != num(first["logId"]) {
		t.Errorf("duplicate = %v", dup)
	}
	if r := change(map[string]any{"amount": 50, "reason": "签到", "refNo": refA}); r.Status != 400 || r.Message() != "幂等号已使用" {
		t.Errorf("ref conflict: %d %s", r.Status, r.Raw)
	}
	if r := pcall(t, http.MethodPost, "/open/v1/users/"+id+"/counters/nope/change", demoSecretHdr(), map[string]any{"amount": 1, "reason": "x", "refNo": "d-" + uniqueSuffix()}); r.Status != 400 || r.Message() != "账户不存在" {
		t.Errorf("unknown key: %d %s", r.Status, r.Raw)
	}
	for _, amount := range []any{0, "abc", 1.00001, nil} {
		if r := change(map[string]any{"amount": amount, "reason": "x", "refNo": "e-" + uniqueSuffix()}); r.Status != 400 {
			t.Errorf("amount %v: %d %s", amount, r.Status, r.Raw)
		}
	}
	if r := change(map[string]any{"amount": 1, "refNo": "f-" + uniqueSuffix()}); r.Status != 400 {
		t.Errorf("missing reason: %d %s", r.Status, r.Raw)
	}
	str := okData(t, change(map[string]any{"amount": "10.5", "reason": "字符串金额", "refNo": "g-" + uniqueSuffix()}))
	if fnum(str["balance"]) != 80.5 {
		t.Errorf("string amount = %v", str)
	}

	counters := okData(t, pcall(t, http.MethodGet, "/open/v1/users/"+id+"/counters", demoSecretHdr(), nil))
	if fnum(counters["coins"]) != 80.5 {
		t.Errorf("counters = %v", counters)
	}
	me := okData(t, pcall(t, http.MethodGet, "/open/v1/me", bearerHdr(openLogin(t, username, queryString(t, "SELECT config_value FROM sys_config WHERE config_key = 'app.user.initPassword'"))), nil))
	if fnum(sub(me, "counters")["coins"]) != 80.5 {
		t.Errorf("me counters = %v", me["counters"])
	}

	logs := okRows(t, pcall(t, http.MethodGet, base+"logs?pageSize=10", demoSecretHdr(), nil))
	if len(logs) != 3 || fnum(logs[0]["amount"]) != 10.5 || fnum(logs[2]["balanceAfter"]) != 100 || logs[2]["refNo"] != refA {
		t.Errorf("logs = %v", logs)
	}
	if logs[0]["operator"] != "api:"+demoKey || logs[0]["fieldKey"] != "coins" || logs[0]["createTime"] == nil {
		t.Errorf("log row = %v", logs[0])
	}
	if missing := hasKeys(logs[0], "id", "fieldKey", "amount", "balanceAfter", "reason", "refNo", "operator", "createTime"); len(missing) > 0 {
		t.Errorf("log row lacks %v", missing)
	}
	if b := queryString(t, "SELECT balance FROM app_counter WHERE app_id = ? AND user_id = ? AND field_key = 'coins'", demoAppID, num(created["id"])); b != "80.5000" {
		t.Errorf("stored balance = %q", b)
	}
}

func TestPlatformOpenFieldsList(t *testing.T) {
	fields := okList(t, pcall(t, http.MethodGet, "/open/v1/fields", demoKeyHdr(), nil))
	byKey := map[string]map[string]any{}
	for _, f := range fields {
		byKey[f["key"].(string)] = f
	}
	for _, k := range []string{"coins", "level", "inviteCode", "birthday"} {
		if byKey[k] == nil {
			t.Fatalf("fields lack %s: %v", k, fields)
		}
	}
	if missing := hasKeys(byKey["coins"], "key", "label", "type", "required", "userEditable", "defaultValue", "options", "sort"); len(missing) > 0 {
		t.Errorf("field lacks %v", missing)
	}
	if o := sub(byKey["coins"], "options"); o["allowNegative"] != false || o["max"] != nil {
		t.Errorf("coins options = %v", byKey["coins"]["options"])
	}
	if opts := list(byKey["level"], "options"); len(opts) != 3 || opts[0] != "bronze" {
		t.Errorf("level options = %v", byKey["level"]["options"])
	}
	if byKey["inviteCode"]["options"] != nil || num(byKey["inviteCode"]["userEditable"]) != 1 {
		t.Errorf("inviteCode = %v", byKey["inviteCode"])
	}
}
