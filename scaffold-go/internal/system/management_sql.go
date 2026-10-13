package system

import (
	"context"

	"scaffold/internal/framework"
)

// 对应 Java SystemManagementMapper：每条语句原样搬运，#{x} → ?，参数按出现顺序重复。

// mgmtUserFilterArgs 为 countUsers / selectUsers 共用的 WHERE 参数：keyword×5, status×2
func mgmtUserFilterArgs(keyword *string, status *int) []any {
	return []any{keyword, keyword, keyword, keyword, keyword, status, status}
}

// countUsers
func mgmtCountUsers(ctx context.Context, q framework.Querier, keyword *string, status *int) (int64, error) {
	return framework.SelectInt64(ctx, q, `
		SELECT COUNT(*)
		FROM sys_user u
		WHERE u.deleted = 0
		  AND (? IS NULL OR ? = ''
		       OR u.username LIKE CONCAT('%', ?, '%')
		       OR u.nickname LIKE CONCAT('%', ?, '%')
		       OR u.phone LIKE CONCAT('%', ?, '%'))
		  AND (? IS NULL OR u.status = ?)`,
		mgmtUserFilterArgs(keyword, status)...)
}

// selectUsers：appIdsText / appNames 来自 sys_user_app，调用方用 mgmtUserRows 把 appIdsText 拆为 appIds。
func mgmtSelectUsers(ctx context.Context, q framework.Querier, keyword *string, status *int, offset, pageSize int) ([]framework.Row, error) {
	return framework.SelectRows(ctx, q, `
		SELECT u.id,
		       u.username,
		       u.nickname,
		       u.phone,
		       u.email,
		       COALESCE(GROUP_CONCAT(DISTINCT r.role_name ORDER BY r.sort, r.id SEPARATOR ','), '') AS roleName,
		       COALESCE(GROUP_CONCAT(DISTINCT a.id ORDER BY a.id SEPARATOR ','), '') AS appIdsText,
		       COALESCE(GROUP_CONCAT(DISTINCT a.app_name ORDER BY a.id SEPARATOR ','), '') AS appNames,
		       u.status,
		       DATE_FORMAT(u.create_time, '%Y-%m-%d %H:%i') AS createTime,
		       DATE_FORMAT(u.last_login_time, '%Y-%m-%d %H:%i') AS lastLogin
		FROM sys_user u
		LEFT JOIN sys_user_role ur ON ur.user_id = u.id
		LEFT JOIN sys_role r ON r.id = ur.role_id AND r.deleted = 0
		LEFT JOIN sys_user_app ua ON ua.user_id = u.id
		LEFT JOIN app_info a ON a.id = ua.app_id AND a.deleted = 0
		WHERE u.deleted = 0
		  AND (? IS NULL OR ? = ''
		       OR u.username LIKE CONCAT('%', ?, '%')
		       OR u.nickname LIKE CONCAT('%', ?, '%')
		       OR u.phone LIKE CONCAT('%', ?, '%'))
		  AND (? IS NULL OR u.status = ?)
		GROUP BY u.id, u.username, u.nickname, u.phone, u.email, u.status, u.create_time, u.last_login_time
		ORDER BY u.id
		LIMIT ?, ?`,
		append(mgmtUserFilterArgs(keyword, status), offset, pageSize)...)
}

// selectAppOptions：users/options 与后台账号表单的可选应用；非 super_admin 只返回已绑定的应用。
func mgmtSelectAppOptions(ctx context.Context, q framework.Querier, scope appScope) ([]framework.Row, error) {
	clause, args := scope.clause("a.id")
	return framework.SelectRows(ctx, q, `
		SELECT a.id, a.app_code AS code, a.app_name AS name
		FROM app_info a
		WHERE a.deleted = 0`+clause+`
		ORDER BY a.id`, args...)
}

// countRoles
func mgmtCountRoles(ctx context.Context, q framework.Querier, keyword *string, status *int) (int64, error) {
	return framework.SelectInt64(ctx, q, `
		SELECT COUNT(*)
		FROM sys_role r
		WHERE r.deleted = 0
		  AND (? IS NULL OR ? = ''
		       OR r.role_name LIKE CONCAT('%', ?, '%')
		       OR r.role_code LIKE CONCAT('%', ?, '%'))
		  AND (? IS NULL OR r.status = ?)`,
		keyword, keyword, keyword, keyword, status, status)
}

