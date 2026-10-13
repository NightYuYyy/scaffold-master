package app

// 对应 AuthPermissionApiTest。

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// readOnlyUserFixture 对应 readOnlyFixture()：只读角色（仅菜单 101）+ 用户，注册清理；MonitorStatsApiTest 复用。
func readOnlyUserFixture(t testing.TB, prefix string) (name string, roleID int64) {
	t.Helper()
	name = prefix + uniqueSuffix()
	exec(t, "INSERT INTO sys_role (role_name, role_code, data_scope, status, deleted) VALUES (?, ?, 'SELF', 1, 0)", name, name)
	roleID = queryInt64(t, "SELECT id FROM sys_role WHERE role_code = ?", name)
	t.Cleanup(func() {
		exec(t, "DELETE FROM sys_role_menu WHERE role_id = ?", roleID)
		exec(t, "DELETE FROM sys_role WHERE id = ?", roleID)
	})
	exec(t, "INSERT INTO sys_user (username, password, nickname, dept_id, status, deleted, create_by) VALUES (?, ?, ?, 5, 1, 0, ?)",
		name, hash("admin123"), name, name)
	userID := queryInt64(t, "SELECT id FROM sys_user WHERE username = ?", name)
	t.Cleanup(func() {
		exec(t, "DELETE FROM sys_user_role WHERE user_id = ?", userID)
		exec(t, "DELETE FROM sys_user WHERE id = ?", userID)
	})
	exec(t, "INSERT INTO sys_user_role (user_id, role_id) VALUES (?, ?)", userID, roleID)
	exec(t, "INSERT INTO sys_role_menu (role_id, menu_id) VALUES (?, 101)", roleID)
	return name, roleID
}

// emptyList 对应 isEqualTo(List.of())：必须是数组且为空（null 不算）。
func emptyList(v any) bool {
	l, ok := v.([]any)
	return ok && len(l) == 0
}

func TestDisabledOrDeletedRolesCannotSupplyMenusOrPermissions(t *testing.T) {
	username, roleID := readOnlyUserFixture(t, "auth_read_")
	token := login(t, username, "admin123")
	if r := get(t, "/api/system/users", token); r.Status != 200 {
		t.Fatalf("users status %d: %s", r.Status, r.Raw)
	}
	for _, update := range []string{"status = 0", "status = 1, deleted = 1"} {
		exec(t, "UPDATE sys_role SET "+update+" WHERE id = ?", roleID)
		if me := okData(t, get(t, "/api/auth/me", token)); !emptyList(me["permissions"]) {
			t.Errorf("[%s] permissions = %v, want []", update, me["permissions"])
		}
		if routes := okData(t, get(t, "/api/auth/routes", token)); !emptyList(routes["routes"]) {
			t.Errorf("[%s] routes = %v, want []", update, routes["routes"])
		}
		if r := get(t, "/api/system/users", token); r.Status != http.StatusForbidden {
			t.Errorf("[%s] users status %d: %s", update, r.Status, r.Raw)
		}
	}
}

func TestReadOnlyUserGetsFilterOptionsWithoutManagementPermissions(t *testing.T) {
	username, _ := readOnlyUserFixture(t, "auth_read_")
	token := login(t, username, "admin123")
	options := okData(t, get(t, "/api/system/users/options", token))
	if !emptyList(options["roles"]) {
		t.Errorf("roles = %v, want []", options["roles"])
	}
	if !emptyList(options["posts"]) {
		t.Errorf("posts = %v, want []", options["posts"])
	}
	if options["initPassword"] != "" {
		t.Errorf("initPassword = %v, want \"\"", options["initPassword"])
	}
	if depts, ok := options["depts"].([]any); !ok || len(depts) == 0 {
		t.Errorf("depts = %v, want non-empty", options["depts"])
	}
	raw, _ := json.Marshal(options)
	for _, s := range []string{"phone", "email", "leader"} {
		if strings.Contains(string(raw), s) {
			t.Errorf("options contain %q: %s", s, raw)
		}
	}
	if r := get(t, "/api/system/dict/options/sys_normal_disable", token); r.Status != 200 {
		t.Errorf("dict options status %d: %s", r.Status, r.Raw)
	}
	for _, path := range []string{"/api/system/roles", "/api/system/posts", "/api/system/configs", "/api/system/dict/data/sys_normal_disable"} {
		if r := get(t, path, token); r.Status != http.StatusForbidden {
			t.Errorf("%s status %d: %s", path, r.Status, r.Raw)
		}
	}
	if r := post(t, "/api/system/users", token, map[string]any{"username": "blocked"}); r.Status != http.StatusForbidden {
		t.Errorf("POST users status %d: %s", r.Status, r.Raw)
	}
	if r := get(t, "/api/system/users/options", ""); r.Status != http.StatusUnauthorized {
		t.Errorf("anonymous options status %d: %s", r.Status, r.Raw)
	}
	if r := get(t, "/api/system/dict/options/sys_normal_disable", ""); r.Status != http.StatusUnauthorized {
		t.Errorf("anonymous dict options status %d: %s", r.Status, r.Raw)
	}
}

func TestRapidReloginDoesNotReuseARevokedToken(t *testing.T) {
	oldToken := loginAsAdmin(t)
	post(t, "/api/auth/logout", oldToken, nil)
	newToken := loginAsAdmin(t)
	if oldToken == newToken {
		t.Fatalf("new token equals revoked token")
	}
	if r := get(t, "/api/auth/me", oldToken); r.Status != http.StatusUnauthorized {
		t.Errorf("old token status %d: %s", r.Status, r.Raw)
	}
	if r := get(t, "/api/auth/me", newToken); r.Status != 200 {
		t.Errorf("new token status %d: %s", r.Status, r.Raw)
	}
}

func TestDatabaseSessionUsesConfiguredTimezone(t *testing.T) {
	zone := os.Getenv("DB_TIMEZONE")
	if zone == "" {
		zone = "+08:00"
	}
	// ZoneId.of(zone) 同时接受 "+08:00" 与 "Asia/Shanghai"。
	var offsetSeconds int
	if parsed, err := time.Parse("-07:00", zone); err == nil {
		_, offsetSeconds = parsed.Zone()
	} else {
		loc, err := time.LoadLocation(zone)
		if err != nil {
			t.Fatalf("invalid DB_TIMEZONE %q: %v", zone, err)
		}
		_, offsetSeconds = time.Now().In(loc).Zone()
	}
	expectedMinutes := int64(offsetSeconds / 60)
	if got := queryInt64(t, "SELECT TIMESTAMPDIFF(MINUTE, UTC_TIMESTAMP(), NOW())"); got != expectedMinutes {
		t.Errorf("session offset = %d minutes, want %d", got, expectedMinutes)
	}
}
