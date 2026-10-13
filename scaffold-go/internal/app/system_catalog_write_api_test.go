package app

// 对应 SystemCatalogWriteApiTest。

import (
	"net/http"
	"testing"
)

// idOf 对应 idOf(body)：断言 code=200 并取 data.id。
func idOf(t testing.TB, r resp) int64 {
	t.Helper()
	return num(okData(t, r)["id"])
}

func TestRoleCrudPersistsAndSoftDeletesRecord(t *testing.T) {
	suffix := uniqueSuffix()
	token := loginAsAdmin(t)

	create := post(t, "/api/system/roles", token, map[string]any{
		"name":   "集成测试角色" + suffix,
		"code":   "it_role_" + suffix,
		"sort":   88,
		"status": 1,
		"remark": "集成测试创建",
	})
	if create.Status != 200 {
		t.Fatalf("create status %d: %s", create.Status, create.Raw)
	}
	id := itoa(idOf(t, create))

	update := put(t, "/api/system/roles/"+id, token, map[string]any{
		"name":   "集成测试角色已更新" + suffix,
		"sort":   89,
		"status": 0,
		"remark": "已更新",
	})
	if update.Status != 200 {
		t.Fatalf("update status %d: %s", update.Status, update.Raw)
	}

	rows := okRows(t, get(t, "/api/system/roles?pageNum=1&pageSize=10&keyword="+suffix, token))
	if len(rows) == 0 {
		t.Fatalf("role list empty")
	}
	role := rows[0]
	if role["name"] != "集成测试角色已更新"+suffix {
		t.Errorf("name = %v", role["name"])
	}
	if num(role["status"]) != 0 {
		t.Errorf("status = %v, want 0", role["status"])
	}
	if _, ok := role["dataScope"]; ok {
		t.Errorf("role still exposes dataScope: %v", role)
	}

	if r := del(t, "/api/system/roles/"+id, token); r.Status != 200 {
		t.Fatalf("delete status %d: %s", r.Status, r.Raw)
	}
	if rows := okRows(t, get(t, "/api/system/roles?pageNum=1&pageSize=10&keyword="+suffix, token)); len(rows) != 0 {
		t.Errorf("rows after delete = %v", rows)
	}
}

func TestMenuCrudPersistsAndDeletesRecord(t *testing.T) {
	suffix := uniqueSuffix()
	token := loginAsAdmin(t)

	menuCreate := post(t, "/api/system/menus", token, map[string]any{
		"parentId":   200,
		"name":       "集成测试菜单" + suffix,
		"type":       "menu",
		"icon":       "menu",
		"path":       "/system/it-" + suffix,
		"component":  "system/it/index",
		"permission": "system:it:list:" + suffix,
		"sort":       90,
		"visible":    1,
		"status":     1,
	})
	if menuCreate.Status != 200 {
		t.Fatalf("menu create status %d: %s", menuCreate.Status, menuCreate.Raw)
	}
	menuID := itoa(idOf(t, menuCreate))

	menuUpdate := put(t, "/api/system/menus/"+menuID, token, map[string]any{
		"name":    "集成测试菜单已更新" + suffix,
		"visible": 0,
		"status":  0,
	})
	if menuUpdate.Status != 200 {
		t.Fatalf("menu update status %d: %s", menuUpdate.Status, menuUpdate.Raw)
	}
	if r := del(t, "/api/system/menus/"+menuID, token); r.Status != 200 {
		t.Fatalf("menu delete status %d: %s", r.Status, r.Raw)
	}
}

func TestDictionaryAndConfigCrudPersistAndDeleteRecords(t *testing.T) {
	suffix := uniqueSuffix()
	token := loginAsAdmin(t)
	dictType := "it_dict_" + suffix

	dictTypeCreate := post(t, "/api/system/dict/types", token, map[string]any{
		"dictName": "集成测试字典" + suffix,
		"dictType": dictType,
		"status":   1,
		"remark":   "集成测试创建",
	})
	if dictTypeCreate.Status != 200 {
		t.Fatalf("dict type create status %d: %s", dictTypeCreate.Status, dictTypeCreate.Raw)
	}
	dictTypeID := itoa(idOf(t, dictTypeCreate))

	dictDataCreate := post(t, "/api/system/dict/data", token, map[string]any{
		"dictType": dictType,
		"label":    "选项A",
		"value":    "A",
		"sort":     1,
		"status":   1,
		"tone":     "ok",
		"def":      true,
	})
	if dictDataCreate.Status != 200 {
		t.Fatalf("dict data create status %d: %s", dictDataCreate.Status, dictDataCreate.Raw)
	}
	dictDataID := itoa(idOf(t, dictDataCreate))

	dictData := get(t, "/api/system/dict/data/"+dictType, token)
	if labels := pluck(okList(t, dictData), "label"); !containsString(labels, "选项A") {
		t.Errorf("dict labels %v lack 选项A", labels)
	}

	if r := del(t, "/api/system/dict/data/"+dictDataID, token); r.Status != http.StatusOK {
		t.Fatalf("dict data delete status %d: %s", r.Status, r.Raw)
	}
	if r := del(t, "/api/system/dict/types/"+dictTypeID, token); r.Status != http.StatusOK {
		t.Fatalf("dict type delete status %d: %s", r.Status, r.Raw)
	}

	configCreate := post(t, "/api/system/configs", token, map[string]any{
		"name":    "集成测试参数" + suffix,
		"key":     "it.config." + suffix,
		"value":   "enabled",
		"type":    "N",
		"builtin": false,
		"remark":  "集成测试创建",
	})
	if configCreate.Status != 200 {
		t.Fatalf("config create status %d: %s", configCreate.Status, configCreate.Raw)
	}
	configID := itoa(idOf(t, configCreate))

	configUpdate := put(t, "/api/system/configs/"+configID, token, map[string]any{
		"name":   "集成测试参数已更新" + suffix,
		"value":  "disabled",
		"remark": "已更新",
	})
	if configUpdate.Status != 200 {
		t.Fatalf("config update status %d: %s", configUpdate.Status, configUpdate.Raw)
	}

	rows := okRows(t, get(t, "/api/system/configs?pageNum=1&pageSize=10&keyword="+suffix, token))
	if len(rows) == 0 {
		t.Fatalf("config list empty")
	}
	config := rows[0]
	if config["name"] != "集成测试参数已更新"+suffix {
		t.Errorf("name = %v", config["name"])
	}
	if config["value"] != "disabled" {
		t.Errorf("value = %v", config["value"])
	}

	if r := del(t, "/api/system/configs/"+configID, token); r.Status != 200 {
		t.Fatalf("config delete status %d: %s", r.Status, r.Raw)
	}
}
