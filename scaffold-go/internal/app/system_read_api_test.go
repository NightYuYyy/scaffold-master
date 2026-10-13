package app

// 对应 SystemReadApiTest。

import (
	"net/http"
	"testing"
)

func TestUserAndRoleListsReturnFrontendFields(t *testing.T) {
	token := loginAsAdmin(t)

	users := get(t, "/api/system/users?pageNum=1&pageSize=5&keyword=admin", token)
	if users.Status != 200 {
		t.Fatalf("users status %d: %s", users.Status, users.Raw)
	}
	userPage := okData(t, users)
	if num(userPage["total"]) < 1 {
		t.Errorf("total = %v, want >= 1", userPage["total"])
	}
	rows := users.Rows()
	if len(rows) == 0 {
		t.Fatalf("users rows empty")
	}
	user := rows[0]
	if user["username"] != "admin" {
		t.Errorf("username = %v", user["username"])
	}
	if missing := hasKeys(user, "nickname", "appIds", "appNames", "roleName", "lastLogin"); len(missing) > 0 {
		t.Errorf("user missing %v: %v", missing, user)
	}
	// 未绑定应用的账号：appIds 为空数组而不是 null，appNames 为空串
	if !emptyList(user["appIds"]) {
		t.Errorf("admin appIds = %v, want []", user["appIds"])
	}
	if user["appNames"] != "" {
		t.Errorf("admin appNames = %v, want \"\"", user["appNames"])
	}
	for _, gone := range []string{"deptName", "postName"} {
		if _, ok := user[gone]; ok {
			t.Errorf("user still exposes %q: %v", gone, user)
		}
	}

	roles := get(t, "/api/system/roles?pageNum=1&pageSize=10&keyword=admin", token)
	if roles.Status != 200 {
		t.Fatalf("roles status %d: %s", roles.Status, roles.Raw)
	}
	roleRows := okRows(t, roles)
	if len(roleRows) == 0 {
		t.Fatalf("roles rows empty")
	}
	role := roleRows[0]
	if role["code"] != "super_admin" {
		t.Errorf("code = %v", role["code"])
	}
	if role["builtin"] != true {
		t.Errorf("builtin = %v", role["builtin"])
	}
	if num(role["userCount"]) < 1 {
		t.Errorf("userCount = %v, want >= 1", role["userCount"])
	}
	if _, ok := role["dataScope"]; ok {
		t.Errorf("role still exposes dataScope: %v", role)
	}
}

func TestPermissionGraphReturnsRolesMenusAndUserRoleLinks(t *testing.T) {
	r := get(t, "/api/system/permissions/graph", loginAsAdmin(t))
	if r.Status != 200 {
		t.Fatalf("status %d: %s", r.Status, r.Raw)
	}
	graph := okData(t, r)
	if graph["usersVisible"] != true {
		t.Errorf("usersVisible = %v", graph["usersVisible"])
	}
	roles := asMaps(list(graph, "roles"))
	if !containsString(pluck(roles, "code"), "super_admin") {
		t.Errorf("roles lack super_admin: %v", roles)
	}
	if len(roles) == 0 {
		t.Fatalf("roles empty")
	}
	if missing := hasKeys(roles[0], "id", "name", "builtin", "userCount", "menuIds"); len(missing) > 0 {
		t.Errorf("roles[0] missing %v: %v", missing, roles[0])
	}
	menus := asMaps(list(graph, "menus"))
	if !containsString(pluck(menus, "permission"), "system:user:remove") {
		t.Errorf("menus lack system:user:remove")
	}
	users := asMaps(list(graph, "users"))
	if !containsString(pluck(users, "username"), "admin") {
		t.Errorf("users lack admin")
	}
	if len(users) == 0 {
		t.Fatalf("users empty")
	}
	for _, k := range []string{"password", "phone", "email"} {
		if _, ok := users[0][k]; ok {
			t.Errorf("users[0] exposes %s: %v", k, users[0])
		}
	}
	links := list(graph, "userRoles")
	pair := false
	for _, l := range links {
		if arr, ok := l.([]any); ok && len(arr) == 2 {
			pair = true
		}
	}
	if !pair {
		t.Errorf("no userRoles link of size 2: %v", links)
	}
}

func TestPermissionGraphRequiresLogin(t *testing.T) {
	if r := get(t, "/api/system/permissions/graph", ""); r.Status != http.StatusUnauthorized {
		t.Errorf("status %d: %s", r.Status, r.Raw)
	}
}

func TestMenuApiReturnsTreeStructure(t *testing.T) {
	token := loginAsAdmin(t)

	menus := get(t, "/api/system/menus/tree", token)
	if menus.Status != 200 {
		t.Fatalf("menus status %d: %s", menus.Status, menus.Raw)
	}
	menuTree := okList(t, menus)
	paths := pluck(menuTree, "path")
	for _, p := range []string{"/dashboard", "/platform", "/system"} {
		if !containsString(paths, p) {
			t.Errorf("menu paths %v lack %s", paths, p)
		}
	}
	var systemMenu map[string]any
	for _, row := range menuTree {
		if row["path"] == "/system" {
			systemMenu = row
			break
		}
	}
	if systemMenu == nil {
		t.Fatalf("no /system menu")
	}
	childPaths := pluck(asMaps(list(systemMenu, "children")), "path")
	for _, p := range []string{"/system/user", "/system/role"} {
		if !containsString(childPaths, p) {
			t.Errorf("/system children %v lack %s", childPaths, p)
		}
	}

	// 部门 / 岗位接口已随用户中心改造移除
	for _, path := range []string{"/api/system/depts/tree", "/api/system/posts"} {
		if r := get(t, path, token); r.Status != http.StatusNotFound {
			t.Errorf("%s status %d, want 404: %s", path, r.Status, r.Raw)
		}
	}
}

func TestDictionaryConfigAndLogApisReturnData(t *testing.T) {
	token := loginAsAdmin(t)

	dictTypes := get(t, "/api/system/dict/types?pageNum=1&pageSize=20", token)
	if dictTypes.Status != 200 {
		t.Fatalf("dict types status %d: %s", dictTypes.Status, dictTypes.Raw)
	}
	if types := pluck(okRows(t, dictTypes), "dictType"); !containsString(types, "sys_yes_no") {
		t.Errorf("dict types %v lack sys_yes_no", types)
	}

	dictData := get(t, "/api/system/dict/data/sys_yes_no", token)
	if dictData.Status != 200 {
		t.Fatalf("dict data status %d: %s", dictData.Status, dictData.Raw)
	}
	labels := pluck(okList(t, dictData), "label")
	for _, l := range []string{"是", "否"} {
		if !containsString(labels, l) {
			t.Errorf("labels %v lack %s", labels, l)
		}
	}

	configs := get(t, "/api/system/configs?pageNum=1&pageSize=20", token)
	if configs.Status != 200 {
		t.Fatalf("configs status %d: %s", configs.Status, configs.Raw)
	}
	if keys := pluck(okRows(t, configs), "key"); !containsString(keys, "sys.user.initPassword") {
		t.Errorf("config keys %v lack sys.user.initPassword", keys)
	}

	loginLogs := get(t, "/api/monitor/login-logs?pageNum=1&pageSize=5&keyword=admin", token)
	if loginLogs.Status != 200 {
		t.Fatalf("login logs status %d: %s", loginLogs.Status, loginLogs.Raw)
	}
	if total := num(okData(t, loginLogs)["total"]); total < 1 {
		t.Errorf("login logs total = %d, want >= 1", total)
	}
}
