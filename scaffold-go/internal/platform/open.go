package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"scaffold/internal/framework"
)

// ---------- 用户视图 ----------

// buildUserView 组装 /me 形态；admin 为 true 时附带 appId 与 registerIp。
func buildUserView(fields []fieldDef, u *appUser, balances map[string]json.Number, admin bool) framework.Row {
	row := framework.Row{
		"id": u.ID, "username": u.Username, "nickname": u.Nickname, "avatar": u.Avatar, "status": u.Status,
		"profile": profileView(fields, u.Profile), "counters": countersView(fields, balances),
		"createTime": u.CreateTime, "lastLoginTime": u.LastLoginTime,
	}
	if admin {
		row["appId"], row["registerIp"] = u.AppID, u.RegisterIP
	}
	return row
}

// userViews 批量组装同一应用用户的视图。
func (s *svc) userViews(ctx context.Context, appID int64, users []*appUser, admin bool) ([]framework.Row, error) {
	fields, err := loadFields(ctx, s.db, appID, true)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	balances, err := loadCounters(ctx, s.db, appID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]framework.Row, 0, len(users))
	for _, u := range users {
		out = append(out, buildUserView(fields, u, balances[u.ID], admin))
	}
	return out, nil
}

func (s *svc) userView(ctx context.Context, u *appUser, admin bool) (framework.Row, error) {
	views, err := s.userViews(ctx, u.AppID, []*appUser{u}, admin)
	if err != nil {
		return nil, err
	}
	return views[0], nil
}

// ---------- 建用户 / 改资料（注册、服务端、后台共用） ----------

type userInput struct {
	username string
	password string
	nickname *string
	avatar   *string
	status   int
	profile  json.RawMessage
}

// createUser 校验 profile（含必填）、账号唯一后写入。
func (s *svc) createUser(ctx context.Context, appID int64, in userInput, mode profileMode, createBy string, ip *string) (int64, error) {
	profileIn, err := decodeProfile(in.profile)
	if err != nil {
		return 0, err
	}
	fields, err := loadFields(ctx, s.db, appID, true)
	if err != nil {
		return 0, err
	}
	profile, err := applyProfile(fields, nil, profileIn, mode)
	if err != nil {
		return 0, err
	}
	if n, err := countUsername(ctx, s.db, appID, in.username); err != nil || n > 0 {
		if err != nil {
			return 0, err
		}
		return 0, framework.Biz("账号已存在")
	}
	hashed, err := framework.HashPassword(in.password)
	if err != nil {
		return 0, err
	}
	nickname := in.username
	if !blank(in.nickname) {
		nickname = *in.nickname
	}
	profileJSON, err := marshalProfile(profile)
	if err != nil {
		return 0, err
	}
	id, err := insertUser(ctx, s.db, appID, in.username, hashed, nickname, nullIfEmpty(in.avatar), in.status, profileJSON, ip, createBy)
	if isDuplicate(err) {
		return 0, framework.Biz("账号已存在")
	}
	return id, err
}

// updateProfile 改昵称 / 头像 / profile；mode 决定字段可改范围。
func (s *svc) updateProfile(ctx context.Context, u *appUser, req userWriteRequest, mode profileMode, operator string) error {
	if req.Nickname != nil && blank(req.Nickname) {
		return framework.Invalid("nickname", "不能为空")
	}
	if err := checkLen("nickname", req.Nickname, 64); err != nil {
		return err
	}
	if err := checkLen("avatar", req.Avatar, 500); err != nil {
		return err
	}
	profileIn, err := decodeProfile(req.Profile)
	if err != nil {
		return err
	}
	var profileJSON *string
	if profileIn != nil {
		fields, err := loadFields(ctx, s.db, u.AppID, true)
		if err != nil {
			return err
		}
		merged, err := applyProfile(fields, u.Profile, profileIn, mode)
		if err != nil {
			return err
		}
		js, err := marshalProfile(merged)
		if err != nil {
			return err
		}
		profileJSON = &js
	}
	_, err = updateUser(ctx, s.db, u.ID, req.Nickname, req.Avatar, profileJSON, operator)
	return err
}

// setUserStatus 启停用户；停用撤销其全部令牌。
func (s *svc) setUserStatus(ctx context.Context, u *appUser, status int, operator string) error {
	n, err := updateUserStatus(ctx, s.db, u.ID, status, operator)
	if err != nil {
		return err
	}
	if n == 0 {
		return framework.Biz("用户不存在")
	}
	if status == 0 {
		return s.revokeUserTokens(ctx, u.AppID, u.ID)
	}
	return nil
}

