package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"maps"
	"net/http"
	"strings"
	"unicode/utf16"

	"scaffold/internal/framework"
)

// ManagementService 对应 Java SystemManagementService。
type ManagementService struct {
	db   *sql.DB
	perm *PermissionResolver
}

func NewManagementService(d Deps) *ManagementService {
	return &ManagementService{db: d.DB, perm: d.Perm}
}

type mgmtPage = framework.PageResult[framework.Row]

// UserOptions 后台账号页所需的引用选项，避免为只读用户申请角色和参数管理权限。
// apps 为当前账号可见的应用（super_admin 全部，其余为 sys_user_app 绑定）。
func (s *ManagementService) UserOptions(ctx context.Context) (map[string]any, error) {
	username := mgmtCurrentUsername(ctx)
	canEdit := s.perm.HasPermission(ctx, username, "system:user:add") ||
		s.perm.HasPermission(ctx, username, "system:user:edit")
	canSetPassword := s.perm.HasPermission(ctx, username, "system:user:add") ||
		s.perm.HasPermission(ctx, username, "system:user:resetPwd")
	scope, err := selectAppScope(ctx, s.db, username)
	if err != nil {
		return nil, err
	}
	apps, err := mgmtSelectAppOptions(ctx, s.db, scope)
	if err != nil {
		return nil, err
	}
	one := 1
	roles := []framework.Row{}
	if canEdit {
		roleRows, err := mgmtSelectRoles(ctx, s.db, nil, &one, 0, 5000)
		if err != nil {
			return nil, err
		}
		for _, row := range roleRows {
			roles = append(roles, framework.Row{"id": row["id"], "name": row["name"]})
		}
	}
	initialPassword := ""
	if canSetPassword {
		key := "sys.user.initPassword"
		configs, err := mgmtSelectConfigs(ctx, s.db, &key, 0, 10)
		if err != nil {
			return nil, err
		}
		for _, row := range configs {
			if row["key"] == key {
				initialPassword = framework.AsString(row["value"])
				break
			}
		}
	}
	return map[string]any{"roles": roles, "apps": apps, "initPassword": initialPassword}, nil
}

func (s *ManagementService) DictOptions(ctx context.Context, dictType string) ([]framework.Row, error) {
	return mgmtSelectDictOptions(ctx, s.db, dictType)
}

func (s *ManagementService) Users(ctx context.Context, q framework.PageQuery, keyword *string, status *int) (mgmtPage, error) {
	total, err := mgmtCountUsers(ctx, s.db, keyword, status)
	if err != nil {
		return mgmtPage{}, err
	}
	rows, err := mgmtSelectUsers(ctx, s.db, keyword, status, q.Offset(), q.NormalizedPageSize())
	if err != nil {
		return mgmtPage{}, err
	}
	return framework.PageOf(mgmtUserRows(rows), total, q), nil
}

func (s *ManagementService) ExportUsers(ctx context.Context, keyword *string, status *int) (string, error) {
	rows, err := mgmtSelectUsers(ctx, s.db, keyword, status, 0, 5000)
	if err != nil {
		return "", err
	}
	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, []any{
			row["username"],
			row["nickname"],
			row["phone"],
			row["email"],
			row["appNames"],
			row["roleName"],
			mgmtUserStatusText(row["status"]),
			row["createTime"],
			row["lastLogin"],
		})
	}
	return mgmtCsv(
		[]string{"登录账号", "用户昵称", "手机号", "邮箱", "可管理应用", "角色", "状态", "创建时间", "最近登录"},
		data,
	), nil
}

func (s *ManagementService) Roles(ctx context.Context, q framework.PageQuery, keyword *string, status *int) (mgmtPage, error) {
	total, err := mgmtCountRoles(ctx, s.db, keyword, status)
	if err != nil {
		return mgmtPage{}, err
	}
	rows, err := mgmtSelectRoles(ctx, s.db, keyword, status, q.Offset(), q.NormalizedPageSize())
	if err != nil {
		return mgmtPage{}, err
	}
	return framework.PageOf(mgmtRoleRows(rows), total, q), nil
}

// PermissionGraph
// 权限关系图：角色、菜单与「用户-角色」关联一次返回，供溯源视图、菜单星图与问答规则在前端组合。
// 用户节点只对有用户查询权限的账号返回。
func (s *ManagementService) PermissionGraph(ctx context.Context) (map[string]any, error) {
	roleRows, err := mgmtSelectRoles(ctx, s.db, nil, nil, 0, 5000)
	if err != nil {
		return nil, err
	}
	roles := mgmtPick(mgmtRoleRows(roleRows), "id", "name", "code", "status", "builtin", "userCount", "menuIds")
	menuRows, err := mgmtSelectMenus(ctx, s.db, nil, nil)
	if err != nil {
		return nil, err
	}
	menus := mgmtPick(menuRows, "id", "parentId", "name", "type", "icon", "path", "permission", "sort", "visible", "status")
	canSeeUsers := s.perm.HasPermission(ctx, mgmtCurrentUsername(ctx), "system:user:list")
	users := []framework.Row{}
	userRoles := [][]int64{}
	if canSeeUsers {
		if users, err = mgmtSelectGraphUsers(ctx, s.db); err != nil {
			return nil, err
		}
		userIDs := map[int64]bool{}
		for _, row := range users {
			userIDs[framework.AsInt64(row["id"])] = true
		}
		links, err := mgmtSelectUserRoleLinks(ctx, s.db)
		if err != nil {
			return nil, err
		}
		for _, row := range links {
			if userIDs[framework.AsInt64(row["userId"])] {
				userRoles = append(userRoles, []int64{framework.AsInt64(row["userId"]), framework.AsInt64(row["roleId"])})
			}
		}
	}
	return map[string]any{
		"users":        users,
		"roles":        roles,
		"menus":        menus,
		"userRoles":    userRoles,
		"usersVisible": canSeeUsers,
	}, nil
}

