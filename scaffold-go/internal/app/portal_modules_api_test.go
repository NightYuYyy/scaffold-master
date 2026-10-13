package app

// 对应 PortalModulesApiTest：工作台（docs/platform-api.md「工作台」）与代码生成器。

import (
	"strings"
	"testing"
)

// seedPortalRows 对应 @BeforeEach seedPortalRows。
func seedPortalRows(t testing.TB) {
	t.Helper()
	exec(t, `
                INSERT INTO gen_table (table_name, table_comment, class_name, module_name, business_name, synced, create_by, update_by)
                VALUES ('it_purchase_order', '集成测试采购订单', 'PurchaseOrder', 'demo', 'purchaseOrder', 1, 'test', 'test')
                ON DUPLICATE KEY UPDATE table_comment = VALUES(table_comment), class_name = VALUES(class_name),
                    module_name = VALUES(module_name), business_name = VALUES(business_name), synced = 1, update_by = 'test'
                `)
	tableID := queryInt64(t, "SELECT id FROM gen_table WHERE table_name = 'it_purchase_order'")
	exec(t, `
                INSERT INTO gen_table_column (table_id, column_name, java_field, java_type, jdbc_type, column_comment,
                    is_insert, is_edit, is_list, is_query, query_type, form_type, required, sort)
                SELECT ?, 'order_no', 'orderNo', 'String', 'varchar', '订单编号', 1, 1, 1, 1, 'LIKE', '文本框', 1, 1
                WHERE NOT EXISTS (
                    SELECT 1 FROM gen_table_column WHERE table_id = ? AND column_name = 'order_no'
                )
                `, tableID, tableID)
	exec(t, `
                INSERT INTO gen_table_column (table_id, column_name, java_field, java_type, jdbc_type, column_comment,
                    is_insert, is_edit, is_list, is_query, query_type, form_type, required, sort)
                SELECT ?, 'amount', 'amount', 'BigDecimal', 'decimal', '采购金额', 1, 1, 1, 0, '=', '数字输入框', 1, 2
                WHERE NOT EXISTS (
                    SELECT 1 FROM gen_table_column WHERE table_id = ? AND column_name = 'amount'
                )
                `, tableID, tableID)
}

// seedDashboardActivity 往示例应用（id 1）写入一名刚注册的应用用户、一条成功登录与一笔积分流水，并注册清理。
func seedDashboardActivity(t testing.TB) (username, refNo string) {
	t.Helper()
	suffix := uniqueSuffix()
	username, refNo = "dash_user_"+suffix, "it-dash-"+suffix
	exec(t, "INSERT INTO app_user (app_id, username, nickname, status, deleted, create_by, create_time) VALUES (1, ?, '工作台测试用户', 1, 0, 'it', NOW())", username)
	exec(t, "INSERT INTO app_auth_log (app_id, user_id, username, event, status, ip, msg, event_time) VALUES (1, NULL, ?, 'login', 1, '127.0.0.1', '登录成功', NOW())", username)
	exec(t, "INSERT INTO app_counter_log (app_id, user_id, field_key, amount, balance_after, reason, ref_no, operator, create_time) VALUES (1, 1, 'coins', 5, 105, '工作台测试', ?, 'admin:it', NOW())", refNo)
	t.Cleanup(func() {
		exec(t, "DELETE FROM app_counter_log WHERE ref_no = ?", refNo)
		exec(t, "DELETE FROM app_auth_log WHERE username = ?", username)
		exec(t, "DELETE FROM app_user WHERE username = ?", username)
	})
	return username, refNo
}

// isNumber 断言 JSON 数字。
func isNumber(v any) bool { _, ok := v.(float64); return ok }

