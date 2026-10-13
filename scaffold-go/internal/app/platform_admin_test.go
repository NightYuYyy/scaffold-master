package app

// 用户中心平台：管理接口 /api/platform 集成测试（契约见 docs/platform-api.md）。

import (
	"net/http"
	"strings"
	"testing"
)

// adminCreateApp 以后台账号创建应用并登记清理（含该应用下的全部数据）。
func adminCreateApp(t testing.TB, token string, extra map[string]any) (int64, string) {
	t.Helper()
	code := "t_" + uniqueSuffix()
	body := map[string]any{"code": code, "name": "测试应用" + code, "remark": "集成测试"}
	for k, v := range extra {
		body[k] = v
	}
	r := post(t, "/api/platform/apps", token, body)
	id := num(okData(t, r)["id"])
	t.Cleanup(func() {
		for _, table := range []string{"app_counter_log", "app_counter", "app_user_identity", "app_auth_log", "app_user", "app_field", "app_api_key", "sys_user_app", "app_info"} {
			col := "app_id"
			if table == "app_info" {
				col = "id"
			}
			exec(t, "DELETE FROM "+table+" WHERE "+col+" = ?", id)
		}
	})
	return id, code
}

func TestPlatformAdminAppCrudAndKeys(t *testing.T) {
	token := loginAsAdmin(t)
	appID, code := adminCreateApp(t, token, map[string]any{"tokenExpiresSeconds": 3600, "allowRegister": 0})
	id := itoa(appID)

	if r := post(t, "/api/platform/apps", token, map[string]any{"code": code, "name": "dup"}); r.Status != 400 || r.Message() != "应用编码已存在" {
		t.Errorf("duplicate code: %d %s", r.Status, r.Raw)
	}
	if r := post(t, "/api/platform/apps", token, map[string]any{"code": "Bad-Code", "name": "x"}); r.Status != 400 {
		t.Errorf("bad code: %d %s", r.Status, r.Raw)
	}

	detail := okData(t, get(t, "/api/platform/apps/"+id, token))
	if detail["code"] != code || num(detail["tokenExpiresSeconds"]) != 3600 || num(detail["allowRegister"]) != 0 || num(detail["userCount"]) != 0 || num(detail["keyCount"]) != 0 {
		t.Errorf("detail = %v", detail)
	}
	if missing := hasKeys(detail, "id", "code", "name", "status", "allowRegister", "captchaEnabled", "tokenExpiresSeconds", "remark", "userCount", "keyCount", "createTime"); len(missing) > 0 {
		t.Errorf("detail lacks %v", missing)
	}

	okData(t, put(t, "/api/platform/apps/"+id, token, map[string]any{"name": "改名", "captchaEnabled": 0}))
	okData(t, put(t, "/api/platform/apps/"+id+"/status", token, map[string]any{"status": 0}))
	rows := okRows(t, get(t, "/api/platform/apps?keyword="+code+"&status=0", token))
	if len(rows) != 1 || rows[0]["name"] != "改名" || num(rows[0]["captchaEnabled"]) != 0 {
		t.Errorf("list = %v", rows)
	}
	options := okList(t, get(t, "/api/platform/apps/options", token))
	if !containsString(pluck(options, "code"), code) {
		t.Errorf("options lack %s", code)
	}

	// 密钥：新增返回明文一次；列表脱敏
	key := okData(t, post(t, "/api/platform/apps/"+id+"/keys", token, map[string]any{"name": "生产"}))
	secret, _ := key["apiSecret"].(string)
	keys := okList(t, get(t, "/api/platform/apps/"+id+"/keys", token))
	if len(keys) != 1 || keys[0]["apiKey"] != key["apiKey"] || keys[0]["name"] != "生产" {
		t.Fatalf("keys = %v", keys)
	}
	if keys[0]["secretMasked"] != secret[:7]+"****"+secret[44:] || keys[0]["apiSecret"] != nil || keys[0]["api_secret"] != nil {
		t.Errorf("masked = %v", keys[0])
	}
	if r := post(t, "/api/platform/apps/"+id+"/keys", token, map[string]any{"name": ""}); r.Status != 400 {
		t.Errorf("blank key name: %d %s", r.Status, r.Raw)
	}

	// 有用户时不能删除
	user := okData(t, post(t, "/api/platform/users", token, map[string]any{"appId": appID, "username": "au_" + uniqueSuffix()}))
	if r := del(t, "/api/platform/apps/"+id, token); r.Status != 400 || r.Message() != "应用下仍有用户，不能删除" {
		t.Errorf("delete with users: %d %s", r.Status, r.Raw)
	}
	okData(t, del(t, "/api/platform/users/"+itoa(num(user["id"])), token))
	okData(t, del(t, "/api/platform/apps/"+id+"/keys/"+itoa(num(key["id"])), token))
	if r := del(t, "/api/platform/apps/"+id+"/keys/"+itoa(num(key["id"])), token); r.Status != 400 || r.Message() != "密钥不存在" {
		t.Errorf("delete missing key: %d %s", r.Status, r.Raw)
	}
	okData(t, del(t, "/api/platform/apps/"+id, token))
	if r := get(t, "/api/platform/apps/"+id, token); r.Status != 400 || r.Message() != "应用不存在" {
		t.Errorf("deleted app detail: %d %s", r.Status, r.Raw)
	}
	if n := queryInt64(t, "SELECT deleted FROM app_info WHERE id = ?", appID); n != 1 {
		t.Errorf("deleted flag = %d", n)
	}

	export := get(t, "/api/platform/apps/export?keyword="+code, token)
	if export.Status != 200 || export.Header.Get("Content-Type") != "text/csv;charset=UTF-8" || !strings.HasPrefix(export.Raw, "编码,名称,状态,用户数,密钥数,创建时间\n") {
		t.Errorf("export: %d %q %s", export.Status, export.Header.Get("Content-Type"), export.Raw)
	}
	if n := queryInt64(t, "SELECT COUNT(*) FROM sys_oper_log WHERE module = '应用管理' AND action = '新增' AND request_params LIKE ?", "%"+code+"%"); n == 0 {
		t.Errorf("oper log missing for app create")
	}
}

