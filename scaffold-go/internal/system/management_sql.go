package system

import (
	"context"

	"scaffold/internal/framework"
)

// 对应 Java SystemManagementMapper：每条语句原样搬运，#{x} → ?，参数按出现顺序重复。
// selectUserProfileByUsername / selectActiveRoleScopes / selectDeptAndChildIds / selectRoleDeptIds 已在 datascope.go。

// mgmtScopeClause 对应 <script> 中的数据范围片段：
// #{scope.allScope} = TRUE OR u.id = #{scope.userId} [OR u.dept_id IN (...)]
func mgmtScopeClause(scope framework.DataScopeCriteria) (string, []any) {
	clause := "? = TRUE OR u.id = ?"
	args := []any{scope.AllScope, scope.UserID}
	if len(scope.DeptIDs) > 0 {
		clause += " OR u.dept_id IN (" + framework.In(len(scope.DeptIDs)) + ")"
		args = append(args, framework.Int64Args(scope.DeptIDs)...)
	}
	return clause, args
}

// mgmtUserFilterArgs 为 countUsers / selectUsers 共用的 WHERE 参数：keyword×5, status×2, deptId×3, postId×2, scope...
func mgmtUserFilterArgs(keyword *string, status *int, deptID, postID *int64, scopeArgs []any) []any {
	args := []any{keyword, keyword, keyword, keyword, keyword, status, status, deptID, deptID, deptID, postID, postID}
	return append(args, scopeArgs...)
}

// countUsers
func mgmtCountUsers(ctx context.Context, q framework.Querier, keyword *string, status *int, deptID, postID *int64, scope framework.DataScopeCriteria) (int64, error) {
	clause, scopeArgs := mgmtScopeClause(scope)
	return framework.SelectInt64(ctx, q, `
		SELECT COUNT(DISTINCT u.id)
		FROM sys_user u
		LEFT JOIN sys_dept d ON d.id = u.dept_id
		WHERE u.deleted = 0
		  AND (? IS NULL OR ? = ''
		       OR u.username LIKE CONCAT('%', ?, '%')
		       OR u.nickname LIKE CONCAT('%', ?, '%')
		       OR u.phone LIKE CONCAT('%', ?, '%'))
		  AND (? IS NULL OR u.status = ?)
		  AND (? IS NULL OR u.dept_id IN (SELECT sd.id FROM sys_dept sd WHERE sd.id = ? OR FIND_IN_SET(?, sd.ancestors)))
		  AND (? IS NULL OR EXISTS (SELECT 1 FROM sys_user_post fup WHERE fup.user_id = u.id AND fup.post_id = ?))
		  AND (`+clause+`)`,
		mgmtUserFilterArgs(keyword, status, deptID, postID, scopeArgs)...)
}

