package app

// 对应 OperationLogAspectApiTest。

import (
	"net/url"
	"strings"
	"testing"
)

// operLogCreateUser 对应 createUser(headers, suffix)，并在清理阶段删除该用户（Java 在用例末尾删除）。
func operLogCreateUser(t testing.TB, token, suffix string) int64 {
	t.Helper()
	create := post(t, "/api/system/users", token, map[string]any{
		"username": "log_user_" + suffix,
		"nickname": "日志切面用户" + suffix,
		"password": "admin123",
		"phone":    "",
		"email":    "log_user_" + suffix + "@example.com",
		"status":   1,
		"roleIds":  []int{4},
		"appIds":   []int{1},
	})
	if create.Status != 200 {
		t.Fatalf("create user status %d: %s", create.Status, create.Raw)
	}
	id := num(okData(t, create)["id"])
	t.Cleanup(func() { del(t, "/api/system/users/"+itoa(id), token) })
	return id
}

func TestAnnotatedWriteEndpointCreatesOperationLog(t *testing.T) {
	suffix := uniqueSuffix()
	roleName := "日志切面测试角色" + suffix
	token := loginAsAdmin(t)

	create := post(t, "/api/system/roles", token, map[string]any{
		"name":   roleName,
		"code":   "log_" + suffix,
		"sort":   88,
		"status": 1,
		"remark": "操作日志切面测试",
	})
	if create.Status != 200 {
		t.Fatalf("create status %d: %s", create.Status, create.Raw)
	}
	id := num(okData(t, create)["id"])
	t.Cleanup(func() { del(t, "/api/system/roles/"+itoa(id), token) })

	// 按本用例的角色名定位日志行，避免并发用例的新增角色日志混入
	log := queryMap(t, `
                SELECT module, action, request_method, request_url, request_params, oper_name, result, error_msg
                FROM sys_oper_log
                WHERE module = '角色管理' AND action = '新增' AND request_params LIKE CONCAT('%', ?, '%')
                ORDER BY id DESC
                LIMIT 1
                `, roleName)
	if log == nil {
		t.Fatalf("no oper log row for %s", roleName)
	}
	for k, want := range map[string]string{
		"module":         "角色管理",
		"action":         "新增",
		"request_method": "POST",
		"request_url":    "/api/system/roles",
		"oper_name":      "admin",
	} {
		if got := log[k]; got != want {
			t.Errorf("%s = %v, want %s", k, got, want)
		}
	}
	if num(log["result"]) != 1 {
		t.Errorf("result = %v, want 1", log["result"])
	}
	if log["error_msg"] != nil {
		t.Errorf("error_msg = %v, want null", log["error_msg"])
	}

	logs := get(t, "/api/monitor/oper-logs?pageNum=1&pageSize=5&keyword="+url.QueryEscape(roleName), token)
	if logs.Status != 200 {
		t.Fatalf("logs status %d: %s", logs.Status, logs.Raw)
	}
	found := false
	for _, row := range okRows(t, logs) {
		if row["module"] == "角色管理" && row["action"] == "新增" && row["url"] == "/api/system/roles" {
			found = true
			if _, ok := row["dept"]; ok {
				t.Errorf("oper log row still exposes dept: %v", row)
			}
		}
	}
	if !found {
		t.Errorf("no matching oper log row: %s", logs.Raw)
	}
}

func TestOperationLogMasksPasswordFields(t *testing.T) {
	suffix := uniqueSuffix()
	token := loginAsAdmin(t)
	id := operLogCreateUser(t, token, suffix)

	reset := put(t, "/api/system/users/"+itoa(id)+"/password", token, map[string]any{"password": "Sensitive@123"})
	if reset.Status != 200 {
		t.Fatalf("reset status %d: %s", reset.Status, reset.Raw)
	}

	params := queryString(t, `
                SELECT request_params
                FROM sys_oper_log
                WHERE module = '后台账号' AND action = '重置密码' AND request_url = ?
                ORDER BY id DESC
                LIMIT 1
                `, "/api/system/users/"+itoa(id)+"/password")
	if strings.Contains(params, "Sensitive@123") {
		t.Errorf("request_params leaks password: %s", params)
	}
	if !strings.Contains(params, `"password":"******"`) {
		t.Errorf("request_params not masked: %s", params)
	}
}