func TestPlatformAdminKeyDisableRevokesUserTokens(t *testing.T) {
	token := loginAsAdmin(t)
	appID, _ := adminCreateApp(t, token, map[string]any{"captchaEnabled": 0})
	key := okData(t, post(t, "/api/platform/apps/"+itoa(appID)+"/keys", token, map[string]any{"name": "k"}))
	apiKey, _ := key["apiKey"].(string)
	username := "ku_" + uniqueSuffix()

	reg := okData(t, pcall(t, http.MethodPost, "/open/v1/auth/register", keyHdr(apiKey), map[string]any{"username": username, "password": "secret1"}))
	userToken, _ := reg["token"].(string)
	okData(t, pcall(t, http.MethodGet, "/open/v1/me", bearerHdr(userToken), nil))

	okData(t, put(t, "/api/platform/apps/"+itoa(appID)+"/keys/"+itoa(num(key["id"]))+"/status", token, map[string]any{"status": 0}))
	if r := pcall(t, http.MethodGet, "/open/v1/me", bearerHdr(userToken), nil); r.Status != 401 || r.Message() != "未认证或登录已过期" {
		t.Errorf("token after key disable: %d %s", r.Status, r.Raw)
	}
	if r := pcall(t, http.MethodGet, "/open/v1/fields", keyHdr(apiKey), nil); r.Status != 401 || r.Message() != "应用或密钥无效" {
		t.Errorf("disabled key: %d %s", r.Status, r.Raw)
	}
	if n := queryInt64(t, "SELECT COUNT(*) FROM app_api_key WHERE id = ? AND last_used_time IS NOT NULL", num(key["id"])); n != 1 {
		t.Errorf("last_used_time not touched")
	}
	if r := put(t, "/api/platform/apps/"+itoa(appID)+"/keys/999999999/status", token, map[string]any{"status": 1}); r.Status != 400 || r.Message() != "密钥不存在" {
		t.Errorf("missing key status: %d %s", r.Status, r.Raw)
	}
}

