package system

import (
	"context"
	"database/sql"
	"net/http"
	"slices"
	"sort"
	"strings"
	"unicode/utf16"

	"scaffold/internal/framework"
)

const superAdmin = "super_admin"

// ---------- AuthMapper ----------

// authUserRow 对应 Java AuthUserRow。
type authUserRow struct {
	ID       int64
	Username string
	Password string
	Nickname *string
	Status   *int
	DeptID   *int64
	DeptName *string
}

func selectUserByUsername(ctx context.Context, q framework.Querier, username string) (*authUserRow, error) {
	row, err := framework.SelectRow(ctx, q, `
		SELECT u.id, u.username, u.password, u.nickname, u.status, u.dept_id, d.dept_name
		FROM sys_user u
		LEFT JOIN sys_dept d ON d.id = u.dept_id
		WHERE u.username = ? AND u.deleted = 0
		LIMIT 1`, username)
	if err != nil || row == nil {
		return nil, err
	}
	u := &authUserRow{
		ID:       framework.AsInt64(row["id"]),
		Username: framework.StrOrEmpty(row["username"]),
		Password: framework.StrOrEmpty(row["password"]),
		DeptID:   framework.AsInt64Ptr(row["dept_id"]),
	}
	if v := row["nickname"]; v != nil {
		s := framework.StrOrEmpty(v)
		u.Nickname = &s
	}
	if v := row["status"]; v != nil {
		n := int(framework.AsInt64(v))
		u.Status = &n
	}
	if v := row["dept_name"]; v != nil {
		s := framework.StrOrEmpty(v)
		u.DeptName = &s
	}
	return u, nil
}

func (u *authUserRow) enabled() bool { return u != nil && u.Status != nil && *u.Status == 1 }

func selectRoleCodes(ctx context.Context, q framework.Querier, userID int64) ([]string, error) {
	return framework.SelectStrings(ctx, q, `
		SELECT r.role_code
		FROM sys_user_role ur
		JOIN sys_role r ON r.id = ur.role_id
		WHERE ur.user_id = ? AND r.status = 1 AND r.deleted = 0
		ORDER BY r.sort, r.id`, userID)
}

func selectPermissions(ctx context.Context, q framework.Querier, userID int64) ([]string, error) {
	return framework.SelectStrings(ctx, q, `
		SELECT DISTINCT m.permission
		FROM sys_user_role ur
		JOIN sys_role r ON r.id = ur.role_id AND r.status = 1 AND r.deleted = 0
		JOIN sys_role_menu rm ON rm.role_id = ur.role_id
		JOIN sys_menu m ON m.id = rm.menu_id
		WHERE ur.user_id = ?
		  AND m.status = 1
		  AND m.permission IS NOT NULL
		  AND m.permission <> ''
		ORDER BY m.permission`, userID)
}

func selectAllPermissions(ctx context.Context, q framework.Querier) ([]string, error) {
	return framework.SelectStrings(ctx, q, `
		SELECT DISTINCT permission
		FROM sys_menu
		WHERE status = 1 AND permission IS NOT NULL AND permission <> ''
		ORDER BY permission`)
}

// menuRow 对应 Java MenuRow。
type menuRow struct {
	ID         int64
	ParentID   int64
	MenuName   *string
	MenuType   *string
	Icon       *string
	Path       *string
	Component  *string
	Permission *string
	Sort       int64
}

func toMenuRows(rows []framework.Row) []menuRow {
	out := make([]menuRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, menuRow{
			ID:         framework.AsInt64(r["id"]),
			ParentID:   framework.AsInt64(r["parent_id"]),
			MenuName:   strPtr(r["menu_name"]),
			MenuType:   strPtr(r["menu_type"]),
			Icon:       strPtr(r["icon"]),
			Path:       strPtr(r["path"]),
			Component:  strPtr(r["component"]),
			Permission: strPtr(r["permission"]),
			Sort:       framework.AsInt64(r["sort"]),
		})
	}
	return out
}

func strPtr(v any) *string {
	if v == nil {
		return nil
	}
	s := framework.StrOrEmpty(v)
	return &s
}