// selectRoles
func mgmtSelectRoles(ctx context.Context, q framework.Querier, keyword *string, status *int, offset, pageSize int) ([]framework.Row, error) {
	return framework.SelectRows(ctx, q, `
		SELECT r.id,
		       r.role_name AS name,
		       r.role_code AS code,
		       (SELECT COUNT(*) FROM sys_user_role ur JOIN sys_user cu ON cu.id = ur.user_id AND cu.deleted = 0 WHERE ur.role_id = r.id) AS userCount,
		       r.sort,
		       r.status,
		       r.builtin,
		       DATE_FORMAT(r.create_time, '%Y-%m-%d %H:%i') AS createTime,
		       (SELECT GROUP_CONCAT(rm.menu_id ORDER BY rm.menu_id)
		        FROM sys_role_menu rm
		        WHERE rm.role_id = r.id) AS menuIdsText,
		       r.remark
		FROM sys_role r
		WHERE r.deleted = 0
		  AND (? IS NULL OR ? = ''
		       OR r.role_name LIKE CONCAT('%', ?, '%')
		       OR r.role_code LIKE CONCAT('%', ?, '%'))
		  AND (? IS NULL OR r.status = ?)
		ORDER BY r.sort, r.id
		LIMIT ?, ?`,
		keyword, keyword, keyword, keyword, status, status, offset, pageSize)
}

// selectMenus
func mgmtSelectMenus(ctx context.Context, q framework.Querier, keyword *string, status *int) ([]framework.Row, error) {
	return framework.SelectRows(ctx, q, `
		SELECT m.id,
		       m.parent_id AS parentId,
		       m.menu_name AS name,
		       m.menu_type AS type,
		       m.icon,
		       m.path,
		       m.component,
		       m.permission,
		       m.sort,
		       m.visible,
		       m.status
		FROM sys_menu m
		WHERE (? IS NULL OR ? = ''
		       OR m.menu_name LIKE CONCAT('%', ?, '%')
		       OR m.permission LIKE CONCAT('%', ?, '%'))
		  AND (? IS NULL OR m.status = ?)
		ORDER BY m.parent_id, m.sort, m.id`,
		keyword, keyword, keyword, keyword, status, status)
}

// countDictTypes
func mgmtCountDictTypes(ctx context.Context, q framework.Querier, keyword *string, status *int) (int64, error) {
	return framework.SelectInt64(ctx, q, `
		SELECT COUNT(*)
		FROM sys_dict_type t
		WHERE (? IS NULL OR ? = ''
		       OR t.dict_name LIKE CONCAT('%', ?, '%')
		       OR t.dict_type LIKE CONCAT('%', ?, '%'))
		  AND (? IS NULL OR t.status = ?)`,
		keyword, keyword, keyword, keyword, status, status)
}

// selectDictTypes
func mgmtSelectDictTypes(ctx context.Context, q framework.Querier, keyword *string, status *int, offset, pageSize int) ([]framework.Row, error) {
	return framework.SelectRows(ctx, q, `
		SELECT t.id,
		       t.dict_name AS dictName,
		       t.dict_type AS dictType,
		       t.status,
		       DATE_FORMAT(t.create_time, '%Y-%m-%d %H:%i') AS createTime,
		       t.remark
		FROM sys_dict_type t
		WHERE (? IS NULL OR ? = ''
		       OR t.dict_name LIKE CONCAT('%', ?, '%')
		       OR t.dict_type LIKE CONCAT('%', ?, '%'))
		  AND (? IS NULL OR t.status = ?)
		ORDER BY t.id
		LIMIT ?, ?`,
		keyword, keyword, keyword, keyword, status, status, offset, pageSize)
}

// selectDictData
func mgmtSelectDictData(ctx context.Context, q framework.Querier, dictType string) ([]framework.Row, error) {
	return framework.SelectRows(ctx, q, `
		SELECT d.id,
		       d.dict_label AS label,
		       d.dict_value AS value,
		       d.sort,
		       d.status,
		       d.tone,
		       d.is_default AS def
		FROM sys_dict_data d
		WHERE d.dict_type = ?
		ORDER BY d.sort, d.id`, dictType)
}