// selectUsers
func mgmtSelectUsers(ctx context.Context, q framework.Querier, keyword *string, status *int, deptID, postID *int64, scope framework.DataScopeCriteria, offset, pageSize int) ([]framework.Row, error) {
	clause, scopeArgs := mgmtScopeClause(scope)
	return framework.SelectRows(ctx, q, `
		SELECT u.id,
		       u.username,
		       u.nickname,
		       u.phone,
		       u.email,
		       d.dept_name AS deptName,
		       COALESCE(GROUP_CONCAT(DISTINCT p.post_name ORDER BY p.sort, p.id SEPARATOR ','), '') AS postName,
		       COALESCE(GROUP_CONCAT(DISTINCT r.role_name ORDER BY r.sort, r.id SEPARATOR ','), '') AS roleName,
		       u.status,
		       DATE_FORMAT(u.create_time, '%Y-%m-%d %H:%i') AS createTime,
		       DATE_FORMAT(u.last_login_time, '%Y-%m-%d %H:%i') AS lastLogin
		FROM sys_user u
		LEFT JOIN sys_dept d ON d.id = u.dept_id
		LEFT JOIN sys_user_post up ON up.user_id = u.id
		LEFT JOIN sys_post p ON p.id = up.post_id AND p.deleted = 0
		LEFT JOIN sys_user_role ur ON ur.user_id = u.id
		LEFT JOIN sys_role r ON r.id = ur.role_id AND r.deleted = 0
		WHERE u.deleted = 0
		  AND (? IS NULL OR ? = ''
		       OR u.username LIKE CONCAT('%', ?, '%')
		       OR u.nickname LIKE CONCAT('%', ?, '%')
		       OR u.phone LIKE CONCAT('%', ?, '%'))
		  AND (? IS NULL OR u.status = ?)
		  AND (? IS NULL OR u.dept_id IN (SELECT sd.id FROM sys_dept sd WHERE sd.id = ? OR FIND_IN_SET(?, sd.ancestors)))
		  AND (? IS NULL OR EXISTS (SELECT 1 FROM sys_user_post fup WHERE fup.user_id = u.id AND fup.post_id = ?))
		  AND (`+clause+`)
		GROUP BY u.id, u.username, u.nickname, u.phone, u.email, d.dept_name, u.status, u.create_time, u.last_login_time
		ORDER BY u.id
		LIMIT ?, ?`,
		append(mgmtUserFilterArgs(keyword, status, deptID, postID, scopeArgs), offset, pageSize)...)
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
		       CASE r.data_scope
		           WHEN 'ALL' THEN '全部数据权限'
		           WHEN 'DEPT_AND_CHILD' THEN '本部门及以下'
		           WHEN 'DEPT' THEN '本部门数据权限'
		           WHEN 'CUSTOM' THEN '自定义数据权限'
		           ELSE '仅本人数据权限'
		       END AS dataScope,
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

// selectDepts
func mgmtSelectDepts(ctx context.Context, q framework.Querier, keyword *string, status *int) ([]framework.Row, error) {
	return framework.SelectRows(ctx, q, `
		SELECT d.id,
		       d.parent_id AS parentId,
		       d.dept_name AS name,
		       d.leader,
		       d.phone,
		       d.email,
		       d.sort,
		       d.status,
		       (SELECT COUNT(*) FROM sys_user u WHERE u.dept_id = d.id AND u.deleted = 0) AS memberCount
		FROM sys_dept d
		WHERE d.deleted = 0
		  AND (? IS NULL OR ? = ''
		       OR d.dept_name LIKE CONCAT('%', ?, '%')
		       OR d.leader LIKE CONCAT('%', ?, '%'))
		  AND (? IS NULL OR d.status = ?)
		ORDER BY d.parent_id, d.sort, d.id`,
		keyword, keyword, keyword, keyword, status, status)
}

// countPosts
func mgmtCountPosts(ctx context.Context, q framework.Querier, keyword *string, status *int) (int64, error) {
	return framework.SelectInt64(ctx, q, `
		SELECT COUNT(*)
		FROM sys_post p
		WHERE p.deleted = 0
		  AND (? IS NULL OR ? = ''
		       OR p.post_name LIKE CONCAT('%', ?, '%')
		       OR p.post_code LIKE CONCAT('%', ?, '%'))
		  AND (? IS NULL OR p.status = ?)`,
		keyword, keyword, keyword, keyword, status, status)
}

// selectPosts
func mgmtSelectPosts(ctx context.Context, q framework.Querier, keyword *string, status *int, offset, pageSize int) ([]framework.Row, error) {
	return framework.SelectRows(ctx, q, `
		SELECT p.id,
		       p.post_code AS postCode,
		       p.post_name AS postName,
		       p.sort,
		       p.status,
		       p.remark,
		       DATE_FORMAT(p.create_time, '%Y-%m-%d %H:%i') AS createTime,
		       (SELECT COUNT(*) FROM sys_user_post up JOIN sys_user cu ON cu.id = up.user_id AND cu.deleted = 0 WHERE up.post_id = p.id) AS userCount
		FROM sys_post p
		WHERE p.deleted = 0
		  AND (? IS NULL OR ? = ''
		       OR p.post_name LIKE CONCAT('%', ?, '%')
		       OR p.post_code LIKE CONCAT('%', ?, '%'))
		  AND (? IS NULL OR p.status = ?)
		ORDER BY p.sort, p.id
		LIMIT ?, ?`,
		keyword, keyword, keyword, keyword, status, status, offset, pageSize)
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
		       l.dept_name AS dept,
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

// countPostCode
func mgmtCountPostCode(ctx context.Context, q framework.Querier, postCode string, excludeID *int64) (int64, error) {
	return framework.SelectInt64(ctx, q, `
		SELECT COUNT(*)
		FROM sys_post
		WHERE post_code = ?
		  AND deleted = 0
		  AND (? IS NULL OR id <> ?)`, postCode, excludeID, excludeID)
}

// insertPost
func mgmtInsertPost(ctx context.Context, q framework.Querier, postCode, postName string, sort, status int, remark *string, operator string) (int64, error) {
	return framework.Insert(ctx, q, `
		INSERT INTO sys_post (post_code, post_name, sort, status, remark, create_by, update_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, postCode, postName, sort, status, remark, operator, operator)
}

// updatePost
func mgmtUpdatePost(ctx context.Context, q framework.Querier, id int64, postName *string, sort, status *int, remark *string, operator string) (int64, error) {
	return framework.Exec(ctx, q, `
		UPDATE sys_post
		SET post_name = COALESCE(?, post_name),
		    sort = COALESCE(?, sort),
		    status = COALESCE(?, status),
		    remark = COALESCE(?, remark),
		    update_by = ?
		WHERE id = ? AND deleted = 0`, postName, sort, status, remark, operator, id)
}

// softDeletePost
func mgmtSoftDeletePost(ctx context.Context, q framework.Querier, id int64, operator string) (int64, error) {
	return framework.Exec(ctx, q, `
		UPDATE sys_post
		SET deleted = 1, update_by = ?
		WHERE id = ? AND deleted = 0`, operator, id)
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
func mgmtInsertUser(ctx context.Context, q framework.Querier, username, password, nickname string, phone, email *string, deptID *int64, status int, operator string) (int64, error) {
	return framework.Insert(ctx, q, `
		INSERT INTO sys_user (username, password, nickname, phone, email, dept_id, status, create_by, update_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, username, password, nickname, phone, email, deptID, status, operator, operator)
}

// updateUser
func mgmtUpdateUser(ctx context.Context, q framework.Querier, id int64, nickname, phone, email *string, deptID *int64, status *int, operator string) (int64, error) {
	return framework.Exec(ctx, q, `
		UPDATE sys_user
		SET nickname = COALESCE(?, nickname),
		    phone = COALESCE(?, phone),
		    email = COALESCE(?, email),
		    dept_id = COALESCE(?, dept_id),
		    status = COALESCE(?, status),
		    update_by = ?
		WHERE id = ? AND deleted = 0`, nickname, phone, email, deptID, status, operator, id)
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

// deleteUserPosts
func mgmtDeleteUserPosts(ctx context.Context, q framework.Querier, userID int64) error {
	_, err := framework.Exec(ctx, q, `DELETE FROM sys_user_post WHERE user_id = ?`, userID)
	return err
}

// insertUserPost
func mgmtInsertUserPost(ctx context.Context, q framework.Querier, userID, postID int64) error {
	_, err := framework.Exec(ctx, q, `INSERT IGNORE INTO sys_user_post (user_id, post_id) VALUES (?, ?)`, userID, postID)
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
func mgmtInsertRole(ctx context.Context, q framework.Querier, name, code, dataScope string, sort, status int, remark *string, operator string) (int64, error) {
	return framework.Insert(ctx, q, `
		INSERT INTO sys_role (role_name, role_code, data_scope, sort, status, builtin, remark, create_by, update_by)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?)`, name, code, dataScope, sort, status, remark, operator, operator)
}

// updateRole
func mgmtUpdateRole(ctx context.Context, q framework.Querier, id int64, name, dataScope *string, sort, status *int, remark *string, operator string) (int64, error) {
	return framework.Exec(ctx, q, `
		UPDATE sys_role
		SET role_name = COALESCE(?, role_name),
		    data_scope = COALESCE(?, data_scope),
		    sort = COALESCE(?, sort),
		    status = COALESCE(?, status),
		    remark = COALESCE(?, remark),
		    update_by = ?
		WHERE id = ? AND deleted = 0 AND builtin = 0`, name, dataScope, sort, status, remark, operator, id)
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

// selectGraphUsers：权限关系图的用户节点，遵守数据权限，只取图谱展示所需字段。
func mgmtSelectGraphUsers(ctx context.Context, q framework.Querier, scope framework.DataScopeCriteria) ([]framework.Row, error) {
	clause, args := mgmtScopeClause(scope)
	return framework.SelectRows(ctx, q, `
		SELECT u.id, u.username, u.nickname, d.dept_name AS deptName, u.status
		FROM sys_user u
		LEFT JOIN sys_dept d ON d.id = u.dept_id
		WHERE u.deleted = 0
		  AND (`+clause+`)
		ORDER BY u.id
		LIMIT 2000`, args...)
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

// selectDeptAncestors：无行或列为 NULL 均返回 nil（对应 Java 返回 null）。
func mgmtSelectDeptAncestors(ctx context.Context, q framework.Querier, id int64) (*string, error) {
	row, err := framework.SelectRow(ctx, q, `
		SELECT ancestors
		FROM sys_dept
		WHERE id = ? AND deleted = 0`, id)
	if err != nil || row == nil {
		return nil, err
	}
	return strPtr(row["ancestors"]), nil
}

// insertDept
func mgmtInsertDept(ctx context.Context, q framework.Querier, parentID int64, ancestors, name string, leader, phone, email *string, sort, status int, operator string) (int64, error) {
	return framework.Insert(ctx, q, `
		INSERT INTO sys_dept (parent_id, ancestors, dept_name, leader, phone, email, sort, status, create_by, update_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, parentID, ancestors, name, leader, phone, email, sort, status, operator, operator)
}

// updateDept
func mgmtUpdateDept(ctx context.Context, q framework.Querier, id int64, parentID *int64, ancestors, name, leader, phone, email *string, sort, status *int, operator string) (int64, error) {
	return framework.Exec(ctx, q, `
		UPDATE sys_dept
		SET parent_id = COALESCE(?, parent_id),
		    ancestors = COALESCE(?, ancestors),
		    dept_name = COALESCE(?, dept_name),
		    leader = COALESCE(?, leader),
		    phone = COALESCE(?, phone),
		    email = COALESCE(?, email),
		    sort = COALESCE(?, sort),
		    status = COALESCE(?, status),
		    update_by = ?
		WHERE id = ? AND deleted = 0`, parentID, ancestors, name, leader, phone, email, sort, status, operator, id)
}

// softDeleteDept
func mgmtSoftDeleteDept(ctx context.Context, q framework.Querier, id int64, operator string) (int64, error) {
	return framework.Exec(ctx, q, `
		UPDATE sys_dept
		SET deleted = 1, update_by = ?
		WHERE id = ? AND deleted = 0`, operator, id)
}

// countDeptChildren
func mgmtCountDeptChildren(ctx context.Context, q framework.Querier, deptID int64) (int64, error) {
	return framework.SelectInt64(ctx, q, `SELECT COUNT(*) FROM sys_dept WHERE parent_id = ? AND deleted = 0`, deptID)
}

// countDeptUsers
func mgmtCountDeptUsers(ctx context.Context, q framework.Querier, deptID int64) (int64, error) {
	return framework.SelectInt64(ctx, q, `SELECT COUNT(*) FROM sys_user WHERE dept_id = ? AND deleted = 0`, deptID)
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