func TestPlatformAdminFieldCrudValidation(t *testing.T) {
	token := loginAsAdmin(t)
	appID, _ := adminCreateApp(t, token, nil)
	fieldBody := func(extra map[string]any) map[string]any {
		body := map[string]any{"appId": appID, "key": "f1", "label": "字段一", "type": "string"}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}
	if r := post(t, "/api/platform/fields", token, fieldBody(map[string]any{"type": "text"})); r.Status != 400 || r.Message() != "字段类型不正确" {
		t.Errorf("bad type: %d %s", r.Status, r.Raw)
	}
	if r := post(t, "/api/platform/fields", token, fieldBody(map[string]any{"type": "enum"})); r.Status != 400 {
		t.Errorf("enum without options: %d %s", r.Status, r.Raw)
	}
	if r := post(t, "/api/platform/fields", token, fieldBody(map[string]any{"type": "enum", "options": []any{}})); r.Status != 400 {
		t.Errorf("enum empty options: %d %s", r.Status, r.Raw)
	}
	if r := post(t, "/api/platform/fields", token, fieldBody(map[string]any{"key": "1bad"})); r.Status != 400 {
		t.Errorf("bad key: %d %s", r.Status, r.Raw)
	}
	if r := post(t, "/api/platform/fields", token, fieldBody(map[string]any{"type": "number", "defaultValue": "abc"})); r.Status != 400 {
		t.Errorf("bad default: %d %s", r.Status, r.Raw)
	}

	str := okData(t, post(t, "/api/platform/fields", token, fieldBody(map[string]any{"required": 1, "userEditable": 1, "sort": 2})))
	if r := post(t, "/api/platform/fields", token, fieldBody(nil)); r.Status != 400 || r.Message() != "字段键已存在" {
		t.Errorf("duplicate key: %d %s", r.Status, r.Raw)
	}
	okData(t, post(t, "/api/platform/fields", token, fieldBody(map[string]any{"key": "lvl", "type": "enum", "options": []string{"a", "b"}, "defaultValue": "a", "sort": 1})))
	counter := okData(t, post(t, "/api/platform/fields", token, fieldBody(map[string]any{"key": "points", "type": "counter", "userEditable": 1, "options": map[string]any{"allowNegative": true, "max": 1000}, "sort": 3})))
	okData(t, post(t, "/api/platform/fields", token, fieldBody(map[string]any{"key": "vip", "type": "bool", "defaultValue": true, "status": 0, "sort": 4})))

	fields := okList(t, get(t, "/api/platform/fields?appId="+itoa(appID), token))
	if len(fields) != 4 || fields[0]["key"] != "lvl" || fields[1]["key"] != "f1" || fields[2]["key"] != "points" || fields[3]["key"] != "vip" {
		t.Fatalf("fields = %v", fields)
	}
	if num(fields[2]["userEditable"]) != 0 || sub(fields[2], "options")["allowNegative"] != true || fnum(sub(fields[2], "options")["max"]) != 1000 {
		t.Errorf("counter field = %v", fields[2])
	}
	if num(fields[3]["status"]) != 0 || fields[3]["defaultValue"] != "true" {
		t.Errorf("bool field = %v", fields[3])
	}
	if missing := hasKeys(fields[0], "id", "appId", "key", "label", "type", "required", "userEditable", "defaultValue", "options", "sort", "status", "remark", "createTime"); len(missing) > 0 {
		t.Errorf("field lacks %v", missing)
	}

	// 停用字段不出现在开放接口；必填字段注册时必须提供
	apiKey, _ := okData(t, post(t, "/api/platform/apps/"+itoa(appID)+"/keys", token, map[string]any{"name": "k"}))["apiKey"].(string)
	open := okList(t, pcall(t, http.MethodGet, "/open/v1/fields", keyHdr(apiKey), nil))
	if len(open) != 3 {
		t.Errorf("open fields = %v", open)
	}
	if r := pcall(t, http.MethodPost, "/open/v1/users", secretHdr(apiKey, ""), map[string]any{"username": "x_" + uniqueSuffix()}); r.Status != 401 {
		t.Errorf("empty secret: %d", r.Status)
	}

	okData(t, put(t, "/api/platform/fields/"+itoa(num(str["id"])), token, map[string]any{"label": "改名", "required": 0, "key": "ignored", "type": "number"}))
	okData(t, put(t, "/api/platform/fields/"+itoa(num(counter["id"])), token, map[string]any{"options": map[string]any{"allowNegative": false}}))
	fields = okList(t, get(t, "/api/platform/fields?appId="+itoa(appID), token))
	if fields[1]["label"] != "改名" || fields[1]["type"] != "string" || fields[1]["key"] != "f1" || num(fields[1]["required"]) != 0 {
		t.Errorf("updated field = %v", fields[1])
	}
	if o := sub(fields[2], "options"); o["allowNegative"] != false || o["max"] != nil {
		t.Errorf("updated counter options = %v", o)
	}
	okData(t, del(t, "/api/platform/fields/"+itoa(num(str["id"])), token))
	if r := del(t, "/api/platform/fields/"+itoa(num(str["id"])), token); r.Status != 400 || r.Message() != "字段不存在" {
		t.Errorf("delete missing field: %d %s", r.Status, r.Raw)
	}
	if n := queryInt64(t, "SELECT COUNT(*) FROM app_field WHERE app_id = ?", appID); n != 3 {
		t.Errorf("fields left = %d", n)
	}
}