// selectDictOptions：业务下拉只提供启用字典的启用选项，管理接口仍按字典管理权限保护。
func mgmtSelectDictOptions(ctx context.Context, q framework.Querier, dictType string) ([]framework.Row, error) {
	return framework.SelectRows(ctx, q, `
		SELECT d.dict_label AS label, d.dict_value AS value, d.tone,
		       d.is_default AS def, d.status
		FROM sys_dict_data d
		JOIN sys_dict_type t ON t.dict_type = d.dict_type AND t.status = 1
		WHERE d.dict_type = ? AND d.status = 1
		ORDER BY d.sort, d.id`, dictType)
}

// countConfigs
func mgmtCountConfigs(ctx context.Context, q framework.Querier, keyword *string) (int64, error) {
	return framework.SelectInt64(ctx, q, `
		SELECT COUNT(*)
		FROM sys_config c
		WHERE (? IS NULL OR ? = ''
		       OR c.config_name LIKE CONCAT('%', ?, '%')
		       OR c.config_key LIKE CONCAT('%', ?, '%'))`,
		keyword, keyword, keyword, keyword)
}

// selectConfigs
func mgmtSelectConfigs(ctx context.Context, q framework.Querier, keyword *string, offset, pageSize int) ([]framework.Row, error) {
	return framework.SelectRows(ctx, q, `
		SELECT c.id,
		       c.config_name AS name,
		       c.config_key AS `+"`key`"+`,
		       c.config_value AS value,
		       c.config_type AS type,
		       c.builtin,
		       DATE_FORMAT(c.create_time, '%Y-%m-%d %H:%i') AS createTime,
		       c.remark
		FROM sys_config c
		WHERE (? IS NULL OR ? = ''
		       OR c.config_name LIKE CONCAT('%', ?, '%')
		       OR c.config_key LIKE CONCAT('%', ?, '%'))
		ORDER BY c.id
		LIMIT ?, ?`,
		keyword, keyword, keyword, keyword, offset, pageSize)
}

// countOperLogs
func mgmtCountOperLogs(ctx context.Context, q framework.Querier, keyword *string, result *int) (int64, error) {
	return framework.SelectInt64(ctx, q, `
		SELECT COUNT(*)
		FROM sys_oper_log l
		WHERE (? IS NULL OR ? = ''
		       OR l.module LIKE CONCAT('%', ?, '%')
		       OR l.action LIKE CONCAT('%', ?, '%')
		       OR l.oper_name LIKE CONCAT('%', ?, '%')
		       OR l.request_url LIKE CONCAT('%', ?, '%')
		       OR l.request_params LIKE CONCAT('%', ?, '%'))
		  AND (? IS NULL OR l.result = ?)`,
		keyword, keyword, keyword, keyword, keyword, keyword, keyword, result, result)
}

// selectOperLogs
func mgmtSelectOperLogs(ctx context.Context, q framework.Querier, keyword *string, result *int, offset, pageSize int) ([]framework.Row, error) {
	return framework.SelectRows(ctx, q, `
		SELECT l.id,
		       l.module,
		       l.action,
		       l.request_method AS method,
		       l.request_url AS url,
		       l.request_params AS params,
		       l.oper_name AS operName,
		       l.ip,
		       l.location,
		       l.cost,
		       l.result,
		       l.error_msg AS errorMsg,
		       DATE_FORMAT(l.oper_time, '%Y-%m-%d %H:%i:%s') AS operTime
		FROM sys_oper_log l
		WHERE (? IS NULL OR ? = ''
		       OR l.module LIKE CONCAT('%', ?, '%')
		       OR l.action LIKE CONCAT('%', ?, '%')
		       OR l.oper_name LIKE CONCAT('%', ?, '%')
		       OR l.request_url LIKE CONCAT('%', ?, '%')
		       OR l.request_params LIKE CONCAT('%', ?, '%'))
		  AND (? IS NULL OR l.result = ?)
		ORDER BY l.oper_time DESC, l.id DESC
		LIMIT ?, ?`,
		keyword, keyword, keyword, keyword, keyword, keyword, keyword, result, result, offset, pageSize)
}