func (s *ManagementService) RoleMenus(ctx context.Context, roleID int64) (map[string]any, error) {
	ids, err := mgmtSelectRoleMenuIDs(ctx, s.db, roleID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"menuIds": ids}, nil
}

func (s *ManagementService) ExportRoles(ctx context.Context, keyword *string, status *int) (string, error) {
	rows, err := mgmtSelectRoles(ctx, s.db, keyword, status, 0, 5000)
	if err != nil {
		return "", err
	}
	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, []any{row["name"], row["code"],
			mgmtUserStatusText(row["status"]), row["createTime"], row["remark"]})
	}
	return mgmtCsv([]string{"角色名称", "权限字符", "状态", "创建时间", "备注"}, data), nil
}

func (s *ManagementService) Menus(ctx context.Context, keyword *string, status *int) ([]framework.Row, error) {
	rows, err := mgmtSelectMenus(ctx, s.db, keyword, status)
	if err != nil {
		return nil, err
	}
	return mgmtTreeOf(rows), nil
}

func (s *ManagementService) ExportMenus(ctx context.Context, keyword *string, status *int) (string, error) {
	rows, err := mgmtSelectMenus(ctx, s.db, keyword, status)
	if err != nil {
		return "", err
	}
	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, []any{row["name"], row["type"], row["icon"],
			row["path"], row["component"], row["permission"],
			row["sort"], mgmtUserStatusText(row["status"])})
	}
	return mgmtCsv([]string{"菜单名称", "类型", "图标", "路由地址", "组件路径", "权限标识", "排序", "状态"}, data), nil
}

func (s *ManagementService) DictTypes(ctx context.Context, q framework.PageQuery, keyword *string, status *int) (mgmtPage, error) {
	total, err := mgmtCountDictTypes(ctx, s.db, keyword, status)
	if err != nil {
		return mgmtPage{}, err
	}
	rows, err := mgmtSelectDictTypes(ctx, s.db, keyword, status, q.Offset(), q.NormalizedPageSize())
	if err != nil {
		return mgmtPage{}, err
	}
	return framework.PageOf(rows, total, q), nil
}

func (s *ManagementService) ExportDictTypes(ctx context.Context, keyword *string, status *int) (string, error) {
	rows, err := mgmtSelectDictTypes(ctx, s.db, keyword, status, 0, 5000)
	if err != nil {
		return "", err
	}
	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, []any{row["dictName"], row["dictType"],
			mgmtUserStatusText(row["status"]), row["createTime"], row["remark"]})
	}
	return mgmtCsv([]string{"字典名称", "字典类型", "状态", "创建时间", "备注"}, data), nil
}

func (s *ManagementService) DictData(ctx context.Context, dictType string) ([]framework.Row, error) {
	rows, err := mgmtSelectDictData(ctx, s.db, dictType)
	if err != nil {
		return nil, err
	}
	return mgmtBooleanRows(rows, "def"), nil
}

func (s *ManagementService) Configs(ctx context.Context, q framework.PageQuery, keyword *string) (mgmtPage, error) {
	total, err := mgmtCountConfigs(ctx, s.db, keyword)
	if err != nil {
		return mgmtPage{}, err
	}
	rows, err := mgmtSelectConfigs(ctx, s.db, keyword, q.Offset(), q.NormalizedPageSize())
	if err != nil {
		return mgmtPage{}, err
	}
	return framework.PageOf(mgmtBooleanRows(rows, "builtin"), total, q), nil
}

func (s *ManagementService) ExportConfigs(ctx context.Context, keyword *string) (string, error) {
	rows, err := mgmtSelectConfigs(ctx, s.db, keyword, 0, 5000)
	if err != nil {
		return "", err
	}
	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, []any{row["name"], row["key"], row["value"],
			row["type"], row["createTime"], row["remark"]})
	}
	return mgmtCsv([]string{"参数名称", "参数键名", "参数键值", "系统内置", "创建时间", "备注"}, data), nil
}

func (s *ManagementService) OperLogs(ctx context.Context, q framework.PageQuery, keyword *string, result *int) (mgmtPage, error) {
	total, err := mgmtCountOperLogs(ctx, s.db, keyword, result)
	if err != nil {
		return mgmtPage{}, err
	}
	rows, err := mgmtSelectOperLogs(ctx, s.db, keyword, result, q.Offset(), q.NormalizedPageSize())
	if err != nil {
		return mgmtPage{}, err
	}
	return framework.PageOf(rows, total, q), nil
}

func (s *ManagementService) LoginLogs(ctx context.Context, q framework.PageQuery, keyword *string, status *int) (mgmtPage, error) {
	total, err := mgmtCountLoginLogs(ctx, s.db, keyword, status)
	if err != nil {
		return mgmtPage{}, err
	}
	rows, err := mgmtSelectLoginLogs(ctx, s.db, keyword, status, q.Offset(), q.NormalizedPageSize())
	if err != nil {
		return mgmtPage{}, err
	}
	return framework.PageOf(rows, total, q), nil
}

func (s *ManagementService) ExportOperLogs(ctx context.Context, keyword *string, result *int) (string, error) {
	rows, err := mgmtSelectOperLogs(ctx, s.db, keyword, result, 0, 5000)
	if err != nil {
		return "", err
	}
	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, []any{
			row["module"],
			row["action"],
			row["method"],
			row["url"],
			row["params"],
			row["operName"],
			row["ip"],
			row["location"],
			row["cost"],
			mgmtResultText(row["result"]),
			row["operTime"],
			row["errorMsg"],
		})
	}
	return mgmtCsv(
		[]string{"模块", "类型", "请求方式", "请求地址", "请求参数", "操作人", "IP", "地点", "耗时", "结果", "操作时间", "异常信息"},
		data,
	), nil
}