func selectRouteMenus(ctx context.Context, q framework.Querier, userID int64) ([]menuRow, error) {
	rows, err := framework.SelectRows(ctx, q, `
		SELECT DISTINCT m.id, m.parent_id, m.menu_name, m.menu_type, m.icon, m.path,
		       m.component, m.permission, m.sort
		FROM sys_user_role ur
		JOIN sys_role r ON r.id = ur.role_id AND r.status = 1 AND r.deleted = 0
		JOIN sys_role_menu rm ON rm.role_id = ur.role_id
		JOIN sys_menu m ON m.id = rm.menu_id
		WHERE ur.user_id = ?
		  AND m.status = 1
		  AND m.visible = 1
		  AND m.menu_type IN ('dir', 'menu')
		ORDER BY m.parent_id, m.sort, m.id`, userID)
	return toMenuRows(rows), err
}

func selectAllRouteMenus(ctx context.Context, q framework.Querier) ([]menuRow, error) {
	rows, err := framework.SelectRows(ctx, q, `
		SELECT id, parent_id, menu_name, menu_type, icon, path, component, permission, sort
		FROM sys_menu
		WHERE status = 1 AND visible = 1 AND menu_type IN ('dir', 'menu')
		ORDER BY parent_id, sort, id`)
	return toMenuRows(rows), err
}

func insertLoginLog(ctx context.Context, q framework.Querier, username string, status int, msg string) error {
	_, err := framework.Exec(ctx, q, `
		INSERT INTO sys_login_log (username, status, msg, login_time)
		VALUES (?, ?, ?, NOW())`, username, status, msg)
	return err
}

func updateLastLoginTime(ctx context.Context, q framework.Querier, userID int64) error {
	_, err := framework.Exec(ctx, q, `UPDATE sys_user SET last_login_time = NOW() WHERE id = ? AND deleted = 0`, userID)
	return err
}

func updatePassword(ctx context.Context, q framework.Querier, userID int64, username, hashed string) (int64, error) {
	return framework.Exec(ctx, q, `
		UPDATE sys_user
		SET password = ?, update_by = ?, update_time = NOW()
		WHERE id = ? AND deleted = 0`, hashed, username, userID)
}

// ---------- SystemPermissionResolver ----------

// PermissionResolver 对应 Java SystemPermissionResolver。
type PermissionResolver struct{ db *sql.DB }

func NewPermissionResolver(db *sql.DB) *PermissionResolver { return &PermissionResolver{db: db} }

func (p *PermissionResolver) HasPermission(ctx context.Context, username, permission string) bool {
	user, err := selectUserByUsername(ctx, p.db, username)
	if err != nil || !user.enabled() {
		return false
	}
	roles, err := selectRoleCodes(ctx, p.db, user.ID)
	if err != nil {
		return false
	}
	if slices.Contains(roles, superAdmin) {
		return true
	}
	perms, err := selectPermissions(ctx, p.db, user.ID)
	return err == nil && slices.Contains(perms, permission)
}

// ---------- AuthService ----------

type loginRequest struct {
	Username    *string `json:"username"`
	Password    *string `json:"password"`
	Captcha     *string `json:"captcha"`
	CaptchaUUID *string `json:"captchaUuid"`
}

type changePasswordRequest struct {
	OldPassword *string `json:"oldPassword"`
	NewPassword *string `json:"newPassword"`
}

type loginResponse struct {
	Token     string `json:"token"`
	ExpiresIn int64  `json:"expiresIn"`
}