// countLoginLogs
func mgmtCountLoginLogs(ctx context.Context, q framework.Querier, keyword *string, status *int) (int64, error) {
	return framework.SelectInt64(ctx, q, `
		SELECT COUNT(*)
		FROM sys_login_log l
		WHERE (? IS NULL OR ? = ''
		       OR l.username LIKE CONCAT('%', ?, '%')
		       OR l.ip LIKE CONCAT('%', ?, '%')
		       OR l.msg LIKE CONCAT('%', ?, '%'))
		  AND (? IS NULL OR l.status = ?)`,
		keyword, keyword, keyword, keyword, keyword, status, status)
}

// selectLoginLogs
func mgmtSelectLoginLogs(ctx context.Context, q framework.Querier, keyword *string, status *int, offset, pageSize int) ([]framework.Row, error) {
	return framework.SelectRows(ctx, q, `
		SELECT l.id,
		       l.username,
		       l.ip,
		       l.location,
		       l.browser,
		       l.os,
		       l.status,
		       l.msg,
		       DATE_FORMAT(l.login_time, '%Y-%m-%d %H:%i:%s') AS loginTime
		FROM sys_login_log l
		WHERE (? IS NULL OR ? = ''
		       OR l.username LIKE CONCAT('%', ?, '%')
		       OR l.ip LIKE CONCAT('%', ?, '%')
		       OR l.msg LIKE CONCAT('%', ?, '%'))
		  AND (? IS NULL OR l.status = ?)
		ORDER BY l.login_time DESC, l.id DESC
		LIMIT ?, ?`,
		keyword, keyword, keyword, keyword, keyword, status, status, offset, pageSize)
}

// deleteOperLog
func mgmtDeleteOperLog(ctx context.Context, q framework.Querier, id int64) (int64, error) {
	return framework.Exec(ctx, q, `DELETE FROM sys_oper_log WHERE id = ?`, id)
}

// clearOperLogs
func mgmtClearOperLogs(ctx context.Context, q framework.Querier) (int64, error) {
	return framework.Exec(ctx, q, `DELETE FROM sys_oper_log`)
}

// deleteLoginLog
func mgmtDeleteLoginLog(ctx context.Context, q framework.Querier, id int64) (int64, error) {
	return framework.Exec(ctx, q, `DELETE FROM sys_login_log WHERE id = ?`, id)
}

// clearLoginLogs
func mgmtClearLoginLogs(ctx context.Context, q framework.Querier) (int64, error) {
	return framework.Exec(ctx, q, `DELETE FROM sys_login_log`)
}

// countUsername
func mgmtCountUsername(ctx context.Context, q framework.Querier, username string, excludeID *int64) (int64, error) {
	return framework.SelectInt64(ctx, q, `
		SELECT COUNT(*)
		FROM sys_user
		WHERE username = ?
		  AND deleted = 0
		  AND (? IS NULL OR id <> ?)`, username, excludeID, excludeID)
}