func (s *ManagementService) ExportLoginLogs(ctx context.Context, keyword *string, status *int) (string, error) {
	rows, err := mgmtSelectLoginLogs(ctx, s.db, keyword, status, 0, 5000)
	if err != nil {
		return "", err
	}
	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, []any{
			row["username"],
			row["ip"],
			row["location"],
			row["browser"],
			row["os"],
			mgmtResultText(row["status"]),
			row["msg"],
			row["loginTime"],
		})
	}
	return mgmtCsv(
		[]string{"登录账号", "IP", "地点", "浏览器", "系统", "状态", "描述", "登录时间"},
		data,
	), nil
}

// DeleteOperLog @Transactional
func (s *ManagementService) DeleteOperLog(ctx context.Context, id int64) error {
	return framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		n, err := mgmtDeleteOperLog(ctx, tx, id)
		return mgmtAffected(n, err, "操作日志不存在")
	})
}

// ClearOperLogs @Transactional
func (s *ManagementService) ClearOperLogs(ctx context.Context) error {
	return framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		_, err := mgmtClearOperLogs(ctx, tx)
		return err
	})
}

// DeleteLoginLog @Transactional
func (s *ManagementService) DeleteLoginLog(ctx context.Context, id int64) error {
	return framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		n, err := mgmtDeleteLoginLog(ctx, tx, id)
		return mgmtAffected(n, err, "登录日志不存在")
	})
}

// ClearLoginLogs @Transactional
func (s *ManagementService) ClearLoginLogs(ctx context.Context) error {
	return framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		_, err := mgmtClearLoginLogs(ctx, tx)
		return err
	})
}

// CreateUser @Transactional
func (s *ManagementService) CreateUser(ctx context.Context, req UserWriteRequest) (IdResponse, error) {
	var userID int64
	err := framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		username, err := mgmtRequired(req.Username, "登录账号不能为空")
		if err != nil {
			return err
		}
		nickname, err := mgmtRequired(req.Nickname, "用户昵称不能为空")
		if err != nil {
			return err
		}
		n, err := mgmtCountUsername(ctx, tx, username, nil)
		if err = mgmtExists(n, err, "登录账号已存在"); err != nil {
			return err
		}
		hashed, err := framework.HashPassword(mgmtDefaultString(req.Password, "admin123"))
		if err != nil {
			return err
		}
		userID, err = mgmtInsertUser(ctx, tx, username, hashed, nickname,
			req.Phone, req.Email, mgmtDefaultInt(req.Status, 1), mgmtCurrentUsername(ctx))
		if err != nil {
			return err
		}
		if err := mgmtReplaceUserRoles(ctx, tx, userID, req.RoleIDs); err != nil {
			return err
		}
		return mgmtReplaceUserApps(ctx, tx, userID, req.AppIDs)
	})
	return IdResponse{ID: userID}, err
}

// UpdateUser @Transactional
func (s *ManagementService) UpdateUser(ctx context.Context, id int64, req UserWriteRequest) error {
	return framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		n, err := mgmtUpdateUser(ctx, tx, id, mgmtTrimToNull(req.Nickname), req.Phone, req.Email, req.Status, mgmtCurrentUsername(ctx))
		if err = mgmtAffected(n, err, "用户不存在或已删除"); err != nil {
			return err
		}
		if req.RoleIDs != nil {
			if err := mgmtReplaceUserRoles(ctx, tx, id, req.RoleIDs); err != nil {
				return err
			}
		}
		if req.AppIDs != nil {
			return mgmtReplaceUserApps(ctx, tx, id, req.AppIDs)
		}
		return nil
	})
}

// UpdateUserStatus @Transactional
func (s *ManagementService) UpdateUserStatus(ctx context.Context, id int64, req UserStatusRequest) error {
	return framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		status := req.Status
		if status == nil || (*status != 0 && *status != 1) {
			return framework.Biz("用户状态只能为正常或停用")
		}
		if id == 1 && *status == 0 {
			return framework.Biz("内置管理员不允许停用")
		}
		n, err := mgmtUpdateUserStatus(ctx, tx, id, *status, mgmtCurrentUsername(ctx))
		return mgmtAffected(n, err, "用户不存在或已删除")
	})
}

// ResetUserPassword @Transactional
func (s *ManagementService) ResetUserPassword(ctx context.Context, id int64, req UserPasswordResetRequest) error {
	return framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		password := mgmtDefaultString(req.Password, "Yy@123456")
		// Java String.length() 按 UTF-16 代码单元计数
		if n := len(utf16.Encode([]rune(password))); n < 6 || n > 64 {
			return framework.Biz("密码长度必须在6到64位之间")
		}
		hashed, err := framework.HashPassword(password)
		if err != nil {
			return err
		}
		n, err := mgmtResetUserPassword(ctx, tx, id, hashed, mgmtCurrentUsername(ctx))
		return mgmtAffected(n, err, "用户不存在或已删除")
	})
}

// DeleteUser @Transactional
func (s *ManagementService) DeleteUser(ctx context.Context, id int64) error {
	return framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		if id == 1 {
			return framework.Biz("内置管理员不允许删除")
		}
		n, err := mgmtSoftDeleteUser(ctx, tx, id, mgmtCurrentUsername(ctx))
		if err = mgmtAffected(n, err, "用户不存在或已删除"); err != nil {
			return err
		}
		return mgmtDeleteUserApps(ctx, tx, id)
	})
}

// CreateRole @Transactional
func (s *ManagementService) CreateRole(ctx context.Context, req RoleWriteRequest) (IdResponse, error) {
	var roleID int64
	err := framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		name, err := mgmtRequired(req.Name, "角色名称不能为空")
		if err != nil {
			return err
		}
		code, err := mgmtRequired(req.Code, "角色编码不能为空")
		if err != nil {
			return err
		}
		n, err := mgmtCountRoleCode(ctx, tx, code, nil)
		if err = mgmtExists(n, err, "角色编码已存在"); err != nil {
			return err
		}
		roleID, err = mgmtInsertRole(ctx, tx, name, code,
			mgmtDefaultInt(req.Sort, 0), mgmtDefaultInt(req.Status, 1), req.Remark, mgmtCurrentUsername(ctx))
		if err != nil {
			return err
		}
		return mgmtReplaceRoleMenus(ctx, tx, roleID, req.MenuIDs)
	})
	return IdResponse{ID: roleID}, err
}

