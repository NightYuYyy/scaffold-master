package app

// 对应 AuthApiTest。文件顶部的 okData/okList/okRows/asMaps/pluck 为全部移植测试共用的小断言辅助。

import (
	"net/http"
	"testing"
)

// okData 对应 Java dataOf(body)：断言 HTTP 200 且 code=200，返回 data 对象。
func okData(t testing.TB, r resp) map[string]any {
	t.Helper()
	if r.Status != 200 || r.Code() != 200 {
		t.Fatalf("expected 200/200, got status=%d body=%s", r.Status, r.Raw)
	}
	return r.Data()
}

// okList 对应 Java listDataOf(body)：data 为对象数组。
func okList(t testing.TB, r resp) []map[string]any {
	t.Helper()
	okData(t, r)
	return asMaps(r.DataList())
}

// okRows 对应 rowsOf(dataOf(body))。
func okRows(t testing.TB, r resp) []map[string]any {
	t.Helper()
	okData(t, r)
	return r.Rows()
}

// asMaps 把 []any 转为对象切片。
func asMaps(l []any) []map[string]any {
	out := make([]map[string]any, 0, len(l))
	for _, x := range l {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// pluck 对应 AssertJ extracting(row -> row.get(key))。
func pluck(rows []map[string]any, key string) []any {
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, r[key])
	}
	return out
}

// hasKeys 对应 containsKeys：键存在即可（值可为 null）。
func hasKeys(m map[string]any, keys ...string) []string {
	var missing []string
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			missing = append(missing, k)
		}
	}
	return missing
}

func TestLoginReturnsTokenAndCurrentUserInfoUsesBearerToken(t *testing.T) {
	token := loginAsAdmin(t)

	r := get(t, "/api/auth/me", token)
	if r.Status != 200 {
		t.Fatalf("status %d: %s", r.Status, r.Raw)
	}
	if r.Code() != 200 {
		t.Fatalf("code %d: %s", r.Code(), r.Raw)
	}
	data := okData(t, r)
	if data["username"] != "admin" {
		t.Errorf("username = %v", data["username"])
	}
	if data["nickname"] != "周明远" {
		t.Errorf("nickname = %v", data["nickname"])
	}
	if !containsString(list(data, "roles"), "super_admin") {
		t.Errorf("roles %v lacks super_admin", data["roles"])
	}
	if !containsString(list(data, "permissions"), "system:user:list") {
		t.Errorf("permissions %v lacks system:user:list", data["permissions"])
	}
}

func TestRoutesReturnsAuthorizedMenuTree(t *testing.T) {
	token := loginAsAdmin(t)

	r := get(t, "/api/auth/routes", token)
	if r.Status != 200 {
		t.Fatalf("status %d: %s", r.Status, r.Raw)
	}
	paths := pluck(asMaps(list(okData(t, r), "routes")), "path")
	for _, p := range []string{"/dashboard", "/system"} {
		if !containsString(paths, p) {
			t.Errorf("routes paths %v lack %s", paths, p)
		}
	}
}

func TestProtectedEndpointWithoutTokenReturnsUnifiedUnauthorizedResponse(t *testing.T) {
	r := get(t, "/api/auth/me", "")

	if r.Status != http.StatusUnauthorized {
		t.Fatalf("status %d: %s", r.Status, r.Raw)
	}
	if r.Code() != 401 {
		t.Errorf("code = %d", r.Code())
	}
	if r.Message() != "未认证或登录已过期" {
		t.Errorf("message = %q", r.Message())
	}
	if r.Body["traceId"] == nil {
		t.Errorf("traceId missing: %s", r.Raw)
	}
}

func TestLogoutRevokesToken(t *testing.T) {
	token := loginAsAdmin(t)

	logout := post(t, "/api/auth/logout", token, nil)
	if logout.Status != 200 {
		t.Fatalf("logout status %d: %s", logout.Status, logout.Raw)
	}
	if logout.Code() != 200 {
		t.Errorf("logout code = %d", logout.Code())
	}

	me := get(t, "/api/auth/me", token)
	if me.Status != http.StatusUnauthorized {
		t.Fatalf("me status %d: %s", me.Status, me.Raw)
	}
	if me.Code() != 401 {
		t.Errorf("me code = %d", me.Code())
	}
}

func TestRevokedTokenCannotReadRoutes(t *testing.T) {
	token := loginAsAdmin(t)

	post(t, "/api/auth/logout", token, nil)

	r := get(t, "/api/auth/routes", token)
	if r.Status != http.StatusUnauthorized {
		t.Fatalf("status %d: %s", r.Status, r.Raw)
	}
	if r.Message() != "未认证或登录已过期" {
		t.Errorf("message = %q", r.Message())
	}
}

func TestLoginWithWrongPasswordReturnsBusinessError(t *testing.T) {
	r := loginResponse(t, "admin", "bad-password")

	if r.Status != http.StatusBadRequest {
		t.Fatalf("status %d: %s", r.Status, r.Raw)
	}
	if r.Code() != 400 {
		t.Errorf("code = %d", r.Code())
	}
	if r.Message() != "账号或密码错误" {
		t.Errorf("message = %q", r.Message())
	}
}

func TestAuthenticatedUserCanChangeOwnPassword(t *testing.T) {
	username := "pwd_" + uniqueSuffix()
	exec(t, `
                        INSERT INTO sys_user (username, password, nickname, status, deleted, create_by, update_by)
                        VALUES (?, ?, ?, 1, 0, 'it', 'it')
                        `,
		username,
		hash("oldPwd123"),
		"改密测试用户",
	)
	t.Cleanup(func() { exec(t, "DELETE FROM sys_user WHERE username = ?", username) })

	token := login(t, username, "oldPwd123")

	r := put(t, "/api/auth/password", token, map[string]any{"oldPassword": "oldPwd123", "newPassword": "newPwd123"})
	if r.Status != 200 {
		t.Fatalf("status %d: %s", r.Status, r.Raw)
	}
	if r.Code() != 200 {
		t.Errorf("code = %d", r.Code())
	}

	oldPasswordLogin := loginResponse(t, username, "oldPwd123")
	if oldPasswordLogin.Status != http.StatusBadRequest {
		t.Errorf("old password login status %d: %s", oldPasswordLogin.Status, oldPasswordLogin.Raw)
	}

	if login(t, username, "newPwd123") == "" {
		t.Errorf("new password login returned blank token")
	}
}

func TestSuccessfulLoginRecordsLastLoginTime(t *testing.T) {
	username := "last_" + uniqueSuffix()
	exec(t, `
                        INSERT INTO sys_user (username, password, nickname, status, deleted, create_by, update_by)
                        VALUES (?, ?, ?, 1, 0, 'it', 'it')
                        `,
		username,
		hash("loginPwd123"),
		"登录时间测试用户",
	)
	t.Cleanup(func() {
		exec(t, "DELETE FROM sys_user WHERE username = ?", username)
		exec(t, "DELETE FROM sys_login_log WHERE username = ?", username)
	})

	before := queryInt64(t, "SELECT COUNT(*) FROM sys_user WHERE username = ? AND last_login_time IS NOT NULL", username)
	if before != 0 {
		t.Fatalf("before = %d, want 0", before)
	}
	if login(t, username, "loginPwd123") == "" {
		t.Fatalf("login returned blank token")
	}
	after := queryInt64(t, "SELECT COUNT(*) FROM sys_user WHERE username = ? AND last_login_time IS NOT NULL", username)
	if after != 1 {
		t.Errorf("after = %d, want 1", after)
	}
}
