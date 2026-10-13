package app

// 对应 GeneratorImportApiTest。

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"scaffold/internal/framework"
)

// cleanImportedSysFile 对应 @BeforeEach cleanImportedTable。
func cleanImportedSysFile(t testing.TB) {
	t.Helper()
	ids, err := framework.SelectInt64s(t.Context(), testDB, "SELECT id FROM gen_table WHERE table_name = 'sys_file'")
	if err != nil {
		t.Fatalf("query gen_table: %v", err)
	}
	for _, tableID := range ids {
		exec(t, "DELETE FROM gen_table_column WHERE table_id = ?", tableID)
		exec(t, "DELETE FROM gen_table WHERE id = ?", tableID)
	}
}

// unzipFiles 对应 unzip(byte[])。
func unzipFiles(t testing.TB, body string) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader([]byte(body)), int64(len(body)))
	if err != nil {
		t.Fatalf("unzip: %v", err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(b)
	}
	return files
}

func TestGeneratorCanListAndImportDatabaseTables(t *testing.T) {
	cleanImportedSysFile(t)
	token := loginAsAdmin(t)

	candidates := get(t, "/api/tool/generator/importable-tables?keyword=sys_file", token)
	if candidates.Status != 200 {
		t.Fatalf("importable-tables status %d: %s", candidates.Status, candidates.Raw)
	}
	cands := okList(t, candidates)
	if len(cands) == 0 {
		t.Fatalf("no importable candidates: %s", candidates.Raw)
	}
	candidate := cands[0]
	if candidate["tableName"] != "sys_file" {
		t.Errorf("tableName = %v", candidate["tableName"])
	}
	if candidate["tableComment"] != "文件信息表" {
		t.Errorf("tableComment = %v", candidate["tableComment"])
	}
	if candidate["imported"] != false {
		t.Errorf("imported = %v, want false", candidate["imported"])
	}

	imported := post(t, "/api/tool/generator/import", token, map[string]any{"tableNames": []string{"sys_file"}})
	if imported.Status != 200 {
		t.Fatalf("import status %d: %s", imported.Status, imported.Raw)
	}
	if n := num(okData(t, imported)["imported"]); n != 1 {
		t.Errorf("imported = %d, want 1", n)
	}

	tableID := queryInt64(t, "SELECT id FROM gen_table WHERE table_name = 'sys_file'")
	if tableID == 0 {
		t.Fatalf("gen_table id is null")
	}
	if n := queryInt64(t, "SELECT COUNT(*) FROM gen_table_column WHERE table_id = ? AND column_name = 'original_name'", tableID); n != 1 {
		t.Errorf("original_name column count = %d, want 1", n)
	}
}

func TestGeneratorCanDownloadCodeZip(t *testing.T) {
	cleanImportedSysFile(t)
	token := loginAsAdmin(t)
	imported := post(t, "/api/tool/generator/import", token, map[string]any{"tableNames": []string{"sys_file"}})
	if imported.Status != 200 {
		t.Fatalf("import status %d: %s", imported.Status, imported.Raw)
	}
	tableID := queryInt64(t, "SELECT id FROM gen_table WHERE table_name = 'sys_file'")

	download := get(t, "/api/tool/generator/tables/"+itoa(tableID)+"/download", token)

	if download.Status != 200 {
		t.Fatalf("download status %d: %s", download.Status, download.Raw)
	}
	if ct := download.Header.Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := download.Header.Get("Content-Disposition"); !strings.Contains(cd, "SysFile.zip") {
		t.Errorf("Content-Disposition = %q", cd)
	}

	files := unzipFiles(t, download.Raw)
	for _, name := range []string{
		"backend/internal/system/sys_file_model.go",
		"backend/internal/system/sys_file_sql.go",
		"backend/internal/system/sys_file_service.go",
		"backend/internal/system/sys_file_handler.go",
		"frontend/src/api/system/sysFile.ts",
		"frontend/src/views/system/sysFile/index.vue",
		"sql/sys_file_menu.sql",
	} {
		if _, ok := files[name]; !ok {
			t.Errorf("zip lacks %s (has %v)", name, keysOf(files))
		}
	}
	handler := files["backend/internal/system/sys_file_handler.go"]
	for _, s := range []string{"func RegisterSysFileRoutes(", `"GET /api/system/sysFile"`, `sec.Perm("system:sysFile:list")`} {
		if !strings.Contains(handler, s) {
			t.Errorf("handler lacks %q", s)
		}
	}
	if model := files["backend/internal/system/sys_file_model.go"]; !strings.Contains(model, "type SysFile struct") {
		t.Errorf("model lacks struct: %s", model)
	}
	vue := files["frontend/src/views/system/sysFile/index.vue"]
	for _, s := range []string{"SysFile", "sys_file"} {
		if !strings.Contains(vue, s) {
			t.Errorf("index.vue lacks %q", s)
		}
	}
	menuSQL := files["sql/sys_file_menu.sql"]
	if !strings.Contains(menuSQL, "sys_file") {
		t.Errorf("menu sql lacks sys_file: %s", menuSQL)
	}
	menuCount := queryInt64(t, "SELECT COUNT(*) FROM sys_menu")
	// 对应 TransactionTemplate + setRollbackOnly：在事务内执行生成的 SQL 并回滚。
	tx, err := testDB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(t.Context(), menuSQL); err != nil {
		t.Fatalf("execute generated menu sql: %v\n%s", err, menuSQL)
	}
	inTx, err := framework.SelectInt64(t.Context(), tx, "SELECT COUNT(*) FROM sys_menu")
	if err != nil {
		t.Fatalf("count in tx: %v", err)
	}
	if inTx != menuCount+1 {
		t.Errorf("menu count in tx = %d, want %d", inTx, menuCount+1)
	}
	row, err := framework.SelectRow(t.Context(), tx, "SELECT menu_type, permission FROM sys_menu WHERE id = (SELECT MAX(id) FROM sys_menu)")
	if err != nil {
		t.Fatalf("select new menu: %v", err)
	}
	if row["menu_type"] != "menu" {
		t.Errorf("menu_type = %v, want menu", row["menu_type"])
	}
	if row["permission"] != "system:sysFile:list" {
		t.Errorf("permission = %v, want system:sysFile:list", row["permission"])
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if after := queryInt64(t, "SELECT COUNT(*) FROM sys_menu"); after != menuCount {
		t.Errorf("menu count after rollback = %d, want %d", after, menuCount)
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