// UpdateRole @Transactional
func (s *ManagementService) UpdateRole(ctx context.Context, id int64, req RoleWriteRequest) error {
	return framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		n, err := mgmtUpdateRole(ctx, tx, id, mgmtTrimToNull(req.Name), req.Sort, req.Status, req.Remark, mgmtCurrentUsername(ctx))
		if err = mgmtAffected(n, err, "角色不存在、已删除或为内置角色"); err != nil {
			return err
		}
		if req.MenuIDs != nil {
			return mgmtReplaceRoleMenus(ctx, tx, id, req.MenuIDs)
		}
		return nil
	})
}

// DeleteRole @Transactional
func (s *ManagementService) DeleteRole(ctx context.Context, id int64) error {
	return framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		n, err := mgmtCountRoleUsers(ctx, tx, id)
		if err = mgmtExists(n, err, "角色已分配用户，不能删除"); err != nil {
			return err
		}
		if err := mgmtDeleteRoleMenus(ctx, tx, id); err != nil {
			return err
		}
		n, err = mgmtSoftDeleteRole(ctx, tx, id, mgmtCurrentUsername(ctx))
		return mgmtAffected(n, err, "角色不存在、已删除或为内置角色")
	})
}

// CreateMenu @Transactional
func (s *ManagementService) CreateMenu(ctx context.Context, req MenuWriteRequest) (IdResponse, error) {
	var menuID int64
	err := framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		name, err := mgmtRequired(req.Name, "菜单名称不能为空")
		if err != nil {
			return err
		}
		if menuID, err = mgmtNextMenuID(ctx, tx); err != nil {
			return err
		}
		parentID := int64(0)
		if req.ParentID != nil {
			parentID = *req.ParentID
		}
		return mgmtInsertMenu(ctx, tx, menuID, parentID, name, mgmtDefaultString(req.Type, "menu"),
			req.Icon, req.Path, req.Component, req.Permission,
			mgmtDefaultInt(req.Sort, 0), mgmtDefaultInt(req.Visible, 1), mgmtDefaultInt(req.Status, 1), mgmtCurrentUsername(ctx))
	})
	return IdResponse{ID: menuID}, err
}

// UpdateMenu @Transactional
func (s *ManagementService) UpdateMenu(ctx context.Context, id int64, req MenuWriteRequest) error {
	return framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		n, err := mgmtUpdateMenu(ctx, tx, id, req.ParentID, mgmtTrimToNull(req.Name), mgmtTrimToNull(req.Type),
			req.Icon, req.Path, req.Component, req.Permission, req.Sort, req.Visible, req.Status, mgmtCurrentUsername(ctx))
		return mgmtAffected(n, err, "菜单不存在")
	})
}

// DeleteMenu @Transactional
func (s *ManagementService) DeleteMenu(ctx context.Context, id int64) error {
	return framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		n, err := mgmtCountMenuChildren(ctx, tx, id)
		if err = mgmtExists(n, err, "存在子菜单，不能删除"); err != nil {
			return err
		}
		n, err = mgmtCountMenuRoles(ctx, tx, id)
		if err = mgmtExists(n, err, "菜单已分配角色，不能删除"); err != nil {
			return err
		}
		n, err = mgmtDeleteMenu(ctx, tx, id)
		return mgmtAffected(n, err, "菜单不存在")
	})
}

// CreateDictType @Transactional
func (s *ManagementService) CreateDictType(ctx context.Context, req DictTypeWriteRequest) (IdResponse, error) {
	var id int64
	err := framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		dictName, err := mgmtRequired(req.DictName, "字典名称不能为空")
		if err != nil {
			return err
		}
		dictType, err := mgmtRequired(req.DictType, "字典类型不能为空")
		if err != nil {
			return err
		}
		n, err := mgmtCountDictType(ctx, tx, dictType, nil)
		if err = mgmtExists(n, err, "字典类型已存在"); err != nil {
			return err
		}
		id, err = mgmtInsertDictType(ctx, tx, dictName, dictType, mgmtDefaultInt(req.Status, 1), req.Remark, mgmtCurrentUsername(ctx))
		return err
	})
	return IdResponse{ID: id}, err
}

// UpdateDictType @Transactional
func (s *ManagementService) UpdateDictType(ctx context.Context, id int64, req DictTypeWriteRequest) error {
	return framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		n, err := mgmtUpdateDictType(ctx, tx, id, mgmtTrimToNull(req.DictName), req.Status, req.Remark, mgmtCurrentUsername(ctx))
		return mgmtAffected(n, err, "字典类型不存在")
	})
}

// DeleteDictType @Transactional
func (s *ManagementService) DeleteDictType(ctx context.Context, id int64) error {
	return framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		dictType, err := mgmtSelectDictTypeCode(ctx, tx, id)
		if err != nil {
			return err
		}
		if dictType == nil {
			return framework.Biz("字典类型不存在")
		}
		n, err := mgmtCountDictDataByType(ctx, tx, *dictType)
		if err = mgmtExists(n, err, "字典类型存在字典数据，不能删除"); err != nil {
			return err
		}
		_, err = mgmtDeleteDictType(ctx, tx, id)
		return err
	})
}