// ---------- 开放接口：App-Key ----------

func (s *svc) openCaptcha(r *http.Request) (any, error) {
	return s.captcha.Generate(r.Context())
}

func (s *svc) openFields(r *http.Request) (any, error) {
	fields, err := loadFields(r.Context(), s.db, openOf(r.Context()).app.ID, true)
	if err != nil {
		return nil, err
	}
	out := make([]framework.Row, 0, len(fields))
	for _, f := range fields {
		out = append(out, f.view(false))
	}
	return out, nil
}

var authEventMsg = map[string]string{"register": "注册成功", "login": "登录成功", "logout": "退出登录", "refresh": "刷新令牌"}

// loginResult 签发令牌、写认证日志并返回 { token, expiresIn, user }。
func (s *svc) loginResult(ctx context.Context, oc *openCtx, userID int64, event, ip string) (any, error) {
	if event != "refresh" {
		if err := touchLastLogin(ctx, s.db, userID, ip); err != nil {
			return nil, err
		}
	}
	token, err := s.issueToken(ctx, oc.app, oc.key, userID)
	if err != nil {
		return nil, err
	}
	u, err := loadAppUser(ctx, s.db, oc.app.ID, userID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, framework.ErrUnauthorized
	}
	insertAuthLog(ctx, s.db, oc.app.ID, &u.ID, &u.Username, event, 1, ip, authEventMsg[event])
	view, err := s.userView(ctx, u, false)
	if err != nil {
		return nil, err
	}
	return framework.Row{"token": token, "expiresIn": oc.app.TokenExpires, "user": view}, nil
}

func (s *svc) openRegister(r *http.Request) (any, error) {
	ctx, oc := r.Context(), openOf(r.Context())
	var req userWriteRequest
	if err := framework.Bind(r, &req); err != nil {
		return nil, err
	}
	username, err := validateUsername(req.Username)
	if err != nil {
		return nil, err
	}
	password, err := validatePassword("password", req.Password)
	if err != nil {
		return nil, err
	}
	if err := checkLen("nickname", req.Nickname, 64); err != nil {
		return nil, err
	}
	ip := framework.ClientIP(r)
	fail := func(err error) error {
		var e *framework.Error
		if errors.As(err, &e) {
			insertAuthLog(ctx, s.db, oc.app.ID, nil, &username, "register_failed", 0, ip, e.Message)
		}
		return err
	}
	if oc.app.AllowRegister != 1 {
		return nil, fail(framework.Biz("该应用未开放注册"))
	}
	if oc.app.CaptchaEnabled == 1 && !s.captcha.Verify(ctx, req.CaptchaUUID, req.Captcha) {
		return nil, fail(framework.Biz("验证码错误或已过期"))
	}
	id, err := s.createUser(ctx, oc.app.ID, userInput{username: username, password: password, nickname: req.Nickname, status: 1, profile: req.Profile}, profileRegister, "register", &ip)
	if err != nil {
		return nil, fail(err)
	}
	return s.loginResult(ctx, oc, id, "register", ip)
}

func (s *svc) openLogin(r *http.Request) (any, error) {
	ctx, oc := r.Context(), openOf(r.Context())
	var req userWriteRequest
	if err := framework.Bind(r, &req); err != nil {
		return nil, err
	}
	if blank(req.Username) {
		return nil, framework.Invalid("username", "不能为空")
	}
	if blank(req.Password) {
		return nil, framework.Invalid("password", "不能为空")
	}
	username, ip := *req.Username, framework.ClientIP(r)
	fail := func(userID *int64, msg string) error {
		insertAuthLog(ctx, s.db, oc.app.ID, userID, &username, "login_failed", 0, ip, msg)
		return framework.Biz(msg)
	}
	if oc.app.CaptchaEnabled == 1 && !s.captcha.Verify(ctx, req.CaptchaUUID, req.Captcha) {
		return nil, fail(nil, "验证码错误或已过期")
	}
	u, err := loadUserByUsername(ctx, s.db, oc.app.ID, username)
	if err != nil {
		return nil, err
	}
	if u == nil || u.Password == nil || !framework.PasswordMatches(*req.Password, *u.Password) {
		var userID *int64
		if u != nil {
			userID = &u.ID
		}
		return nil, fail(userID, "账号或密码错误")
	}
	if u.Status != 1 {
		return nil, fail(&u.ID, "账号已停用")
	}
	return s.loginResult(ctx, oc, u.ID, "login", ip)
}

// ---------- 开放接口：Bearer ----------