func TestPlatformAdminUsersFilteredByAppAndCounterAdjust(t *testing.T) {
	token := loginAsAdmin(t)
	appID, _ := adminCreateApp(t, token, nil)
	username := "am_" + uniqueSuffix()
	cleanupAppUser(t, demoAppID, username)

	other := okData(t, post(t, "/api/platform/users", token, map[string]any{"appId": appID, "username": username, "nickname": "另一应用"}))
	demo := okData(t, post(t, "/api/platform/users", token, map[string]any{"appId": demoAppID, "username": username, "profile": map[string]any{"level": "gold"}}))
	if num(other["appId"]) != appID || num(demo["appId"]) != demoAppID || sub(demo, "profile")["level"] != "gold" || demo["registerIp"] != nil {
		t.Errorf("created: %v / %v", other, demo)
	}
	if r := post(t, "/api/platform/users", token, map[string]any{"appId": demoAppID, "username": username}); r.Status != 400 || r.Message() != "账号已存在" {
		t.Errorf("duplicate: %d %s", r.Status, r.Raw)
	}
	if r := post(t, "/api/platform/users", token, map[string]any{"username": username}); r.Status != 400 {
		t.Errorf("missing appId: %d %s", r.Status, r.Raw)
	}
	if n := queryString(t, "SELECT create_by FROM app_user WHERE id = ?", num(demo["id"])); n != "admin:admin" {
		t.Errorf("create_by = %q", n)
	}

	rows := okRows(t, get(t, "/api/platform/users?appId="+itoa(appID)+"&keyword="+username, token))
	if len(rows) != 1 || num(rows[0]["id"]) != num(other["id"]) {
		t.Errorf("app rows = %v", rows)
	}
	rows = okRows(t, get(t, "/api/platform/users?appId=1&keyword="+username, token))
	if len(rows) != 1 || num(rows[0]["id"]) != num(demo["id"]) {
		t.Errorf("demo rows = %v", rows)
	}
	if missing := hasKeys(rows[0], "id", "appId", "username", "nickname", "avatar", "status", "profile", "counters", "registerIp", "lastLoginTime", "createTime"); len(missing) > 0 {
		t.Errorf("row lacks %v", missing)
	}

	demoID := itoa(num(demo["id"]))
	okData(t, put(t, "/api/platform/users/"+demoID, token, map[string]any{"nickname": "后台改名", "profile": map[string]any{"inviteCode": "ADM"}}))
	detail := okData(t, get(t, "/api/platform/users/"+demoID, token))
	if detail["nickname"] != "后台改名" || sub(detail, "profile")["inviteCode"] != "ADM" || sub(detail, "profile")["level"] != "gold" {
		t.Errorf("detail = %v", detail)
	}

	// 账户调整：operator = admin:<账号>，refNo 自动生成
	adj := okData(t, post(t, "/api/platform/users/"+demoID+"/counters/coins/adjust", token, map[string]any{"amount": 50, "reason": "后台补发"}))
	if fnum(adj["balance"]) != 50 || adj["duplicated"] != false {
		t.Errorf("adjust = %v", adj)
	}
	okData(t, post(t, "/api/platform/users/"+demoID+"/counters/coins/adjust", token, map[string]any{"amount": -20.25, "reason": "后台扣减"}))
	logRow := queryMap(t, "SELECT operator, ref_no FROM app_counter_log WHERE id = ?", num(adj["logId"]))
	if logRow["operator"] != "admin:admin" || !strings.HasPrefix(logRow["ref_no"].(string), "admin-") {
		t.Errorf("log row = %v", logRow)
	}
	if r := post(t, "/api/platform/users/"+demoID+"/counters/nope/adjust", token, map[string]any{"amount": 1, "reason": "x"}); r.Status != 400 || r.Message() != "账户不存在" {
		t.Errorf("unknown counter: %d %s", r.Status, r.Raw)
	}
	logs := okRows(t, get(t, "/api/platform/counter-logs?appId=1&userId="+demoID, token))
	if len(logs) != 2 || fnum(logs[0]["amount"]) != -20.25 || fnum(logs[0]["balanceAfter"]) != 29.75 || logs[0]["username"] != username || logs[0]["operator"] != "admin:admin" {
		t.Errorf("counter logs = %v", logs)
	}
	logs = okRows(t, get(t, "/api/platform/counter-logs?appId=1&keyword="+username+"&fieldKey=coins", token))
	if len(logs) != 2 {
		t.Errorf("counter logs by keyword = %v", logs)
	}
	export := get(t, "/api/platform/counter-logs/export?appId=1&userId="+demoID, token)
	if export.Status != 200 || !strings.HasPrefix(export.Raw, "账号,字段,变动量,变动后余额,原因,幂等号,操作方,时间\n") || !strings.Contains(export.Raw, "后台补发") {
		t.Errorf("export: %s", export.Raw)
	}
	users := get(t, "/api/platform/users/export?appId=1&keyword="+username, token)
	if users.Status != 200 || !strings.Contains(users.Raw, "账号,昵称,状态,") || !strings.Contains(users.Raw, username+",后台改名,正常,") || !strings.Contains(users.Raw, "29.75") {
		t.Errorf("users export: %s", users.Raw)
	}

	// 状态 / 重置密码 / 删除均撤销 token
	userToken := openLogin(t, username, queryString(t, "SELECT config_value FROM sys_config WHERE config_key = 'app.user.initPassword'"))
	okData(t, put(t, "/api/platform/users/"+demoID+"/password", token, map[string]any{"password": "newPwd1"}))
	if r := pcall(t, http.MethodGet, "/open/v1/me", bearerHdr(userToken), nil); r.Status != 401 {
		t.Errorf("token after reset: %d", r.Status)
	}
	userToken = openLogin(t, username, "newPwd1")
	okData(t, put(t, "/api/platform/users/"+demoID+"/status", token, map[string]any{"status": 0}))
	if r := pcall(t, http.MethodGet, "/open/v1/me", bearerHdr(userToken), nil); r.Status != 401 || r.Message() != "未认证或登录已过期" {
		t.Errorf("token after disable: %d %s", r.Status, r.Raw)
	}
	okData(t, put(t, "/api/platform/users/"+demoID+"/status", token, map[string]any{"status": 1}))
	userToken = openLogin(t, username, "newPwd1")
	okData(t, del(t, "/api/platform/users/"+demoID, token))
	if r := pcall(t, http.MethodGet, "/open/v1/me", bearerHdr(userToken), nil); r.Status != 401 {
		t.Errorf("token after delete: %d", r.Status)
	}
	if r := get(t, "/api/platform/users/"+demoID, token); r.Status != 400 || r.Message() != "用户不存在" {
		t.Errorf("deleted detail: %d %s", r.Status, r.Raw)
	}
}