// CreateDictData @Transactional
func (s *ManagementService) CreateDictData(ctx context.Context, req DictDataWriteRequest) (IdResponse, error) {
	var id int64
	err := framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		dictType, err := mgmtRequired(req.DictType, "字典类型不能为空")
		if err != nil {
			return err
		}
		label, err := mgmtRequired(req.Label, "字典标签不能为空")
		if err != nil {
			return err
		}
		value, err := mgmtRequired(req.Value, "字典值不能为空")
		if err != nil {
			return err
		}
		id, err = mgmtInsertDictData(ctx, tx, dictType, label, value, mgmtDefaultInt(req.Sort, 0), mgmtDefaultInt(req.Status, 1),
			req.Tone, mgmtBoolAsInt(req.Def), req.Remark, mgmtCurrentUsername(ctx))
		return err
	})
	return IdResponse{ID: id}, err
}

// UpdateDictData @Transactional
func (s *ManagementService) UpdateDictData(ctx context.Context, id int64, req DictDataWriteRequest) error {
	return framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		n, err := mgmtUpdateDictData(ctx, tx, id, mgmtTrimToNull(req.DictType), mgmtTrimToNull(req.Label), mgmtTrimToNull(req.Value),
			req.Sort, req.Status, req.Tone, mgmtBoolAsIntPtr(req.Def), req.Remark, mgmtCurrentUsername(ctx))
		return mgmtAffected(n, err, "字典数据不存在")
	})
}

// DeleteDictData @Transactional
func (s *ManagementService) DeleteDictData(ctx context.Context, id int64) error {
	return framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		n, err := mgmtDeleteDictData(ctx, tx, id)
		return mgmtAffected(n, err, "字典数据不存在")
	})
}

// CreateConfig @Transactional
func (s *ManagementService) CreateConfig(ctx context.Context, req ConfigWriteRequest) (IdResponse, error) {
	var id int64
	err := framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		name, err := mgmtRequired(req.Name, "参数名称不能为空")
		if err != nil {
			return err
		}
		key, err := mgmtRequired(req.Key, "参数键名不能为空")
		if err != nil {
			return err
		}
		value, err := mgmtRequired(req.Value, "参数键值不能为空")
		if err != nil {
			return err
		}
		n, err := mgmtCountConfigKey(ctx, tx, key, nil)
		if err = mgmtExists(n, err, "参数键名已存在"); err != nil {
			return err
		}
		id, err = mgmtInsertConfig(ctx, tx, name, key, value, mgmtDefaultString(req.Type, "N"), mgmtBoolAsInt(req.Builtin), req.Remark, mgmtCurrentUsername(ctx))
		return err
	})
	return IdResponse{ID: id}, err
}

// UpdateConfig @Transactional
func (s *ManagementService) UpdateConfig(ctx context.Context, id int64, req ConfigWriteRequest) error {
	return framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		n, err := mgmtUpdateConfig(ctx, tx, id, mgmtTrimToNull(req.Name), mgmtTrimToNull(req.Value), mgmtTrimToNull(req.Type),
			mgmtBoolAsIntPtr(req.Builtin), req.Remark, mgmtCurrentUsername(ctx))
		return mgmtAffected(n, err, "参数配置不存在或为内置参数")
	})
}

// DeleteConfig @Transactional
func (s *ManagementService) DeleteConfig(ctx context.Context, id int64) error {
	return framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		n, err := mgmtDeleteConfig(ctx, tx, id)
		return mgmtAffected(n, err, "参数配置不存在或为内置参数")
	})
}

// ---------- 私有辅助（对应 Java 同名 private 方法） ----------

// treeOf：按 id 建索引（LinkedHashMap 语义：重复 id 覆盖值但保留首次顺序），parentId 找不到即为根。
func mgmtTreeOf(rows []framework.Row) []framework.Row {
	byID := make(map[int64]framework.Row, len(rows))
	order := make([]int64, 0, len(rows))
	for _, row := range rows {
		node := maps.Clone(row)
		node["children"] = []framework.Row{}
		id := framework.AsInt64(node["id"])
		if _, seen := byID[id]; !seen {
			order = append(order, id)
		}
		byID[id] = node
	}
	roots := []framework.Row{}
	for _, id := range order {
		node := byID[id]
		parent, ok := byID[framework.AsInt64(node["parentId"])]
		if !ok {
			roots = append(roots, node)
		} else {
			parent["children"] = append(parent["children"].([]framework.Row), node)
		}
	}
	return roots
}

// booleanRows
func mgmtBooleanRows(rows []framework.Row, key string) []framework.Row {
	out := make([]framework.Row, 0, len(rows))
	for _, row := range rows {
		out = append(out, mgmtBooleanRow(row, key))
	}
	return out
}

// roleRows：builtin 转布尔，menuIdsText 拆为 menuIds 列表。
func mgmtRoleRows(rows []framework.Row) []framework.Row {
	out := make([]framework.Row, 0, len(rows))
	for _, row := range rows {
		c := mgmtBooleanRow(row, "builtin")
		c["menuIds"] = mgmtSplitIDs(c["menuIdsText"])
		delete(c, "menuIdsText")
		out = append(out, c)
	}
	return out
}

// userRows：appIdsText 拆为 appIds 列表（appNames 保持逗号拼接的字符串）。
func mgmtUserRows(rows []framework.Row) []framework.Row {
	out := make([]framework.Row, 0, len(rows))
	for _, row := range rows {
		c := maps.Clone(row)
		c["appIds"] = mgmtSplitIDs(c["appIdsText"])
		delete(c, "appIdsText")
		out = append(out, c)
	}
	return out
}

// mgmtSplitIDs 把 GROUP_CONCAT 出来的 "1,2,3" 拆为 []int64；空、NULL 均为空切片。
func mgmtSplitIDs(v any) []int64 {
	ids := []int64{}
	text := framework.AsString(v)
	if strings.TrimSpace(text) == "" || text == "null" {
		return ids
	}
	for _, id := range strings.Split(text, ",") {
		if strings.TrimSpace(id) != "" {
			ids = append(ids, framework.AsInt64(id))
		}
	}
	return ids
}

