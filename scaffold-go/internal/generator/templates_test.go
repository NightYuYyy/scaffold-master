package generator

import (
	"go/format"
	"strings"
	"testing"

	"scaffold/internal/framework"
)

func TestRenderFiles(t *testing.T) {
	table := framework.Row{"tableName": "sys_demo_item", "className": "SysDemoItem", "module": "system"}
	// 形如 selectColumns 的行：tinyint 列为 int64，NULL 为 nil。
	columns := []framework.Row{
		{"col": "id", "javaField": lowerCamel("id"), "javaType": javaType("bigint"), "comment": nil, "list": int64(0)},
		{"col": "item_name", "javaField": lowerCamel("item_name"), "javaType": javaType("varchar"), "comment": "名称", "list": int64(1)},
		{"col": "amount", "javaField": lowerCamel("amount"), "javaType": javaType("decimal"), "comment": nil, "list": int64(1)},
		{"col": "create_time", "javaField": lowerCamel("create_time"), "javaType": javaType("datetime"), "comment": "创建时间", "list": int64(1)},
	}
	files := renderFiles(table, columns)
	if len(files) != 7 {
		t.Fatalf("want 7 files, got %d", len(files))
	}
	wantPaths := []string{
		"backend/internal/system/sys_demo_item_model.go",
		"backend/internal/system/sys_demo_item_sql.go",
		"backend/internal/system/sys_demo_item_service.go",
		"backend/internal/system/sys_demo_item_handler.go",
		"frontend/src/api/system/sysDemoItem.ts",
		"frontend/src/views/system/sysDemoItem/index.vue",
		"sql/sys_demo_item_menu.sql",
	}
	for i, want := range wantPaths {
		if files[i].Path != want {
			t.Errorf("file %d path = %s, want %s", i, files[i].Path, want)
		}
	}
	if activeFileName("sys_demo_item") != files[3].Name {
		t.Errorf("active file should be the handler, got %s", activeFileName("sys_demo_item"))
	}

	// 四个 Go 文件必须是合法且已格式化的 Go 源码
	for _, f := range files[:4] {
		formatted, err := format.Source([]byte(f.Code))
		if err != nil {
			t.Errorf("%s is not valid Go: %v\n%s", f.Name, err, f.Code)
		} else if string(formatted) != f.Code {
			t.Errorf("%s is not gofmt-ed", f.Name)
		}
		if !strings.HasPrefix(f.Code, "// Code generated") || !strings.Contains(f.Code, "\npackage system\n") {
			t.Errorf("%s missing header/package", f.Name)
		}
	}

	model := files[0].Code
	collapsed := strings.Join(strings.Fields(model), " ") // gofmt 会对齐字段列，比较前折叠空白
	for _, want := range []string{"type SysDemoItem struct", "Id int64 `json:\"id\"`", "ItemName string `json:\"itemName\"` // 名称",
		"Amount float64 `json:\"amount\"`", "CreateTime string `json:\"createTime\"` // 创建时间"} {
		if !strings.Contains(collapsed, want) {
			t.Errorf("model missing %q:\n%s", want, model)
		}
	}
	sqlFile := files[1].Code
	for _, want := range []string{"func countSysDemoItem(", "func selectSysDemoItemList(", "item_name AS itemName", "FROM sys_demo_item", "ORDER BY id DESC"} {
		if !strings.Contains(sqlFile, want) {
			t.Errorf("sql file missing %q:\n%s", want, sqlFile)
		}
	}
	if !strings.Contains(files[2].Code, "func (s *SysDemoItemService) List(") || !strings.Contains(files[2].Code, "framework.PageOf(rows, total, q)") {
		t.Errorf("service missing List:\n%s", files[2].Code)
	}
	handler := files[3].Code
	for _, want := range []string{"func RegisterSysDemoItemRoutes(", `"GET /api/system/sysDemoItem"`, `sec.Perm("system:sysDemoItem:list")`} {
		if !strings.Contains(handler, want) {
			t.Errorf("handler missing %q:\n%s", want, handler)
		}
	}

	menu := files[6].Code
	for _, want := range []string{"'/system/sysDemoItem'", "'system:sysDemoItem:list'"} {
		if !strings.Contains(menu, want) {
			t.Errorf("menu sql missing %q", want)
		}
	}
	if !strings.Contains(files[5].Code, "const primaryField = 'itemName'") {
		t.Errorf("view should pick first list-enabled field, got:\n%s", files[5].Code)
	}
}

func TestNaming(t *testing.T) {
	cases := map[string][2]string{ // tableName → {className, module}
		"sys_demo_item": {"SysDemoItem", "system"},
		"biz_order":     {"BizOrder", "biz"},
		"plain":         {"Plain", "system"},
		"_lead":         {"Lead", "system"},
		"a__b_":         {"AB", "a"},
	}
	for in, want := range cases {
		if got := upperCamel(in); got != want[0] {
			t.Errorf("upperCamel(%q) = %q, want %q", in, got, want[0])
		}
		if got := moduleName(in); got != want[1] {
			t.Errorf("moduleName(%q) = %q, want %q", in, got, want[1])
		}
	}
	if businessName("sys_demo_item") != "demoItem" || businessName("plain") != "plain" || decapitalize("SysDemoItem") != "sysDemoItem" {
		t.Errorf("businessName/decapitalize mismatch")
	}
	if goType("Long") != "int64" || goType("Integer") != "int" || goType("BigDecimal") != "float64" || goType("LocalDateTime") != "string" || goType("String") != "string" {
		t.Errorf("goType mapping mismatch")
	}
}