func TestDashboardSummaryReturnsDocumentedShapeAndCounts(t *testing.T) {
	username, _ := seedDashboardActivity(t)
	token := loginAsAdmin(t)

	for rng, buckets := range map[string]int{"24h": 24, "7d": 7, "30d": 30} {
		r := get(t, "/api/dashboard/summary?range="+rng, token)
		if r.Status != 200 {
			t.Fatalf("[%s] status %d: %s", rng, r.Status, r.Raw)
		}
		summary := okData(t, r)
		if summary["range"] != rng {
			t.Errorf("[%s] range = %v", rng, summary["range"])
		}

		stats := asMaps(list(summary, "stats"))
		if keys := pluck(stats, "key"); !exactly(keys, "apps", "users", "registrations", "logins", "operators") {
			t.Fatalf("[%s] stats keys = %v", rng, keys)
		}
		byKey := map[string]map[string]any{}
		for _, stat := range stats {
			byKey[stat["key"].(string)] = stat
			if missing := hasKeys(stat, "label", "value", "sub"); len(missing) > 0 {
				t.Errorf("[%s] stat %v missing %v", rng, stat["key"], missing)
			}
			if !isNumber(stat["value"]) || num(stat["value"]) < 0 {
				t.Errorf("[%s] stat %v value = %v, want number >= 0", rng, stat["key"], stat["value"])
			}
		}
		for _, k := range []string{"users", "registrations", "logins"} {
			stat := byKey[k]
			if !isNumber(stat["delta"]) {
				t.Errorf("[%s] %s delta = %v, want number", rng, k, stat["delta"])
			}
			if _, ok := stat["up"].(bool); !ok {
				t.Errorf("[%s] %s up = %v, want bool", rng, k, stat["up"])
			}
			if _, ok := stat["fresh"].(bool); !ok {
				t.Errorf("[%s] %s fresh = %v, want bool", rng, k, stat["fresh"])
			}
		}
		if num(byKey["apps"]["value"]) < 1 || num(byKey["users"]["value"]) < 1 ||
			num(byKey["registrations"]["value"]) < 1 || num(byKey["logins"]["value"]) < 1 || num(byKey["operators"]["value"]) < 2 {
			t.Errorf("[%s] seeded rows not counted: %v", rng, stats)
		}
		if sub, _ := byKey["logins"]["sub"].(string); !strings.HasPrefix(sub, "成功率 ") || !strings.HasSuffix(sub, "%") {
			t.Errorf("[%s] logins sub = %q", rng, sub)
		}

		trend, _ := summary["trend"].(map[string]any)
		if trend == nil {
			t.Fatalf("[%s] trend missing: %v", rng, summary)
		}
		labels := list(trend, "labels")
		if len(labels) != buckets || len(list(trend, "registrations")) != buckets || len(list(trend, "logins")) != buckets {
			t.Errorf("[%s] trend lengths = %d/%d/%d, want %d", rng, len(labels), len(list(trend, "registrations")), len(list(trend, "logins")), buckets)
		}
		sep := "-"
		if rng == "24h" {
			sep = ":"
		}
		for i, label := range labels {
			if s, _ := label.(string); len(s) != 5 || s[2:3] != sep || (rng == "24h" && !strings.HasSuffix(s, ":00")) {
				t.Errorf("[%s] labels[%d] = %v", rng, i, label)
			}
		}
		var regSum, loginSum int64
		for i := 0; i < buckets; i++ {
			regSum += num(list(trend, "registrations")[i])
			loginSum += num(list(trend, "logins")[i])
		}
		if regSum < 1 || loginSum < 1 {
			t.Errorf("[%s] trend sums = %d/%d, want >= 1 (last bucket should hold the seeded rows)", rng, regSum, loginSum)
		}

		apps := asMaps(list(summary, "apps"))
		var demo map[string]any
		for _, app := range apps {
			if app["code"] == "demo" {
				demo = app
			}
		}
		if demo == nil {
			t.Fatalf("[%s] apps lack demo: %v", rng, apps)
		}
		if missing := hasKeys(demo, "id", "code", "name", "users", "registrations", "logins"); len(missing) > 0 {
			t.Errorf("[%s] demo app missing %v: %v", rng, missing, demo)
		}
		if num(demo["users"]) < 1 || num(demo["registrations"]) < 1 || num(demo["logins"]) < 1 {
			t.Errorf("[%s] demo app counts = %v", rng, demo)
		}

		activities := asMaps(list(summary, "activities"))
		hit := false
		for _, a := range activities {
			if missing := hasKeys(a, "app", "who", "action", "time", "tint"); len(missing) > 0 {
				t.Errorf("[%s] activity missing %v: %v", rng, missing, a)
			}
			if a["who"] == username && a["action"] == "登录" && a["tint"] == "ok" && a["app"] == "示例应用" {
				hit = true
			}
		}
		if !hit {
			t.Errorf("[%s] activities lack seeded login of %s: %v", rng, username, activities)
		}

		counters := asMaps(list(summary, "counters"))
		hit = false
		for _, c := range counters {
			if missing := hasKeys(c, "app", "fieldKey", "label", "total", "changes"); len(missing) > 0 {
				t.Errorf("[%s] counter missing %v: %v", rng, missing, c)
			}
			if c["app"] == "示例应用" && c["fieldKey"] == "coins" && c["label"] == "积分" && isNumber(c["total"]) && num(c["total"]) >= 5 && num(c["changes"]) >= 1 {
				hit = true
			}
		}
		if !hit {
			t.Errorf("[%s] counters lack seeded coins flow: %v", rng, counters)
		}
	}

	if d := okData(t, get(t, "/api/dashboard/summary?range=unknown", token)); d["range"] != "24h" {
		t.Errorf("range=unknown → %v, want 24h", d["range"])
	}
}

