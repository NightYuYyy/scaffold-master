package app

// 对应 DataScopeApiTest。

import (
	"fmt"
	"strings"
	"testing"
)

const dataScopeAdmin123Hash = "$2b$10$L7q28dpKkcfrrUzfvZebqe7pvxcKpaSgh7EUenf9Q5z2LiYKxDpIu"

// scopeInsertRole 对应 insertRole，并注册清理（sys_role_menu / sys_role_dept / sys_role）。
func scopeInsertRole(t testing.TB, code, dataScope string) int64 {
	t.Helper()
	exec(t, `
                INSERT INTO sys_role (role_name, role_code, data_scope, sort, status, builtin, remark, create_by, update_by)
                VALUES (?, ?, ?, 99, 1, 0, '数据权限集成测试', 'it', 'it')
                `, "数据权限测试角色", code, dataScope)
	roleID := queryInt64(t, "SELECT id FROM sys_role WHERE role_code = ?", code)
	t.Cleanup(func() {
		exec(t, "DELETE FROM sys_role_menu WHERE role_id = ?", roleID)
		exec(t, "DELETE FROM sys_role_dept WHERE role_id = ?", roleID)
		exec(t, "DELETE FROM sys_role WHERE id = ?", roleID)
	})
	exec(t, "INSERT INTO sys_role_menu (role_id, menu_id) VALUES (?, 101)", roleID)
	return roleID
}

// scopeInsertUser 对应 insertUser；roleID 为 0 表示不绑定角色。
func scopeInsertUser(t testing.TB, username, nickname string, deptID, roleID int64) int64 {
	t.Helper()
	exec(t, `
                INSERT INTO sys_user (username, password, nickname, dept_id, status, deleted, create_by, update_by)
                VALUES (?, ?, ?, ?, 1, 0, 'it', 'it')
                `, username, dataScopeAdmin123Hash, nickname, deptID)
	userID := queryInt64(t, "SELECT id FROM sys_user WHERE username = ?", username)
	t.Cleanup(func() {
		exec(t, "DELETE FROM sys_user_role WHERE user_id = ?", userID)
		exec(t, "DELETE FROM sys_user WHERE id = ?", userID)
	})
	if roleID != 0 {
		exec(t, "INSERT INTO sys_user_role (user_id, role_id) VALUES (?, ?)", userID, roleID)
	}
	return userID
}

// exactly 对应 containsExactly：元素与顺序都相同。
func exactly(got []any, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if fmt.Sprint(got[i]) != want[i] {
			return false
		}
	}
	return true
}

func TestSelfDataScopeLimitsUserListAndExportToCurrentUser(t *testing.T) {
	suffix := uniqueSuffix()
	roleID := scopeInsertRole(t, "scope_self_"+suffix, "SELF")
	scopeInsertUser(t, "scope_self_owner_"+suffix, "本人用户", 6, roleID)
	scopeInsertUser(t, "scope_self_peer_"+suffix, "同部门用户", 6, 0)

	token := login(t, "scope_self_owner_"+suffix, "admin123")

	users := get(t, "/api/system/users?pageNum=1&pageSize=10&keyword=scope_self_", token)
	if users.Status != 200 {
		t.Fatalf("users status %d: %s", users.Status, users.Raw)
	}
	page := okData(t, users)
	if num(page["total"]) != 1 {
		t.Errorf("total = %v, want 1", page["total"])
	}
	if names := pluck(users.Rows(), "username"); !exactly(names, "scope_self_owner_"+suffix) {
		t.Errorf("usernames = %v", names)
	}

	export := get(t, "/api/system/users/export?keyword=scope_self_", token)
	if export.Status != 200 {
		t.Fatalf("export status %d: %s", export.Status, export.Raw)
	}
	if ct := export.Header.Get("Content-Type"); ct != "text/csv;charset=UTF-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if !strings.Contains(export.Raw, "scope_self_owner_"+suffix) {
		t.Errorf("export lacks owner: %s", export.Raw)
	}
	if strings.Contains(export.Raw, "scope_self_peer_"+suffix) {
		t.Errorf("export leaks peer: %s", export.Raw)
	}
}

func TestDeptAndChildDataScopeIncludesCurrentDeptAndChildren(t *testing.T) {
	suffix := uniqueSuffix()
	roleID := scopeInsertRole(t, "scope_tree_"+suffix, "DEPT_AND_CHILD")
	scopeInsertUser(t, "scope_tree_owner_"+suffix, "部门负责人", 2, roleID)
	scopeInsertUser(t, "scope_tree_child_"+suffix, "子部门用户", 5, 0)
	scopeInsertUser(t, "scope_tree_other_"+suffix, "其他部门用户", 8, 0)

	token := login(t, "scope_tree_owner_"+suffix, "admin123")

	users := get(t, "/api/system/users?pageNum=1&pageSize=10&keyword=scope_tree_", token)
	if users.Status != 200 {
		t.Fatalf("users status %d: %s", users.Status, users.Raw)
	}
	if names := pluck(okRows(t, users), "username"); !exactly(names, "scope_tree_owner_"+suffix, "scope_tree_child_"+suffix) {
		t.Errorf("usernames = %v", names)
	}
}

func TestCustomDataScopeIncludesConfiguredDepartments(t *testing.T) {
	suffix := uniqueSuffix()
	roleID := scopeInsertRole(t, "scope_custom_"+suffix, "CUSTOM")
	exec(t, "INSERT INTO sys_role_dept (role_id, dept_id) VALUES (?, 8)", roleID)
	scopeInsertUser(t, "scope_custom_owner_"+suffix, "审批专员", 6, roleID)
	scopeInsertUser(t, "scope_custom_allowed_"+suffix, "授权部门用户", 8, 0)
	scopeInsertUser(t, "scope_custom_denied_"+suffix, "未授权部门用户", 11, 0)

	token := login(t, "scope_custom_owner_"+suffix, "admin123")

	users := get(t, "/api/system/users?pageNum=1&pageSize=10&keyword=scope_custom_", token)
	if users.Status != 200 {
		t.Fatalf("users status %d: %s", users.Status, users.Raw)
	}
	if names := pluck(okRows(t, users), "username"); !exactly(names, "scope_custom_owner_"+suffix, "scope_custom_allowed_"+suffix) {
		t.Errorf("usernames = %v", names)
	}
}
