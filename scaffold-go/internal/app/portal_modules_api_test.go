package app

// 对应 PortalModulesApiTest。

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

func TestDashboardSummaryReturnsRealCountsAndActivities(t *testing.T) {
	seedPortalRows(t)
	token := loginAsAdmin(t)

	r := get(t, "/api/dashboard/summary", token)

	if r.Status != 200 {
		t.Fatalf("status %d: %s", r.Status, r.Raw)
	}
	summary := okData(t, r)
	statKeys := pluck(asMaps(list(summary, "stats")), "key")
	for _, k := range []string{"users", "online", "roles", "api", "menus", "depts"} {
		if !containsString(statKeys, k) {
			t.Errorf("stats keys %v lack %s", statKeys, k)
		}
	}
	if l := list(summary, "trend"); len(l) != 24 {
		t.Errorf("trend len = %d, want 24", len(l))
	}
	activities := asMaps(list(summary, "activities"))
	if len(activities) == 0 {
		t.Fatalf("activities empty: %v", summary["activities"])
	}
	if missing := hasKeys(activities[0], "who", "action", "target", "time", "tint"); len(missing) > 0 {
		t.Errorf("activities[0] missing %v: %v", missing, activities[0])
	}
	if _, ok := summary["trendPrev"]; !ok {
		t.Errorf("summary lacks trendPrev")
	}
	dailyOps := asMaps(list(summary, "dailyOps"))
	if len(dailyOps) != 7 {
		t.Errorf("dailyOps len = %d, want 7", len(dailyOps))
	}
	for i, day := range dailyOps {
		if missing := hasKeys(day, "date", "total", "failed"); len(missing) > 0 {
			t.Errorf("dailyOps[%d] missing %v: %v", i, missing, day)
		}
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