func TestPlatformAdminAppScopeForAppAdmin(t *testing.T) {
	admin := loginAsAdmin(t)
	otherID, otherCode := adminCreateApp(t, admin, nil)
	other := itoa(otherID)
	token := login(t, "appadmin", "admin123")

	apps := okRows(t, get(t, "/api/platform/apps?pageSize=100", token))
	if !containsString(pluck(apps, "code"), "demo") || containsString(pluck(apps, "code"), otherCode) {
		t.Errorf("appadmin apps = %v", pluck(apps, "code"))
	}
	options := okList(t, get(t, "/api/platform/apps/options", token))
	if containsString(pluck(options, "code"), otherCode) || !containsString(pluck(options, "code"), "demo") {
		t.Errorf("appadmin options = %v", pluck(options, "code"))
	}
	okData(t, get(t, "/api/platform/apps/1", token))
	okData(t, get(t, "/api/platform/users?appId=1", token))
	okData(t, get(t, "/api/platform/fields?appId=1", token))

	for _, path := range []string{
		"/api/platform/apps/" + other, "/api/platform/apps/" + other + "/keys",
		"/api/platform/users?appId=" + other, "/api/platform/fields?appId=" + other,
		"/api/platform/counter-logs?appId=" + other, "/api/platform/auth-logs?appId=" + other,
		"/api/platform/users/export?appId=" + other,
	} {
		if r := get(t, path, token); r.Status != http.StatusForbidden {
			t.Errorf("GET %s as appadmin: %d %s", path, r.Status, r.Raw)
		}
	}
	if r := post(t, "/api/platform/users", token, map[string]any{"appId": otherID, "username": "x_" + uniqueSuffix()}); r.Status != 403 {
		t.Errorf("create user in other app: %d %s", r.Status, r.Raw)
	}
	if r := post(t, "/api/platform/fields", token, map[string]any{"appId": otherID, "key": "k", "label": "l", "type": "string"}); r.Status != 403 {
		t.Errorf("create field in other app: %d %s", r.Status, r.Raw)
	}
	if r := put(t, "/api/platform/apps/"+other+"/status", token, map[string]any{"status": 0}); r.Status != 403 {
		t.Errorf("status other app: %d %s", r.Status, r.Raw)
	}
	if r := post(t, "/api/platform/apps", token, map[string]any{"code": "x_" + uniqueSuffix(), "name": "x"}); r.Status != 403 {
		t.Errorf("appadmin create app: %d %s", r.Status, r.Raw)
	}

	otherUser := okData(t, post(t, "/api/platform/users", admin, map[string]any{"appId": otherID, "username": "ou_" + uniqueSuffix()}))
	if r := get(t, "/api/platform/users/"+itoa(num(otherUser["id"])), token); r.Status != 403 {
		t.Errorf("other app user detail: %d %s", r.Status, r.Raw)
	}
	// 缺省 appId 时取可见应用的第一个：appadmin 只绑定了应用 1
	rows := okRows(t, get(t, "/api/platform/users", token))
	for _, row := range rows {
		if num(row["appId"]) != 1 {
			t.Errorf("default appId row = %v", row)
		}
	}
}