// booleanRow
func mgmtBooleanRow(row framework.Row, key string) framework.Row {
	c := maps.Clone(row)
	c[key] = framework.AsInt64(c[key]) == 1
	return c
}

// mgmtPick 对应 for (key : List.of(...)) node.put(key, row.get(key))。
func mgmtPick(rows []framework.Row, keys ...string) []framework.Row {
	out := make([]framework.Row, 0, len(rows))
	for _, row := range rows {
		node := make(framework.Row, len(keys))
		for _, k := range keys {
			node[k] = row[k]
		}
		out = append(out, node)
	}
	return out
}

// replaceUserRoles
func mgmtReplaceUserRoles(ctx context.Context, q framework.Querier, userID int64, roleIDs []int64) error {
	if err := mgmtDeleteUserRoles(ctx, q, userID); err != nil {
		return err
	}
	for _, roleID := range roleIDs {
		if err := mgmtInsertUserRole(ctx, q, userID, roleID); err != nil {
			return err
		}
	}
	return nil
}

// replaceUserApps：整体替换 sys_user_app 绑定，与角色同一模式。
func mgmtReplaceUserApps(ctx context.Context, q framework.Querier, userID int64, appIDs []int64) error {
	if err := mgmtDeleteUserApps(ctx, q, userID); err != nil {
		return err
	}
	for _, appID := range appIDs {
		if err := mgmtInsertUserApp(ctx, q, userID, appID); err != nil {
			return err
		}
	}
	return nil
}

// replaceRoleMenus
func mgmtReplaceRoleMenus(ctx context.Context, q framework.Querier, roleID int64, menuIDs []int64) error {
	if err := mgmtDeleteRoleMenus(ctx, q, roleID); err != nil {
		return err
	}
	for _, menuID := range menuIDs {
		if err := mgmtInsertRoleMenu(ctx, q, roleID, menuID); err != nil {
			return err
		}
	}
	return nil
}

// currentUsername：未登录时为 "system"。
func mgmtCurrentUsername(ctx context.Context) string {
	if u := framework.CurrentUsername(ctx); u != "" {
		return u
	}
	return "system"
}

// required
func mgmtRequired(v *string, msg string) (string, error) {
	t := mgmtTrimToNull(v)
	if t == nil {
		return "", framework.Biz(msg)
	}
	return *t, nil
}

// trimToNull：Java String.trim() 只去除 <= U+0020 的字符。
func mgmtTrimToNull(v *string) *string {
	if v == nil {
		return nil
	}
	t := strings.TrimFunc(*v, func(r rune) bool { return r <= ' ' })
	if t == "" {
		return nil
	}
	return &t
}

// defaultString
func mgmtDefaultString(v *string, def string) string {
	if t := mgmtTrimToNull(v); t != nil {
		return *t
	}
	return def
}

// defaultInt
func mgmtDefaultInt(v *int, def int) int {
	if v == nil {
		return def
	}
	return *v
}

// boolAsInt：Boolean.TRUE.equals(v) ? 1 : 0
func mgmtBoolAsInt(v *bool) int {
	if v != nil && *v {
		return 1
	}
	return 0
}

// 对应 v == null ? null : boolAsInt(v)
func mgmtBoolAsIntPtr(v *bool) *int {
	if v == nil {
		return nil
	}
	n := mgmtBoolAsInt(v)
	return &n
}

// mgmtAffected 对应 if (mapper.update/delete(...) == 0) throw new BusinessException(msg)。
func mgmtAffected(n int64, err error, msg string) error {
	if err != nil {
		return err
	}
	if n == 0 {
		return framework.Biz(msg)
	}
	return nil
}

// mgmtExists 对应 if (mapper.countXxx(...) > 0) throw new BusinessException(msg)。
func mgmtExists(n int64, err error, msg string) error {
	if err != nil {
		return err
	}
	if n > 0 {
		return framework.Biz(msg)
	}
	return nil
}

// csv
func mgmtCsv(headers []string, rows [][]any) string {
	var sb strings.Builder
	sb.WriteString(strings.Join(headers, ","))
	sb.WriteByte('\n')
	for _, row := range rows {
		for i, v := range row {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(mgmtCsvCell(v))
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// csvCell
func mgmtCsvCell(v any) string {
	text := framework.StrOrEmpty(v)
	if strings.Contains(text, `"`) {
		text = strings.ReplaceAll(text, `"`, `""`)
	}
	if strings.ContainsAny(text, ",\"\n\r") {
		return `"` + text + `"`
	}
	return text
}

// 对应 value instanceof Number ? intValue() == 1 : "1".equals(String.valueOf(value))
func mgmtIsOne(v any) bool {
	switch v.(type) {
	case int64, int, int32, float64, json.Number:
		return framework.AsInt64(v) == 1
	}
	return framework.AsString(v) == "1"
}

// resultText
func mgmtResultText(v any) string {
	if mgmtIsOne(v) {
		return "成功"
	}
	return "失败"
}

// userStatusText
func mgmtUserStatusText(v any) string {
	if mgmtIsOne(v) {
		return "正常"
	}
	return "停用"
}

// ---------- SystemManagementController / MonitorController（日志部分） ----------

// mgmtJSON 把 (data, err) 写成 R.success(data)。
func mgmtJSON(fn func(r *http.Request) (any, error)) framework.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		data, err := fn(r)
		if err != nil {
			return err
		}
		return framework.OK(w, r, data)
	}
}

// mgmtPageHandler 对应 (PageQuery query, String keyword, Integer <filter>) 形式的分页接口。
func mgmtPageHandler(filter string, fn func(ctx context.Context, q framework.PageQuery, keyword *string, status *int) (mgmtPage, error)) framework.HandlerFunc {
	return mgmtJSON(func(r *http.Request) (any, error) {
		q, err := framework.ParsePage(r)
		if err != nil {
			return nil, err
		}
		status, err := framework.QueryInt(r, filter)
		if err != nil {
			return nil, err
		}
		return fn(r.Context(), q, framework.QueryString(r, "keyword"), status)
	})
}

// mgmtExportHandler 对应 (String keyword, Integer <filter>) 形式的 CSV 导出接口。
func mgmtExportHandler(filename, filter string, fn func(ctx context.Context, keyword *string, status *int) (string, error)) framework.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		status, err := framework.QueryInt(r, filter)
		if err != nil {
			return err
		}
		content, err := fn(r.Context(), framework.QueryString(r, "keyword"), status)
		if err != nil {
			return err
		}
		return framework.CSV(w, filename, content)
	}
}

