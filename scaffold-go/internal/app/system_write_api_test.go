package app

// 对应 SystemWriteApiTest。

import (
	"net/http"
	"strings"
	"testing"
)

// writeCreateUser 对应 createUser(headers, suffix, username, nickname)：角色 4（审计专员），绑定示例应用 1。
func writeCreateUser(t testing.TB, token, username, nickname string) int64 {
	t.Helper()
	create := post(t, "/api/system/users", token, map[string]any{
		"username": username,
		"nickname": nickname,
		"password": "admin123",
		"phone":    "",
		"email":    username + "@example.com",
		"status":   1,
		"roleIds":  []int{4},
		"appIds":   []int{1},
	})
	if create.Status != 200 {
		t.Fatalf("create user status %d: %s", create.Status, create.Raw)
	}
	return num(okData(t, create)["id"])
}

// userAppCount 当前账号在 sys_user_app 中的绑定数。
func userAppCount(t testing.TB, userID int64) int64 {
	t.Helper()
	return queryInt64(t, "SELECT COUNT(*) FROM sys_user_app WHERE user_id = ?", userID)
}

func TestBackendPermissionAnnotationRejectsUserWithoutPermission(t *testing.T) {
	// 种子账号 appadmin 为应用管理员，没有 system:user:list
	token := login(t, "appadmin", "admin123")

	r := get(t, "/api/system/users", token)

	if r.Status != http.StatusForbidden {
		t.Fatalf("status %d: %s", r.Status, r.Raw)
	}
	if r.Code() != 403 {
		t.Errorf("code = %d", r.Code())
	}
	if r.Message() != "没有访问权限" {
		t.Errorf("message = %q", r.Message())
	}
}

func TestUserCrudPersistsAssignmentsAndSoftDeletesRecord(t *testing.T) {
	suffix := uniqueSuffix()
	token := loginAsAdmin(t)

	create := post(t, "/api/system/users", token, map[string]any{
		"username": "it_" + suffix,
		"nickname": "集成测试用户" + suffix,
		"password": "admin123",
		"phone":    "",
		"email":    "it_" + suffix + "@example.com",
		"status":   1,
		"roleIds":  []int{4},
		"appIds":   []int{1},
	})
	if create.Status != 200 {
		t.Fatalf("create status %d: %s", create.Status, create.Raw)
	}
	userID := num(okData(t, create)["id"])
	id := itoa(userID)
	if n := userAppCount(t, userID); n != 1 {
		t.Errorf("sys_user_app rows after create = %d, want 1", n)
	}

	rows := okRows(t, get(t, "/api/system/users?pageNum=1&pageSize=10&keyword="+suffix, token))
	if len(rows) == 0 {
		t.Fatalf("user list empty")
	}
	if ids := list(rows[0], "appIds"); len(ids) != 1 || num(ids[0]) != 1 {
		t.Errorf("appIds = %v, want [1]", rows[0]["appIds"])
	}
	if rows[0]["appNames"] != "示例应用" {
		t.Errorf("appNames = %v, want 示例应用", rows[0]["appNames"])
	}
	if roleName, _ := rows[0]["roleName"].(string); !strings.Contains(roleName, "审计专员") {
		t.Errorf("roleName = %v, want 审计专员", rows[0]["roleName"])
	}

	// 更新：清空应用绑定、换角色、停用
	update := put(t, "/api/system/users/"+id, token, map[string]any{
		"nickname": "集成测试用户已更新" + suffix, "status": 0, "roleIds": []int{3}, "appIds": []int{},
	})
	if update.Status != 200 {
		t.Fatalf("update status %d: %s", update.Status, update.Raw)
	}
	if n := userAppCount(t, userID); n != 0 {
		t.Errorf("sys_user_app rows after clearing = %d, want 0", n)
	}
	rows = okRows(t, get(t, "/api/system/users?pageNum=1&pageSize=10&keyword="+suffix, token))
	if len(rows) == 0 {
		t.Fatalf("user list empty after update")
	}
	if rows[0]["nickname"] != "集成测试用户已更新"+suffix {
		t.Errorf("nickname = %v", rows[0]["nickname"])
	}
	if !emptyList(rows[0]["appIds"]) || rows[0]["appNames"] != "" {
		t.Errorf("appIds/appNames after clearing = %v / %v, want [] / \"\"", rows[0]["appIds"], rows[0]["appNames"])
	}
	if num(rows[0]["status"]) != 0 {
		t.Errorf("status = %v, want 0", rows[0]["status"])
	}

	// 不传 appIds 时保持原绑定；再次绑定后删除账号要连带清掉 sys_user_app
	if r := put(t, "/api/system/users/"+id, token, map[string]any{"appIds": []int{1}}); r.Status != 200 {
		t.Fatalf("rebind status %d: %s", r.Status, r.Raw)
	}
	if r := put(t, "/api/system/users/"+id, token, map[string]any{"nickname": "再次更新" + suffix}); r.Status != 200 {
		t.Fatalf("update without appIds status %d: %s", r.Status, r.Raw)
	}
	if n := userAppCount(t, userID); n != 1 {
		t.Errorf("sys_user_app rows after rebind = %d, want 1", n)
	}

	if r := del(t, "/api/system/users/"+id, token); r.Status != 200 {
		t.Fatalf("delete status %d: %s", r.Status, r.Raw)
	}
	if n := userAppCount(t, userID); n != 0 {
		t.Errorf("sys_user_app rows after delete = %d, want 0", n)
	}
	if after := okRows(t, get(t, "/api/system/users?pageNum=1&pageSize=10&keyword="+suffix, token)); len(after) != 0 {
		t.Errorf("rows after delete = %v", after)
	}
}