func TestDashboardSummaryScopesToBoundAppsForNonSuperAdmin(t *testing.T) {
	seedDashboardActivity(t)

	// 未绑定应用：应用相关统计全部为 0、列表为空；后台账号数不受应用范围影响
	unbound, _ := readOnlyUserFixture(t, "dash_none_")
	summary := okData(t, get(t, "/api/dashboard/summary", login(t, unbound, "admin123")))
	for _, stat := range asMaps(list(summary, "stats")) {
		switch stat["key"] {
		case "operators":
			if num(stat["value"]) < 2 {
				t.Errorf("unbound operators = %v, want >= 2", stat["value"])
			}
		default:
			if num(stat["value"]) != 0 {
				t.Errorf("unbound stat %v = %v, want 0", stat["key"], stat["value"])
			}
		}
	}
	for _, k := range []string{"apps", "activities", "counters"} {
		if !emptyList(summary[k]) {
			t.Errorf("unbound %s = %v, want []", k, summary[k])
		}
	}
	trend, _ := summary["trend"].(map[string]any)
	if len(list(trend, "labels")) != 24 {
		t.Errorf("unbound trend labels = %v, want 24 buckets", trend["labels"])
	}

	// 绑定示例应用：只看到该应用
	bound, _ := readOnlyUserFixture(t, "dash_app_", 1)
	summary = okData(t, get(t, "/api/dashboard/summary?range=7d", login(t, bound, "admin123")))
	apps := asMaps(list(summary, "apps"))
	if len(apps) != 1 || num(apps[0]["id"]) != 1 {
		t.Errorf("bound apps = %v, want only app 1", apps)
	}
	for _, stat := range asMaps(list(summary, "stats")) {
		if stat["key"] == "apps" && num(stat["value"]) != 1 {
			t.Errorf("bound apps stat = %v, want 1", stat["value"])
		}
		if stat["key"] == "logins" && num(stat["value"]) < 1 {
			t.Errorf("bound logins stat = %v, want >= 1", stat["value"])
		}
	}
	if r := get(t, "/api/dashboard/summary", ""); r.Status != 401 {
		t.Errorf("anonymous status %d: %s", r.Status, r.Raw)
	}
}

func TestGeneratorApisExposeTableColumnsPreviewAndSave(t *testing.T) {
	seedPortalRows(t)
	token := loginAsAdmin(t)

	tables := get(t, "/api/tool/generator/tables?pageNum=1&pageSize=10&keyword=it_purchase_order", token)
	if tables.Status != 200 {
		t.Fatalf("tables status %d: %s", tables.Status, tables.Raw)
	}
	rows := okRows(t, tables)
	if len(rows) == 0 {
		t.Fatalf("no gen tables: %s", tables.Raw)
	}
	table := rows[0]
	if table["tableName"] != "it_purchase_order" {
		t.Errorf("tableName = %v", table["tableName"])
	}
	if missing := hasKeys(table, "tableComment", "className", "module", "synced", "createTime"); len(missing) > 0 {
		t.Errorf("table missing %v: %v", missing, table)
	}
	tableID := itoa(num(table["id"]))

	columns := get(t, "/api/tool/generator/tables/"+tableID+"/columns", token)
	if columns.Status != 200 {
		t.Fatalf("columns status %d: %s", columns.Status, columns.Raw)
	}
	fields := pluck(okList(t, columns), "javaField")
	for _, f := range []string{"orderNo", "amount"} {
		if !containsString(fields, f) {
			t.Errorf("javaFields %v lack %s", fields, f)
		}
	}

	preview := get(t, "/api/tool/generator/tables/"+tableID+"/preview", token)
	if preview.Status != 200 {
		t.Fatalf("preview status %d: %s", preview.Status, preview.Raw)
	}
	previewData := okData(t, preview)
	names := pluck(asMaps(list(previewData, "files")), "name")
	for _, n := range []string{"it_purchase_order_handler.go", "it_purchase_order_sql.go", "index.vue"} {
		if !containsString(names, n) {
			t.Errorf("preview files %v lack %s", names, n)
		}
	}
	if code, _ := previewData["code"].(string); !strings.Contains(code, "func RegisterPurchaseOrderRoutes(") {
		t.Errorf("preview code lacks RegisterPurchaseOrderRoutes")
	}

	save := put(t, "/api/tool/generator/tables/"+tableID, token, map[string]any{"tableComment": "集成测试采购订单-已保存", "synced": false})
	if save.Status != 200 {
		t.Errorf("save status %d: %s", save.Status, save.Raw)
	}
}