// mgmtTreeHandler 对应 (String keyword, Integer status) 形式的树接口。
func mgmtTreeHandler(fn func(ctx context.Context, keyword *string, status *int) ([]framework.Row, error)) framework.HandlerFunc {
	return mgmtJSON(func(r *http.Request) (any, error) {
		status, err := framework.QueryInt(r, "status")
		if err != nil {
			return nil, err
		}
		return fn(r.Context(), framework.QueryString(r, "keyword"), status)
	})
}

// mgmtCreateHandler 对应 @PostMapping + @RequestBody → R<IdResponse>。
func mgmtCreateHandler[T any](fn func(ctx context.Context, req T) (IdResponse, error)) framework.HandlerFunc {
	return mgmtJSON(func(r *http.Request) (any, error) {
		var req T
		if err := framework.Bind(r, &req); err != nil {
			return nil, err
		}
		return fn(r.Context(), req)
	})
}

// mgmtUpdateHandler 对应 @PutMapping("/{id}") + @RequestBody → R<Void>。
func mgmtUpdateHandler[T any](fn func(ctx context.Context, id int64, req T) error) framework.HandlerFunc {
	return mgmtJSON(func(r *http.Request) (any, error) {
		id, err := framework.PathInt64(r, "id")
		if err != nil {
			return nil, err
		}
		var req T
		if err := framework.Bind(r, &req); err != nil {
			return nil, err
		}
		return nil, fn(r.Context(), id, req)
	})
}

// mgmtDeleteHandler 对应 @DeleteMapping("/{id}") → R<Void>。
func mgmtDeleteHandler(fn func(ctx context.Context, id int64) error) framework.HandlerFunc {
	return mgmtJSON(func(r *http.Request) (any, error) {
		id, err := framework.PathInt64(r, "id")
		if err != nil {
			return nil, err
		}
		return nil, fn(r.Context(), id)
	})
}