func TestSoftDeletedUserDoesNotBlockRoleDeletion(t *testing.T) {
	suffix := uniqueSuffix()
	token := loginAsAdmin(t)

	role := post(t, "/api/system/roles", token, map[string]any{
		"name":    "软删用户角色" + suffix,
		"code":    "soft_user_role_" + suffix,
		"sort":    99,
		"status":  1,
		"menuIds": []int{201},
	})
	if role.Status != 200 {
		t.Fatalf("role create status %d: %s", role.Status, role.Raw)
	}
	roleID := num(okData(t, role)["id"])

	user := post(t, "/api/system/users", token, map[string]any{
		"username": "soft_role_" + suffix,
		"nickname": "软删角色用户" + suffix,
		"password": "admin123",
		"status":   1,
		"roleIds":  []int64{roleID},
		"appIds":   []int{1},
	})
	if user.Status != 200 {
		t.Fatalf("user create status %d: %s", user.Status, user.Raw)
	}
	userID := num(okData(t, user)["id"])

	if r := del(t, "/api/system/users/"+itoa(userID), token); r.Status != 200 {
		t.Fatalf("user delete status %d: %s", r.Status, r.Raw)
	}

	deleteRole := del(t, "/api/system/roles/"+itoa(roleID), token)
	if deleteRole.Status != 200 {
		t.Fatalf("role delete status %d: %s", deleteRole.Status, deleteRole.Raw)
	}
}

func TestUserStatusPasswordAndExportApisWork(t *testing.T) {
	suffix := uniqueSuffix()
	token := loginAsAdmin(t)
	username := "it_user_ext_" + suffix
	id := itoa(writeCreateUser(t, token, username, "用户增强测试"+suffix))
	t.Cleanup(func() { del(t, "/api/system/users/"+id, token) })

	disable := put(t, "/api/system/users/"+id+"/status", token, map[string]any{"status": 0})
	if disable.Status != 200 {
		t.Fatalf("disable status %d: %s", disable.Status, disable.Raw)
	}

	disabledLogin := loginResponse(t, username, "admin123")
	if disabledLogin.Status != http.StatusBadRequest {
		t.Errorf("disabled login status %d: %s", disabledLogin.Status, disabledLogin.Raw)
	}
	if disabledLogin.Message() != "账号已停用" {
		t.Errorf("disabled login message = %q", disabledLogin.Message())
	}

	enable := put(t, "/api/system/users/"+id+"/status", token, map[string]any{"status": 1})
	if enable.Status != 200 {
		t.Fatalf("enable status %d: %s", enable.Status, enable.Raw)
	}

	reset := put(t, "/api/system/users/"+id+"/password", token, map[string]any{"password": "Yy@123456"})
	if reset.Status != 200 {
		t.Fatalf("reset status %d: %s", reset.Status, reset.Raw)
	}
	if login(t, username, "Yy@123456") == "" {
		t.Errorf("login with new password returned blank token")
	}

	export := get(t, "/api/system/users/export?keyword="+suffix, token)
	if export.Status != 200 {
		t.Fatalf("export status %d: %s", export.Status, export.Raw)
	}
	if ct := export.Header.Get("Content-Type"); ct != "text/csv;charset=UTF-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := export.Header.Get("Content-Disposition"); !strings.Contains(cd, "users") {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if !strings.Contains(export.Raw, "登录账号,用户昵称,手机号,邮箱,可管理应用,角色,状态,创建时间,最近登录") {
		t.Errorf("export lacks header: %s", export.Raw)
	}
	if !strings.Contains(export.Raw, username+",用户增强测试"+suffix+",,"+username+"@example.com,示例应用,审计专员,正常,") {
		t.Errorf("export lacks row with bound app: %s", export.Raw)
	}
}