func (s *svc) openLogout(r *http.Request) (any, error) {
	ctx, oc := r.Context(), openOf(r.Context())
	if err := s.revokeToken(ctx, oc.app.ID, oc.user.ID, oc.jti); err != nil {
		return nil, err
	}
	insertAuthLog(ctx, s.db, oc.app.ID, &oc.user.ID, &oc.user.Username, "logout", 1, framework.ClientIP(r), authEventMsg["logout"])
	return nil, nil
}

func (s *svc) openRefresh(r *http.Request) (any, error) {
	ctx, oc := r.Context(), openOf(r.Context())
	if err := s.revokeToken(ctx, oc.app.ID, oc.user.ID, oc.jti); err != nil {
		return nil, err
	}
	return s.loginResult(ctx, oc, oc.user.ID, "refresh", framework.ClientIP(r))
}

func (s *svc) openMe(r *http.Request) (any, error) {
	return s.userView(r.Context(), openOf(r.Context()).user, false)
}

func (s *svc) openUpdateMe(r *http.Request) (any, error) {
	oc := openOf(r.Context())
	var req userWriteRequest
	if err := framework.Bind(r, &req); err != nil {
		return nil, err
	}
	return nil, s.updateProfile(r.Context(), oc.user, req, profileUserEdit, "user:"+oc.user.Username)
}

func (s *svc) openChangePassword(r *http.Request) (any, error) {
	ctx, oc := r.Context(), openOf(r.Context())
	var req changePasswordRequest
	if err := framework.Bind(r, &req); err != nil {
		return nil, err
	}
	if blank(req.OldPassword) {
		return nil, framework.Invalid("oldPassword", "不能为空")
	}
	newPassword, err := validatePassword("newPassword", req.NewPassword)
	if err != nil {
		return nil, err
	}
	u := oc.user
	if u.Password == nil || !framework.PasswordMatches(*req.OldPassword, *u.Password) {
		return nil, framework.Biz("旧密码不正确")
	}
	if framework.PasswordMatches(newPassword, *u.Password) {
		return nil, framework.Biz("新密码不能与旧密码一致")
	}
	hashed, err := framework.HashPassword(newPassword)
	if err != nil {
		return nil, err
	}
	if _, err := updateUserPassword(ctx, s.db, u.ID, hashed, "user:"+u.Username); err != nil {
		return nil, err
	}
	return nil, s.revokeUserTokens(ctx, u.AppID, u.ID)
}

// ---------- 开放接口：App-Key + Secret ----------

func (s *svc) openIntrospect(r *http.Request) (any, error) {
	ctx, oc := r.Context(), openOf(r.Context())
	var req introspectRequest
	if err := framework.Bind(r, &req); err != nil {
		return nil, err
	}
	inactive := framework.Row{"active": false, "userId": nil, "username": nil, "expiresAt": nil}
	if blank(req.Token) {
		return inactive, nil
	}
	info, err := s.parseToken(ctx, *req.Token)
	if err != nil {
		var e *framework.Error
		if errors.As(err, &e) {
			return inactive, nil
		}
		return nil, err
	}
	if info.app.ID != oc.app.ID {
		return inactive, nil
	}
	u, err := loadAppUser(ctx, s.db, oc.app.ID, info.userID)
	if err != nil {
		return nil, err
	}
	if u == nil || u.Status != 1 {
		return inactive, nil
	}
	return framework.Row{"active": true, "userId": u.ID, "username": u.Username, "expiresAt": info.exp.Format("2006-01-02 15:04:05")}, nil
}

func (s *svc) openUsers(r *http.Request) (any, error) {
	oc := openOf(r.Context())
	q, err := framework.ParsePage(r)
	if err != nil {
		return nil, err
	}
	status, err := framework.QueryInt(r, "status")
	if err != nil {
		return nil, err
	}
	users, total, err := listUsers(r.Context(), s.db, oc.app.ID, framework.QueryString(r, "keyword"), status, q.Offset(), q.NormalizedPageSize())
	if err != nil {
		return nil, err
	}
	views, err := s.userViews(r.Context(), oc.app.ID, users, false)
	if err != nil {
		return nil, err
	}
	return framework.PageOf(views, total, q), nil
}

// pathUser 取路径 {id} 对应的本应用用户。
func (s *svc) pathUser(r *http.Request) (*appUser, error) {
	id, err := framework.PathInt64(r, "id")
	if err != nil {
		return nil, err
	}
	u, err := loadAppUser(r.Context(), s.db, openOf(r.Context()).app.ID, id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, framework.Biz("用户不存在")
	}
	return u, nil
}