// RegisterManagementRoutes 对应 SystemManagementController 全部路由 + MonitorController 的 8 个日志路由。
// 中间件顺序：@OperLog 在外层，@RequiresPermission 在内层。
func RegisterManagementRoutes(mux *http.ServeMux, d Deps, s *ManagementService) {
	perm := d.Security.Perm
	oper := func(module, action string) framework.Middleware { return framework.OperLog(d.Recorder, module, action) }

	// ---------- 后台账号 ----------
	mux.Handle("GET /api/system/users", framework.Handle(mgmtPageHandler("status", s.Users), perm("system:user:list")))
	mux.Handle("GET /api/system/users/options", framework.Handle(mgmtJSON(func(r *http.Request) (any, error) {
		return s.UserOptions(r.Context())
	}), perm("system:user:list")))
	mux.Handle("GET /api/system/users/export", framework.Handle(mgmtExportHandler("users.csv", "status", s.ExportUsers), perm("system:user:list")))
	mux.Handle("POST /api/system/users", framework.Handle(mgmtCreateHandler(s.CreateUser), oper("后台账号", "新增"), perm("system:user:add")))
	mux.Handle("PUT /api/system/users/{id}", framework.Handle(mgmtUpdateHandler(s.UpdateUser), oper("后台账号", "修改"), perm("system:user:edit")))
	mux.Handle("PUT /api/system/users/{id}/status", framework.Handle(mgmtUpdateHandler(s.UpdateUserStatus), oper("后台账号", "状态变更"), perm("system:user:edit")))
	mux.Handle("PUT /api/system/users/{id}/password", framework.Handle(mgmtUpdateHandler(s.ResetUserPassword), oper("后台账号", "重置密码"), perm("system:user:resetPwd")))
	mux.Handle("DELETE /api/system/users/{id}", framework.Handle(mgmtDeleteHandler(s.DeleteUser), oper("后台账号", "删除"), perm("system:user:remove")))

	// ---------- 角色 ----------
	mux.Handle("GET /api/system/roles/export", framework.Handle(mgmtExportHandler("roles.csv", "status", s.ExportRoles), perm("system:role:list")))
	mux.Handle("GET /api/system/roles", framework.Handle(mgmtPageHandler("status", s.Roles), perm("system:role:list")))
	mux.Handle("GET /api/system/permissions/graph", framework.Handle(mgmtJSON(func(r *http.Request) (any, error) {
		return s.PermissionGraph(r.Context())
	}), perm("system:role:list")))
	mux.Handle("GET /api/system/roles/{id}/menus", framework.Handle(mgmtJSON(func(r *http.Request) (any, error) {
		id, err := framework.PathInt64(r, "id")
		if err != nil {
			return nil, err
		}
		return s.RoleMenus(r.Context(), id)
	}), perm("system:role:list")))
	mux.Handle("POST /api/system/roles", framework.Handle(mgmtCreateHandler(s.CreateRole), oper("角色管理", "新增"), perm("system:role:add")))
	mux.Handle("PUT /api/system/roles/{id}", framework.Handle(mgmtUpdateHandler(s.UpdateRole), oper("角色管理", "修改"), perm("system:role:edit")))
	mux.Handle("DELETE /api/system/roles/{id}", framework.Handle(mgmtDeleteHandler(s.DeleteRole), oper("角色管理", "删除"), perm("system:role:remove")))

	// ---------- 菜单 ----------
	mux.Handle("GET /api/system/menus/export", framework.Handle(mgmtExportHandler("menus.csv", "status", s.ExportMenus), perm("system:menu:list")))
	mux.Handle("GET /api/system/menus/tree", framework.Handle(mgmtTreeHandler(s.Menus), perm("system:menu:list")))
	mux.Handle("POST /api/system/menus", framework.Handle(mgmtCreateHandler(s.CreateMenu), oper("菜单管理", "新增"), perm("system:menu:add")))
	mux.Handle("PUT /api/system/menus/{id}", framework.Handle(mgmtUpdateHandler(s.UpdateMenu), oper("菜单管理", "修改"), perm("system:menu:edit")))
	mux.Handle("DELETE /api/system/menus/{id}", framework.Handle(mgmtDeleteHandler(s.DeleteMenu), oper("菜单管理", "删除"), perm("system:menu:remove")))

	// ---------- 字典 ----------
	mux.Handle("GET /api/system/dict/types/export", framework.Handle(mgmtExportHandler("dict-types.csv", "status", s.ExportDictTypes), perm("system:dict:list")))
	mux.Handle("GET /api/system/dict/types", framework.Handle(mgmtPageHandler("status", s.DictTypes), perm("system:dict:list")))
	mux.Handle("GET /api/system/dict/options/{dictType}", framework.Handle(mgmtJSON(func(r *http.Request) (any, error) {
		return s.DictOptions(r.Context(), r.PathValue("dictType"))
	})))
	mux.Handle("GET /api/system/dict/data/{dictType}", framework.Handle(mgmtJSON(func(r *http.Request) (any, error) {
		return s.DictData(r.Context(), r.PathValue("dictType"))
	}), perm("system:dict:list")))
	mux.Handle("POST /api/system/dict/types", framework.Handle(mgmtCreateHandler(s.CreateDictType), oper("字典管理", "新增字典类型"), perm("system:dict:add")))
	mux.Handle("PUT /api/system/dict/types/{id}", framework.Handle(mgmtUpdateHandler(s.UpdateDictType), oper("字典管理", "修改字典类型"), perm("system:dict:edit")))
	mux.Handle("DELETE /api/system/dict/types/{id}", framework.Handle(mgmtDeleteHandler(s.DeleteDictType), oper("字典管理", "删除字典类型"), perm("system:dict:remove")))
	mux.Handle("POST /api/system/dict/data", framework.Handle(mgmtCreateHandler(s.CreateDictData), oper("字典管理", "新增字典数据"), perm("system:dict:add")))
	mux.Handle("PUT /api/system/dict/data/{id}", framework.Handle(mgmtUpdateHandler(s.UpdateDictData), oper("字典管理", "修改字典数据"), perm("system:dict:edit")))
	mux.Handle("DELETE /api/system/dict/data/{id}", framework.Handle(mgmtDeleteHandler(s.DeleteDictData), oper("字典管理", "删除字典数据"), perm("system:dict:remove")))

	// ---------- 参数配置 ----------
	mux.Handle("GET /api/system/configs/export", framework.Handle(func(w http.ResponseWriter, r *http.Request) error {
		content, err := s.ExportConfigs(r.Context(), framework.QueryString(r, "keyword"))
		if err != nil {
			return err
		}
		return framework.CSV(w, "configs.csv", content)
	}, perm("system:config:list")))
	mux.Handle("GET /api/system/configs", framework.Handle(mgmtJSON(func(r *http.Request) (any, error) {
		q, err := framework.ParsePage(r)
		if err != nil {
			return nil, err
		}
		return s.Configs(r.Context(), q, framework.QueryString(r, "keyword"))
	}), perm("system:config:list")))
	mux.Handle("POST /api/system/configs", framework.Handle(mgmtCreateHandler(s.CreateConfig), oper("参数配置", "新增"), perm("system:config:add")))
	mux.Handle("PUT /api/system/configs/{id}", framework.Handle(mgmtUpdateHandler(s.UpdateConfig), oper("参数配置", "修改"), perm("system:config:edit")))
	mux.Handle("DELETE /api/system/configs/{id}", framework.Handle(mgmtDeleteHandler(s.DeleteConfig), oper("参数配置", "删除"), perm("system:config:remove")))

	// ---------- MonitorController：操作日志 / 登录日志 ----------
	mux.Handle("GET /api/monitor/oper-logs", framework.Handle(mgmtPageHandler("result", s.OperLogs), perm("monitor:operlog:list")))
	mux.Handle("GET /api/monitor/oper-logs/export", framework.Handle(mgmtExportHandler("oper-logs.csv", "result", s.ExportOperLogs), perm("monitor:operlog:list")))
	mux.Handle("DELETE /api/monitor/oper-logs/{id}", framework.Handle(mgmtDeleteHandler(s.DeleteOperLog), perm("monitor:operlog:remove")))
	mux.Handle("DELETE /api/monitor/oper-logs", framework.Handle(mgmtJSON(func(r *http.Request) (any, error) {
		return nil, s.ClearOperLogs(r.Context())
	}), perm("monitor:operlog:remove")))
	mux.Handle("GET /api/monitor/login-logs", framework.Handle(mgmtPageHandler("status", s.LoginLogs), perm("monitor:loginlog:list")))
	mux.Handle("GET /api/monitor/login-logs/export", framework.Handle(mgmtExportHandler("login-logs.csv", "status", s.ExportLoginLogs), perm("monitor:loginlog:list")))
	mux.Handle("DELETE /api/monitor/login-logs/{id}", framework.Handle(mgmtDeleteHandler(s.DeleteLoginLog), perm("monitor:loginlog:remove")))
	mux.Handle("DELETE /api/monitor/login-logs", framework.Handle(mgmtJSON(func(r *http.Request) (any, error) {
		return nil, s.ClearLoginLogs(r.Context())
	}), perm("monitor:loginlog:remove")))
}
