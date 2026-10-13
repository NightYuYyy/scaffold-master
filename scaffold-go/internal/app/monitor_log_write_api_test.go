package app

// 对应 MonitorLogWriteApiTest。

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func monitorInsertOperLog(t testing.TB, module string) int64 {
	t.Helper()
	exec(t, `
                INSERT INTO sys_oper_log (module, action, request_method, request_url, request_params,
                    oper_name, ip, location, cost, result)
                VALUES (?, 'DELETE', 'DELETE', '/api/test', '{}', 'admin', '127.0.0.1', '本机', 12, 1)
                `, module)
	return queryInt64(t, "SELECT id FROM sys_oper_log WHERE module = ?", module)
}

func monitorInsertOperLogWithParams(t testing.TB, module, params string) int64 {
	t.Helper()
	exec(t, `
                INSERT INTO sys_oper_log (module, action, request_method, request_url, request_params,
                    oper_name, ip, location, cost, result)
                VALUES (?, 'POST', 'POST', '/api/test/params', ?, 'admin', '127.0.0.1', '本机', 12, 1)
                `, module, params)
	return queryInt64(t, "SELECT id FROM sys_oper_log WHERE request_params = ?", params)
}

func monitorInsertLoginLog(t testing.TB, username string) int64 {
	t.Helper()
	exec(t, `
                INSERT INTO sys_login_log (username, ip, location, browser, os, status, msg)
                VALUES (?, '127.0.0.1', '本机', 'JUnit', 'macOS', 1, '集成测试登录日志')
                `, username)
	return queryInt64(t, "SELECT id FROM sys_login_log WHERE username = ?", username)
}

func monitorCountByID(t testing.TB, table string, id int64) int64 {
	t.Helper()
	return queryInt64(t, "SELECT COUNT(*) FROM "+table+" WHERE id = ?", id)
}

func TestMonitorLogDeleteAndClearPersistToDatabase(t *testing.T) {
	token := loginAsAdmin(t)
	suffix := uniqueSuffix()
	operID := monitorInsertOperLog(t, "集成测试模块-"+suffix)
	loginID := monitorInsertLoginLog(t, "it_login_"+suffix)

	deleteOper := del(t, "/api/monitor/oper-logs/"+itoa(operID), token)
	if deleteOper.Status != 200 {
		t.Fatalf("delete oper status %d: %s", deleteOper.Status, deleteOper.Raw)
	}
	if n := monitorCountByID(t, "sys_oper_log", operID); n != 0 {
		t.Errorf("oper log still present: %d", n)
	}

	deleteLogin := del(t, "/api/monitor/login-logs/"+itoa(loginID), token)
	if deleteLogin.Status != 200 {
		t.Fatalf("delete login status %d: %s", deleteLogin.Status, deleteLogin.Raw)
	}
	if n := monitorCountByID(t, "sys_login_log", loginID); n != 0 {
		t.Errorf("login log still present: %d", n)
	}

	monitorInsertOperLog(t, "集成测试清空-"+suffix)
	monitorInsertLoginLog(t, "it_clear_"+suffix)

	clearOper := del(t, "/api/monitor/oper-logs", token)
	if clearOper.Status != 200 {
		t.Fatalf("clear oper status %d: %s", clearOper.Status, clearOper.Raw)
	}
	if n := queryInt64(t, "SELECT COUNT(*) FROM sys_oper_log"); n != 0 {
		t.Errorf("sys_oper_log count after clear = %d, want 0", n)
	}

	clearLogin := del(t, "/api/monitor/login-logs", token)
	if clearLogin.Status != 200 {
		t.Fatalf("clear login status %d: %s", clearLogin.Status, clearLogin.Raw)
	}
	if n := queryInt64(t, "SELECT COUNT(*) FROM sys_login_log"); n != 0 {
		t.Errorf("sys_login_log count after clear = %d, want 0", n)
	}
}

func TestMonitorLogsCanBeExportedAsCsv(t *testing.T) {
	token := loginAsAdmin(t)
	suffix := uniqueSuffix()
	module := "导出测试模块-" + suffix
	username := "export_user_" + suffix
	monitorInsertOperLog(t, module)
	monitorInsertLoginLog(t, username)

	operExport := get(t, "/api/monitor/oper-logs/export?keyword="+url.QueryEscape(module), token)
	if operExport.Status != 200 {
		t.Fatalf("oper export status %d: %s", operExport.Status, operExport.Raw)
	}
	if ct := operExport.Header.Get("Content-Type"); ct != "text/csv;charset=UTF-8" {
		t.Errorf("oper export Content-Type = %q", ct)
	}
	if cd := operExport.Header.Get("Content-Disposition"); !strings.Contains(cd, "oper-logs") {
		t.Errorf("oper export Content-Disposition = %q", cd)
	}
	if !strings.Contains(operExport.Raw, "模块,类型,请求方式,请求地址,请求参数,操作人,IP,地点,耗时,结果,操作时间,异常信息") {
		t.Errorf("oper export lacks header: %s", operExport.Raw)
	}
	if !strings.Contains(operExport.Raw, module) {
		t.Errorf("oper export lacks module %s: %s", module, operExport.Raw)
	}

	loginExport := get(t, "/api/monitor/login-logs/export?keyword="+url.QueryEscape(username), token)
	if loginExport.Status != 200 {
		t.Fatalf("login export status %d: %s", loginExport.Status, loginExport.Raw)
	}
	if ct := loginExport.Header.Get("Content-Type"); ct != "text/csv;charset=UTF-8" {
		t.Errorf("login export Content-Type = %q", ct)
	}
	if cd := loginExport.Header.Get("Content-Disposition"); !strings.Contains(cd, "login-logs") {
		t.Errorf("login export Content-Disposition = %q", cd)
	}
	if !strings.Contains(loginExport.Raw, "登录账号,IP,地点,浏览器,系统,状态,描述,登录时间") {
		t.Errorf("login export lacks header: %s", loginExport.Raw)
	}
	if !strings.Contains(loginExport.Raw, username) {
		t.Errorf("login export lacks username %s: %s", username, loginExport.Raw)
	}
}

func TestOperLogExportIncludesRequestParamsWhenKeywordMatchesParams(t *testing.T) {
	token := loginAsAdmin(t)
	suffix := "params_" + uniqueSuffix()
	monitorInsertOperLogWithParams(t, "参数导出测试", `{"keyword":"`+suffix+`"}`)

	operExport := get(t, "/api/monitor/oper-logs/export?keyword="+suffix, token)

	if operExport.Status != 200 {
		t.Fatalf("status %d: %s", operExport.Status, operExport.Raw)
	}
	if !strings.Contains(operExport.Raw, "请求参数") {
		t.Errorf("export lacks 请求参数 header: %s", operExport.Raw)
	}
	if !strings.Contains(operExport.Raw, suffix) {
		t.Errorf("export lacks params keyword %s: %s", suffix, operExport.Raw)
	}
}