func (s *svc) openUser(r *http.Request) (any, error) {
	u, err := s.pathUser(r)
	if err != nil {
		return nil, err
	}
	return s.userView(r.Context(), u, false)
}

// bindUserInput 服务端 / 后台建用户的公共校验；password 缺省取参数 app.user.initPassword。
func (s *svc) bindUserInput(ctx context.Context, req userWriteRequest) (userInput, error) {
	username, err := validateUsername(req.Username)
	if err != nil {
		return userInput{}, err
	}
	var password string
	if blank(req.Password) {
		if password, err = initPassword(ctx, s.db); err != nil {
			return userInput{}, err
		}
	} else if password, err = validatePassword("password", req.Password); err != nil {
		return userInput{}, err
	}
	if err := checkLen("nickname", req.Nickname, 64); err != nil {
		return userInput{}, err
	}
	if err := checkLen("avatar", req.Avatar, 500); err != nil {
		return userInput{}, err
	}
	status, err := flag("status", req.Status, 1)
	if err != nil {
		return userInput{}, err
	}
	return userInput{username: username, password: password, nickname: req.Nickname, avatar: req.Avatar, status: status, profile: req.Profile}, nil
}

func (s *svc) openCreateUser(r *http.Request) (any, error) {
	ctx, oc := r.Context(), openOf(r.Context())
	var req userWriteRequest
	if err := framework.Bind(r, &req); err != nil {
		return nil, err
	}
	in, err := s.bindUserInput(ctx, req)
	if err != nil {
		return nil, err
	}
	id, err := s.createUser(ctx, oc.app.ID, in, profileCreate, "api:"+oc.key.APIKey, nil)
	if err != nil {
		return nil, err
	}
	u, err := loadAppUser(ctx, s.db, oc.app.ID, id)
	if err != nil {
		return nil, err
	}
	return s.userView(ctx, u, false)
}

func (s *svc) openUpdateProfile(r *http.Request) (any, error) {
	u, err := s.pathUser(r)
	if err != nil {
		return nil, err
	}
	var req userWriteRequest
	if err := framework.Bind(r, &req); err != nil {
		return nil, err
	}
	return nil, s.updateProfile(r.Context(), u, req, profileAdminEdit, "api:"+openOf(r.Context()).key.APIKey)
}

func (s *svc) openUserStatus(r *http.Request) (any, error) {
	u, err := s.pathUser(r)
	if err != nil {
		return nil, err
	}
	var req statusRequest
	if err := framework.Bind(r, &req); err != nil {
		return nil, err
	}
	status, err := requireStatus(req.Status)
	if err != nil {
		return nil, err
	}
	return nil, s.setUserStatus(r.Context(), u, status, "api:"+openOf(r.Context()).key.APIKey)
}

func (s *svc) openUserCounters(r *http.Request) (any, error) {
	u, err := s.pathUser(r)
	if err != nil {
		return nil, err
	}
	view, err := s.userView(r.Context(), u, false)
	if err != nil {
		return nil, err
	}
	return view["counters"], nil
}

func (s *svc) openChangeCounter(r *http.Request) (any, error) {
	ctx, oc := r.Context(), openOf(r.Context())
	u, err := s.pathUser(r)
	if err != nil {
		return nil, err
	}
	field, err := loadCounterField(ctx, s.db, oc.app.ID, r.PathValue("key"))
	if err != nil {
		return nil, err
	}
	if field == nil {
		return nil, framework.Biz("账户不存在")
	}
	var req counterChangeRequest
	if err := framework.Bind(r, &req); err != nil {
		return nil, err
	}
	amount, err := parseAmount(req.Amount)
	if err != nil {
		return nil, err
	}
	reason, err := requireText("reason", req.Reason, 200)
	if err != nil {
		return nil, err
	}
	refNo, err := requireText("refNo", req.RefNo, 64)
	if err != nil {
		return nil, err
	}
	return s.changeCounter(ctx, field, u.ID, amount, reason, refNo, "api:"+oc.key.APIKey)
}

func (s *svc) openCounterLogs(r *http.Request) (any, error) {
	u, err := s.pathUser(r)
	if err != nil {
		return nil, err
	}
	q, err := framework.ParsePage(r)
	if err != nil {
		return nil, err
	}
	key := r.PathValue("key")
	rows, total, err := s.counterLogs(r.Context(), counterLogFilter{appID: u.AppID, userID: &u.ID, fieldKey: &key}, q.Offset(), q.NormalizedPageSize())
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		delete(row, "appId")
		delete(row, "userId")
		delete(row, "username")
	}
	return framework.PageOf(rows, total, q), nil
}