func TestPlatformAdminAuthLogsList(t *testing.T) {
	username := "al_" + uniqueSuffix()
	cleanupAppUser(t, demoAppID, username)
	okData(t, openRegisterResp(t, username, "secret1", nil))
	if r := openLoginResp(t, username, "wrong"); r.Status != 400 {
		t.Fatalf("wrong login: %d", r.Status)
	}
	openLogin(t, username, "secret1")

	token := loginAsAdmin(t)
	rows := okRows(t, get(t, "/api/platform/auth-logs?appId=1&keyword="+username, token))
	if len(rows) != 3 || rows[0]["event"] != "login" || rows[1]["event"] != "login_failed" || rows[2]["event"] != "register" {
		t.Fatalf("auth logs = %v", rows)
	}
	if num(rows[1]["status"]) != 0 || rows[1]["msg"] != "账号或密码错误" || rows[1]["ip"] == nil || rows[1]["eventTime"] == nil || num(rows[1]["userId"]) == 0 {
		t.Errorf("failed row = %v", rows[1])
	}
	if missing := hasKeys(rows[0], "id", "appId", "userId", "username", "event", "status", "ip", "msg", "eventTime"); len(missing) > 0 {
		t.Errorf("row lacks %v", missing)
	}
	failed := okRows(t, get(t, "/api/platform/auth-logs?appId=1&keyword="+username+"&status=0", token))
	if len(failed) != 1 {
		t.Errorf("failed rows = %v", failed)
	}
	byEvent := okRows(t, get(t, "/api/platform/auth-logs?appId=1&keyword="+username+"&event=register", token))
	if len(byEvent) != 1 {
		t.Errorf("register rows = %v", byEvent)
	}
	export := get(t, "/api/platform/auth-logs/export?appId=1&keyword="+username, token)
	if export.Status != 200 || !strings.HasPrefix(export.Raw, "账号,事件,结果,IP,说明,时间\n") || strings.Count(export.Raw, username) != 3 {
		t.Errorf("export: %s", export.Raw)
	}
}