// insertUser
func mgmtInsertUser(ctx context.Context, q framework.Querier, username, password, nickname string, phone, email *string, status int, operator string) (int64, error) {
	return framework.Insert(ctx, q, `
		INSERT INTO sys_user (username, password, nickname, phone, email, status, create_by, update_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, username, password, nickname, phone, email, status, operator, operator)
}

// updateUser
func mgmtUpdateUser(ctx context.Context, q framework.Querier, id int64, nickname, phone, email *string, status *int, operator string) (int64, error) {
	return framework.Exec(ctx, q, `
		UPDATE sys_user
		SET nickname = COALESCE(?, nickname),
		    phone = COALESCE(?, phone),
		    email = COALESCE(?, email),
		    status = COALESCE(?, status),
		    update_by = ?
		WHERE id = ? AND deleted = 0`, nickname, phone, email, status, operator, id)
}

// updateUserStatus
func mgmtUpdateUserStatus(ctx context.Context, q framework.Querier, id int64, status int, operator string) (int64, error) {
	return framework.Exec(ctx, q, `
		UPDATE sys_user
		SET status = ?, update_by = ?
		WHERE id = ? AND deleted = 0`, status, operator, id)
}

// resetUserPassword
func mgmtResetUserPassword(ctx context.Context, q framework.Querier, id int64, password, operator string) (int64, error) {
	return framework.Exec(ctx, q, `
		UPDATE sys_user
		SET password = ?, update_by = ?
		WHERE id = ? AND deleted = 0`, password, operator, id)
}

// softDeleteUser
func mgmtSoftDeleteUser(ctx context.Context, q framework.Querier, id int64, operator string) (int64, error) {
	return framework.Exec(ctx, q, `
		UPDATE sys_user
		SET deleted = 1, update_by = ?
		WHERE id = ? AND deleted = 0`, operator, id)
}

// deleteUserRoles
func mgmtDeleteUserRoles(ctx context.Context, q framework.Querier, userID int64) error {
	_, err := framework.Exec(ctx, q, `DELETE FROM sys_user_role WHERE user_id = ?`, userID)
	return err
}

// insertUserRole
func mgmtInsertUserRole(ctx context.Context, q framework.Querier, userID, roleID int64) error {
	_, err := framework.Exec(ctx, q, `INSERT IGNORE INTO sys_user_role (user_id, role_id) VALUES (?, ?)`, userID, roleID)
	return err
}

// deleteUserApps
func mgmtDeleteUserApps(ctx context.Context, q framework.Querier, userID int64) error {
	_, err := framework.Exec(ctx, q, `DELETE FROM sys_user_app WHERE user_id = ?`, userID)
	return err
}

// insertUserApp
func mgmtInsertUserApp(ctx context.Context, q framework.Querier, userID, appID int64) error {
	_, err := framework.Exec(ctx, q, `INSERT IGNORE INTO sys_user_app (user_id, app_id) VALUES (?, ?)`, userID, appID)
	return err
}

// countRoleCode
func mgmtCountRoleCode(ctx context.Context, q framework.Querier, code string, excludeID *int64) (int64, error) {
	return framework.SelectInt64(ctx, q, `
		SELECT COUNT(*)
		FROM sys_role
		WHERE role_code = ?
		  AND deleted = 0
		  AND (? IS NULL OR id <> ?)`, code, excludeID, excludeID)
}

// insertRole
func mgmtInsertRole(ctx context.Context, q framework.Querier, name, code string, sort, status int, remark *string, operator string) (int64, error) {
	return framework.Insert(ctx, q, `
		INSERT INTO sys_role (role_name, role_code, sort, status, builtin, remark, create_by, update_by)
		VALUES (?, ?, ?, ?, 0, ?, ?, ?)`, name, code, sort, status, remark, operator, operator)
}

// updateRole
func mgmtUpdateRole(ctx context.Context, q framework.Querier, id int64, name *string, sort, status *int, remark *string, operator string) (int64, error) {
	return framework.Exec(ctx, q, `
		UPDATE sys_role
		SET role_name = COALESCE(?, role_name),
		    sort = COALESCE(?, sort),
		    status = COALESCE(?, status),
		    remark = COALESCE(?, remark),
		    update_by = ?
		WHERE id = ? AND deleted = 0 AND builtin = 0`, name, sort, status, remark, operator, id)
}

// softDeleteRole
func mgmtSoftDeleteRole(ctx context.Context, q framework.Querier, id int64, operator string) (int64, error) {
	return framework.Exec(ctx, q, `
		UPDATE sys_role
		SET deleted = 1, update_by = ?
		WHERE id = ? AND deleted = 0 AND builtin = 0`, operator, id)
}

// deleteRoleMenus
func mgmtDeleteRoleMenus(ctx context.Context, q framework.Querier, roleID int64) error {
	_, err := framework.Exec(ctx, q, `DELETE FROM sys_role_menu WHERE role_id = ?`, roleID)
	return err
}

// insertRoleMenu
func mgmtInsertRoleMenu(ctx context.Context, q framework.Querier, roleID, menuID int64) error {
	_, err := framework.Exec(ctx, q, `INSERT IGNORE INTO sys_role_menu (role_id, menu_id) VALUES (?, ?)`, roleID, menuID)
	return err
}

// selectRoleMenuIds
func mgmtSelectRoleMenuIDs(ctx context.Context, q framework.Querier, roleID int64) ([]int64, error) {
	return framework.SelectInt64s(ctx, q, `SELECT menu_id FROM sys_role_menu WHERE role_id = ? ORDER BY menu_id`, roleID)
}

// selectGraphUsers：权限关系图的用户节点，只取图谱展示所需字段。
func mgmtSelectGraphUsers(ctx context.Context, q framework.Querier) ([]framework.Row, error) {
	return framework.SelectRows(ctx, q, `
		SELECT u.id, u.username, u.nickname, u.status
		FROM sys_user u
		WHERE u.deleted = 0
		ORDER BY u.id
		LIMIT 2000`)
}

// selectUserRoleLinks
func mgmtSelectUserRoleLinks(ctx context.Context, q framework.Querier) ([]framework.Row, error) {
	return framework.SelectRows(ctx, q, `
		SELECT ur.user_id AS userId, ur.role_id AS roleId
		FROM sys_user_role ur
		JOIN sys_user u ON u.id = ur.user_id AND u.deleted = 0
		JOIN sys_role r ON r.id = ur.role_id AND r.deleted = 0
		ORDER BY ur.user_id, ur.role_id`)
}

// countRoleUsers
func mgmtCountRoleUsers(ctx context.Context, q framework.Querier, roleID int64) (int64, error) {
	return framework.SelectInt64(ctx, q, `
		SELECT COUNT(*)
		FROM sys_user_role ur
		JOIN sys_user u ON u.id = ur.user_id
		WHERE ur.role_id = ? AND u.deleted = 0`, roleID)
}

// nextMenuId
func mgmtNextMenuID(ctx context.Context, q framework.Querier) (int64, error) {
	return framework.SelectInt64(ctx, q, `SELECT COALESCE(MAX(id), 0) + 1 FROM sys_menu`)
}

// insertMenu
func mgmtInsertMenu(ctx context.Context, q framework.Querier, id, parentID int64, name, menuType string, icon, path, component, permission *string, sort, visible, status int, operator string) error {
	_, err := framework.Exec(ctx, q, `
		INSERT INTO sys_menu (id, parent_id, menu_name, menu_type, icon, path, component, permission, sort, visible, status, create_by, update_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, parentID, name, menuType, icon, path, component, permission, sort, visible, status, operator, operator)
	return err
}

// updateMenu
func mgmtUpdateMenu(ctx context.Context, q framework.Querier, id int64, parentID *int64, name, menuType, icon, path, component, permission *string, sort, visible, status *int, operator string) (int64, error) {
	return framework.Exec(ctx, q, `
		UPDATE sys_menu
		SET parent_id = COALESCE(?, parent_id),
		    menu_name = COALESCE(?, menu_name),
		    menu_type = COALESCE(?, menu_type),
		    icon = COALESCE(?, icon),
		    path = COALESCE(?, path),
		    component = COALESCE(?, component),
		    permission = COALESCE(?, permission),
		    sort = COALESCE(?, sort),
		    visible = COALESCE(?, visible),
		    status = COALESCE(?, status),
		    update_by = ?
		WHERE id = ?`, parentID, name, menuType, icon, path, component, permission, sort, visible, status, operator, id)
}

// deleteMenu
func mgmtDeleteMenu(ctx context.Context, q framework.Querier, id int64) (int64, error) {
	return framework.Exec(ctx, q, `DELETE FROM sys_menu WHERE id = ?`, id)
}

// countMenuChildren
func mgmtCountMenuChildren(ctx context.Context, q framework.Querier, menuID int64) (int64, error) {
	return framework.SelectInt64(ctx, q, `SELECT COUNT(*) FROM sys_menu WHERE parent_id = ?`, menuID)
}

// countMenuRoles
func mgmtCountMenuRoles(ctx context.Context, q framework.Querier, menuID int64) (int64, error) {
	return framework.SelectInt64(ctx, q, `SELECT COUNT(*) FROM sys_role_menu WHERE menu_id = ?`, menuID)
}

// countDictType
func mgmtCountDictType(ctx context.Context, q framework.Querier, dictType string, excludeID *int64) (int64, error) {
	return framework.SelectInt64(ctx, q, `
		SELECT COUNT(*)
		FROM sys_dict_type
		WHERE dict_type = ?
		  AND (? IS NULL OR id <> ?)`, dictType, excludeID, excludeID)
}

// insertDictType
func mgmtInsertDictType(ctx context.Context, q framework.Querier, dictName, dictType string, status int, remark *string, operator string) (int64, error) {
	return framework.Insert(ctx, q, `
		INSERT INTO sys_dict_type (dict_name, dict_type, status, remark, create_by, update_by)
		VALUES (?, ?, ?, ?, ?, ?)`, dictName, dictType, status, remark, operator, operator)
}

// updateDictType
func mgmtUpdateDictType(ctx context.Context, q framework.Querier, id int64, dictName *string, status *int, remark *string, operator string) (int64, error) {
	return framework.Exec(ctx, q, `
		UPDATE sys_dict_type
		SET dict_name = COALESCE(?, dict_name),
		    status = COALESCE(?, status),
		    remark = COALESCE(?, remark),
		    update_by = ?
		WHERE id = ?`, dictName, status, remark, operator, id)
}

// deleteDictType
func mgmtDeleteDictType(ctx context.Context, q framework.Querier, id int64) (int64, error) {
	return framework.Exec(ctx, q, `DELETE FROM sys_dict_type WHERE id = ?`, id)
}

// countDictDataByType
func mgmtCountDictDataByType(ctx context.Context, q framework.Querier, dictType string) (int64, error) {
	return framework.SelectInt64(ctx, q, `SELECT COUNT(*) FROM sys_dict_data WHERE dict_type = ?`, dictType)
}

// selectDictTypeCode：无行或列为 NULL 均返回 nil。
func mgmtSelectDictTypeCode(ctx context.Context, q framework.Querier, id int64) (*string, error) {
	row, err := framework.SelectRow(ctx, q, `SELECT dict_type FROM sys_dict_type WHERE id = ?`, id)
	if err != nil || row == nil {
		return nil, err
	}
	return strPtr(row["dict_type"]), nil
}

// insertDictData
func mgmtInsertDictData(ctx context.Context, q framework.Querier, dictType, label, value string, sort, status int, tone *string, def int, remark *string, operator string) (int64, error) {
	return framework.Insert(ctx, q, `
		INSERT INTO sys_dict_data (dict_type, dict_label, dict_value, sort, status, tone, is_default, remark, create_by, update_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, dictType, label, value, sort, status, tone, def, remark, operator, operator)
}

// updateDictData
func mgmtUpdateDictData(ctx context.Context, q framework.Querier, id int64, dictType, label, value *string, sort, status *int, tone *string, def *int, remark *string, operator string) (int64, error) {
	return framework.Exec(ctx, q, `
		UPDATE sys_dict_data
		SET dict_type = COALESCE(?, dict_type),
		    dict_label = COALESCE(?, dict_label),
		    dict_value = COALESCE(?, dict_value),
		    sort = COALESCE(?, sort),
		    status = COALESCE(?, status),
		    tone = COALESCE(?, tone),
		    is_default = COALESCE(?, is_default),
		    remark = COALESCE(?, remark),
		    update_by = ?
		WHERE id = ?`, dictType, label, value, sort, status, tone, def, remark, operator, id)
}

// deleteDictData
func mgmtDeleteDictData(ctx context.Context, q framework.Querier, id int64) (int64, error) {
	return framework.Exec(ctx, q, `DELETE FROM sys_dict_data WHERE id = ?`, id)
}

// countConfigKey
func mgmtCountConfigKey(ctx context.Context, q framework.Querier, key string, excludeID *int64) (int64, error) {
	return framework.SelectInt64(ctx, q, `
		SELECT COUNT(*)
		FROM sys_config
		WHERE config_key = ?
		  AND (? IS NULL OR id <> ?)`, key, excludeID, excludeID)
}

// insertConfig
func mgmtInsertConfig(ctx context.Context, q framework.Querier, name, key, value, configType string, builtin int, remark *string, operator string) (int64, error) {
	return framework.Insert(ctx, q, `
		INSERT INTO sys_config (config_name, config_key, config_value, config_type, builtin, remark, create_by, update_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, name, key, value, configType, builtin, remark, operator, operator)
}

// updateConfig
func mgmtUpdateConfig(ctx context.Context, q framework.Querier, id int64, name, value, configType *string, builtin *int, remark *string, operator string) (int64, error) {
	return framework.Exec(ctx, q, `
		UPDATE sys_config
		SET config_name = COALESCE(?, config_name),
		    config_value = COALESCE(?, config_value),
		    config_type = COALESCE(?, config_type),
		    builtin = COALESCE(?, builtin),
		    remark = COALESCE(?, remark),
		    update_by = ?
		WHERE id = ? AND builtin = 0`, name, value, configType, builtin, remark, operator, id)
}

// deleteConfig
func mgmtDeleteConfig(ctx context.Context, q framework.Querier, id int64) (int64, error) {
	return framework.Exec(ctx, q, `DELETE FROM sys_config WHERE id = ? AND builtin = 0`, id)
}