type currentUserResponse struct {
	UserID      int64    `json:"userId"`
	Username    string   `json:"username"`
	Nickname    *string  `json:"nickname"`
	DeptID      *int64   `json:"deptId"`
	DeptName    *string  `json:"deptName"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

// routeVO 对应 Java RouteVO。
type routeVO struct {
	ID         int64      `json:"id"`
	ParentID   int64      `json:"parentId"`
	Name       *string    `json:"name"`
	Path       *string    `json:"path"`
	Component  *string    `json:"component"`
	Icon       *string    `json:"icon"`
	Permission *string    `json:"permission"`
	Children   []*routeVO `json:"children"`
}

// AuthService 对应 Java AuthService。
type AuthService struct {
	db     *sql.DB
	jwt    *framework.JWT
	tokens *framework.TokenStore
}

func NewAuthService(db *sql.DB, jwt *framework.JWT, tokens *framework.TokenStore) *AuthService {
	return &AuthService{db: db, jwt: jwt, tokens: tokens}
}

func (s *AuthService) Login(ctx context.Context, req loginRequest) (*loginResponse, error) {
	username := deref(req.Username)
	user, err := selectUserByUsername(ctx, s.db, username)
	if err != nil {
		return nil, err
	}
	if user == nil || !framework.PasswordMatches(deref(req.Password), user.Password) {
		_ = insertLoginLog(ctx, s.db, username, 0, "账号或密码错误")
		return nil, framework.Biz("账号或密码错误")
	}
	if !user.enabled() {
		_ = insertLoginLog(ctx, s.db, username, 0, "账号已停用")
		return nil, framework.Biz("账号已停用")
	}
	token := s.jwt.Generate(user.ID, user.Username)
	if err := s.tokens.Store(ctx, token, user.Username, s.jwt.Expires); err != nil {
		return nil, err
	}
	if err := insertLoginLog(ctx, s.db, user.Username, 1, "登录成功"); err != nil {
		return nil, err
	}
	if err := updateLastLoginTime(ctx, s.db, user.ID); err != nil {
		return nil, err
	}
	return &loginResponse{Token: token, ExpiresIn: s.jwt.ExpiresSeconds()}, nil
}

func (s *AuthService) Logout(ctx context.Context) error {
	if p := framework.CurrentPrincipal(ctx); p != nil {
		s.tokens.Revoke(ctx, p.Token)
		return insertLoginLog(ctx, s.db, p.Username, 1, "退出登录")
	}
	return nil
}

func (s *AuthService) CurrentUser(ctx context.Context) (*currentUserResponse, error) {
	user, err := s.currentUserRow(ctx)
	if err != nil {
		return nil, err
	}
	roles, err := selectRoleCodes(ctx, s.db, user.ID)
	if err != nil {
		return nil, err
	}
	var perms []string
	if slices.Contains(roles, superAdmin) {
		perms, err = selectAllPermissions(ctx, s.db)
	} else {
		perms, err = selectPermissions(ctx, s.db, user.ID)
	}
	if err != nil {
		return nil, err
	}
	return &currentUserResponse{
		UserID: user.ID, Username: user.Username, Nickname: user.Nickname,
		DeptID: user.DeptID, DeptName: user.DeptName, Roles: roles, Permissions: perms,
	}, nil
}

func (s *AuthService) Routes(ctx context.Context) (map[string]any, error) {
	user, err := s.currentUserRow(ctx)
	if err != nil {
		return nil, err
	}
	roles, err := selectRoleCodes(ctx, s.db, user.ID)
	if err != nil {
		return nil, err
	}
	var menus []menuRow
	// 超级管理员不受角色菜单分配限制，新增菜单无需再授权即可见
	if slices.Contains(roles, superAdmin) {
		menus, err = selectAllRouteMenus(ctx, s.db)
	} else {
		menus, err = selectRouteMenus(ctx, s.db, user.ID)
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"routes": buildRoutes(menus)}, nil
}

func (s *AuthService) ChangePassword(ctx context.Context, req changePasswordRequest) error {
	user, err := s.currentUserRow(ctx)
	if err != nil {
		return err
	}
	if !framework.PasswordMatches(deref(req.OldPassword), user.Password) {
		return framework.Biz("旧密码不正确")
	}
	if framework.PasswordMatches(deref(req.NewPassword), user.Password) {
		return framework.Biz("新密码不能与旧密码一致")
	}
	hashed, err := framework.HashPassword(deref(req.NewPassword))
	if err != nil {
		return err
	}
	n, err := updatePassword(ctx, s.db, user.ID, user.Username, hashed)
	if err != nil {
		return err
	}
	if n == 0 {
		return framework.Biz("密码修改失败")
	}
	return nil
}

func (s *AuthService) currentUserRow(ctx context.Context) (*authUserRow, error) {
	user, err := selectUserByUsername(ctx, s.db, framework.CurrentUsername(ctx))
	if err != nil {
		return nil, err
	}
	if !user.enabled() {
		return nil, framework.BizCode(401, "未认证或登录已过期")
	}
	return user, nil
}

func buildRoutes(menus []menuRow) []*routeVO {
	sort.SliceStable(menus, func(i, j int) bool {
		a, b := menus[i], menus[j]
		if a.ParentID != b.ParentID {
			return a.ParentID < b.ParentID
		}
		if a.Sort != b.Sort {
			return a.Sort < b.Sort
		}
		return a.ID < b.ID
	})
	byID := make(map[int64]*routeVO, len(menus))
	order := make([]*routeVO, 0, len(menus))
	for _, m := range menus {
		r := &routeVO{ID: m.ID, ParentID: m.ParentID, Name: m.MenuName, Path: m.Path,
			Component: m.Component, Icon: m.Icon, Permission: m.Permission, Children: []*routeVO{}}
		byID[m.ID] = r
		order = append(order, r)
	}
	roots := []*routeVO{}
	for _, r := range order {
		if parent, ok := byID[r.ParentID]; ok {
			parent.Children = append(parent.Children, r)
		} else {
			roots = append(roots, r)
		}
	}
	return roots
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// blank 对应 @NotBlank：Hibernate 用 String.trim()，只去掉 <= U+0020 的字符。
func blank(s *string) bool {
	return s == nil || strings.TrimFunc(*s, func(r rune) bool { return r <= ' ' }) == ""
}

// ---------- AuthController ----------

// RegisterAuthRoutes 对应 AuthController。
func RegisterAuthRoutes(mux *http.ServeMux, d Deps, s *AuthService, captcha *framework.Captcha) {
	mux.Handle("GET /api/auth/captcha", framework.Handle(func(w http.ResponseWriter, r *http.Request) error {
		data, err := captcha.Generate(r.Context())
		if err != nil {
			return err
		}
		return framework.OK(w, r, data)
	}))

	mux.Handle("POST /api/auth/login", framework.Handle(func(w http.ResponseWriter, r *http.Request) error {
		var req loginRequest
		if err := framework.Bind(r, &req); err != nil {
			return err
		}
		if blank(req.Username) {
			return framework.Invalid("username", "不能为空")
		}
		if blank(req.Password) {
			return framework.Invalid("password", "不能为空")
		}
		if !captcha.Verify(r.Context(), req.CaptchaUUID, req.Captcha) {
			return framework.Biz("验证码错误或已过期")
		}
		resp, err := s.Login(r.Context(), req)
		if err != nil {
			return err
		}
		return framework.OK(w, r, resp)
	}))

	mux.Handle("POST /api/auth/logout", framework.Handle(func(w http.ResponseWriter, r *http.Request) error {
		if err := s.Logout(r.Context()); err != nil {
			return err
		}
		return framework.OK(w, r, nil)
	}))

	mux.Handle("GET /api/auth/me", framework.Handle(func(w http.ResponseWriter, r *http.Request) error {
		resp, err := s.CurrentUser(r.Context())
		if err != nil {
			return err
		}
		return framework.OK(w, r, resp)
	}))

	mux.Handle("GET /api/auth/routes", framework.Handle(func(w http.ResponseWriter, r *http.Request) error {
		resp, err := s.Routes(r.Context())
		if err != nil {
			return err
		}
		return framework.OK(w, r, resp)
	}))

	mux.Handle("PUT /api/auth/password", framework.Handle(func(w http.ResponseWriter, r *http.Request) error {
		var req changePasswordRequest
		if err := framework.Bind(r, &req); err != nil {
			return err
		}
		if blank(req.OldPassword) {
			return framework.Invalid("oldPassword", "不能为空")
		}
		if blank(req.NewPassword) {
			return framework.Invalid("newPassword", "不能为空")
		}
		if n := len(utf16.Encode([]rune(*req.NewPassword))); n < 6 || n > 64 { // @Size 按 String.length() 计 UTF-16 单元
			return framework.Invalid("newPassword", "长度必须在6到64位之间")
		}
		if err := s.ChangePassword(r.Context(), req); err != nil {
			return err
		}
		return framework.OK(w, r, nil)
	}, framework.OperLog(d.Recorder, "账号安全", "修改密码")))
}
