package platform

import (
	"encoding/json"
	"net/http"

	"scaffold/internal/framework"
)

// ---------- 管理接口：应用用户 ----------

func (s *svc) adminUsers(r *http.Request) (any, error) {
	q, err := framework.ParsePage(r)
	if err != nil {
		return nil, err
	}
	appID, err := s.resolveAppID(r)
	if err != nil {
		return nil, err
	}
	status, err := framework.QueryInt(r, "status")
	if err != nil {
		return nil, err
	}
	users, total, err := listUsers(r.Context(), s.db, appID, framework.QueryString(r, "keyword"), status, q.Offset(), q.NormalizedPageSize())
	if err != nil {
		return nil, err
	}
	views, err := s.userViews(r.Context(), appID, users, true)
	if err != nil {
		return nil, err
	}
	return framework.PageOf(views, total, q), nil
}

func (s *svc) adminExportUsers(r *http.Request) (string, error) {
	ctx := r.Context()
	appID, err := s.resolveAppID(r)
	if err != nil {
		return "", err
	}
	status, err := framework.QueryInt(r, "status")
	if err != nil {
		return "", err
	}
	users, _, err := listUsers(ctx, s.db, appID, framework.QueryString(r, "keyword"), status, 0, 5000)
	if err != nil {
		return "", err
	}
	fields, err := loadFields(ctx, s.db, appID, true)
	if err != nil {
		return "", err
	}
	ids := make([]int64, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	balances, err := loadCounters(ctx, s.db, appID, ids)
	if err != nil {
		return "", err
	}
	headers := []string{"账号", "昵称", "状态"}
	for _, f := range fields {
		headers = append(headers, f.Label)
	}
	headers = append(headers, "注册时间", "最近登录")
	data := make([][]any, 0, len(users))
	for _, u := range users {
		profile := profileView(fields, u.Profile)
		counters := countersView(fields, balances[u.ID])
		row := []any{u.Username, u.Nickname, userStatusText(u.Status)}
		for _, f := range fields {
			if f.Type == "counter" {
				row = append(row, counters[f.Key])
			} else {
				row = append(row, csvText(profile[f.Key]))
			}
		}
		data = append(data, append(row, u.CreateTime, u.LastLoginTime))
	}
	return csvOf(headers, data), nil
}

// adminPathUser 路径 {id} 的用户：不存在 → 用户不存在；所属应用不在范围内 → 403。
func (s *svc) adminPathUser(r *http.Request) (*appUser, error) {
	id, err := framework.PathInt64(r, "id")
	if err != nil {
		return nil, err
	}
	u, err := loadUserByID(r.Context(), s.db, id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, framework.Biz("用户不存在")
	}
	if err := s.requireApp(r.Context(), u.AppID); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *svc) adminUser(r *http.Request) (any, error) {
	u, err := s.adminPathUser(r)
	if err != nil {
		return nil, err
	}
	return s.userView(r.Context(), u, true)
}

func (s *svc) adminCreateUser(r *http.Request) (any, error) {
	ctx := r.Context()
	var req userWriteRequest
	if err := framework.Bind(r, &req); err != nil {
		return nil, err
	}
	if req.AppID == nil {
		return nil, framework.Invalid("appId", "不能为空")
	}
	if err := s.requireApp(ctx, *req.AppID); err != nil {
		return nil, err
	}
	app, err := loadApp(ctx, s.db, *req.AppID)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, framework.Biz("应用不存在")
	}
	in, err := s.bindUserInput(ctx, req)
	if err != nil {
		return nil, err
	}
	id, err := s.createUser(ctx, app.ID, in, profileCreate, "admin:"+framework.CurrentUsername(ctx), nil)
	if err != nil {
		return nil, err
	}
	u, err := loadAppUser(ctx, s.db, app.ID, id)
	if err != nil {
		return nil, err
	}
	return s.userView(ctx, u, true)
}

func (s *svc) adminUpdateUser(r *http.Request) (any, error) {
	u, err := s.adminPathUser(r)
	if err != nil {
		return nil, err
	}
	var req userWriteRequest
	if err := framework.Bind(r, &req); err != nil {
		return nil, err
	}
	return nil, s.updateProfile(r.Context(), u, req, profileAdminEdit, "admin:"+framework.CurrentUsername(r.Context()))
}

func (s *svc) adminUserStatus(r *http.Request) (any, error) {
	u, err := s.adminPathUser(r)
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
	return nil, s.setUserStatus(r.Context(), u, status, "admin:"+framework.CurrentUsername(r.Context()))
}

func (s *svc) adminResetPassword(r *http.Request) (any, error) {
	u, err := s.adminPathUser(r)
	if err != nil {
		return nil, err
	}
	var req passwordRequest
	if err := framework.Bind(r, &req); err != nil {
		return nil, err
	}
	password, err := validatePassword("password", req.Password)
	if err != nil {
		return nil, err
	}
	hashed, err := framework.HashPassword(password)
	if err != nil {
		return nil, err
	}
	if _, err := updateUserPassword(r.Context(), s.db, u.ID, hashed, "admin:"+framework.CurrentUsername(r.Context())); err != nil {
		return nil, err
	}
	return nil, s.revokeUserTokens(r.Context(), u.AppID, u.ID)
}

func (s *svc) adminDeleteUser(r *http.Request) (any, error) {
	u, err := s.adminPathUser(r)
	if err != nil {
		return nil, err
	}
	if _, err := softDeleteUser(r.Context(), s.db, u.ID, "admin:"+framework.CurrentUsername(r.Context())); err != nil {
		return nil, err
	}
	return nil, s.revokeUserTokens(r.Context(), u.AppID, u.ID)
}

func (s *svc) adminAdjustCounter(r *http.Request) (any, error) {
	ctx := r.Context()
	u, err := s.adminPathUser(r)
	if err != nil {
		return nil, err
	}
	field, err := loadCounterField(ctx, s.db, u.AppID, r.PathValue("key"))
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
	return s.changeCounter(ctx, field, u.ID, amount, reason, "admin-"+framework.UUIDHex(), "admin:"+framework.CurrentUsername(ctx))
}

// ---------- 管理接口：字段配置 ----------

func (s *svc) adminFields(r *http.Request) (any, error) {
	appID, err := s.resolveAppID(r)
	if err != nil {
		return nil, err
	}
	fields, err := loadFields(r.Context(), s.db, appID, false)
	if err != nil {
		return nil, err
	}
	out := make([]framework.Row, 0, len(fields))
	for _, f := range fields {
		out = append(out, f.view(true))
	}
	return out, nil
}

// bindFieldCommon 新增 / 修改共用：label、required、userEditable、options、defaultValue、sort、status、remark。
// cur 为修改时的现有定义（提供 key / type / 现有 options）。
func bindFieldCommon(req fieldWriteRequest, cur fieldDef) (fieldDef, error) {
	f := cur
	var err error
	if req.Label != nil || cur.ID == 0 {
		if f.Label, err = requireText("label", req.Label, 100); err != nil {
			return f, err
		}
	}
	if f.Required, err = flag("required", req.Required, cur.Required); err != nil {
		return f, err
	}
	if f.UserEditable, err = flag("userEditable", req.UserEditable, cur.UserEditable); err != nil {
		return f, err
	}
	if f.Type == "counter" {
		f.UserEditable = 0
	}
	if req.Options != nil || cur.ID == 0 || f.Type == "counter" && cur.Options == nil {
		if f.Options, err = validateOptions(f.Type, req.Options); err != nil {
			return f, err
		}
	}
	text, present, err := rawScalarText(req.DefaultValue)
	if err != nil {
		return f, err
	}
	if present || cur.ID == 0 {
		if f.DefaultValue, err = validateDefault(f, text); err != nil {
			return f, err
		}
	}
	if req.Sort != nil {
		f.Sort = *req.Sort
	}
	if f.Status, err = flag("status", req.Status, cur.Status); err != nil {
		return f, err
	}
	if err = checkLen("remark", req.Remark, 500); err != nil {
		return f, err
	}
	if req.Remark != nil {
		f.Remark = req.Remark
	}
	return f, nil
}

func (s *svc) adminCreateField(r *http.Request) (any, error) {
	ctx := r.Context()
	var req fieldWriteRequest
	if err := framework.Bind(r, &req); err != nil {
		return nil, err
	}
	if req.AppID == nil {
		return nil, framework.Invalid("appId", "不能为空")
	}
	if err := s.requireApp(ctx, *req.AppID); err != nil {
		return nil, err
	}
	app, err := loadApp(ctx, s.db, *req.AppID)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, framework.Biz("应用不存在")
	}
	if req.Key == nil || !fieldKeyRe.MatchString(*req.Key) {
		return nil, framework.Invalid("key", "1-64 位字母、数字或下划线，且以字母开头")
	}
	ftype := deref(req.Type)
	valid := false
	for _, t := range fieldTypes {
		valid = valid || t == ftype
	}
	if !valid {
		return nil, framework.Biz("字段类型不正确")
	}
	f, err := bindFieldCommon(req, fieldDef{AppID: app.ID, Key: *req.Key, Type: ftype, Status: 1})
	if err != nil {
		return nil, err
	}
	n, err := framework.SelectInt64(ctx, s.db, `SELECT COUNT(*) FROM app_field WHERE app_id = ? AND field_key = ?`, f.AppID, f.Key)
	if err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, framework.Biz("字段键已存在")
	}
	operator := framework.CurrentUsername(ctx)
	id, err := framework.Insert(ctx, s.db, `
		INSERT INTO app_field (app_id, field_key, label, field_type, required, user_editable, default_value, options, sort, status, remark, create_by, update_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.AppID, f.Key, f.Label, f.Type, f.Required, f.UserEditable, f.DefaultValue, f.Options, f.Sort, f.Status, f.Remark, operator, operator)
	if isDuplicate(err) {
		return nil, framework.Biz("字段键已存在")
	}
	if err != nil {
		return nil, err
	}
	return framework.Row{"id": id}, nil
}

// adminPathField 路径 {id} 的字段：不存在 → 字段不存在；应用范围外 → 403。
func (s *svc) adminPathField(r *http.Request) (*fieldDef, error) {
	id, err := framework.PathInt64(r, "id")
	if err != nil {
		return nil, err
	}
	f, err := loadField(r.Context(), s.db, id)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, framework.Biz("字段不存在")
	}
	if err := s.requireApp(r.Context(), f.AppID); err != nil {
		return nil, err
	}
	return f, nil
}

func (s *svc) adminUpdateField(r *http.Request) (any, error) {
	cur, err := s.adminPathField(r)
	if err != nil {
		return nil, err
	}
	var req fieldWriteRequest
	if err := framework.Bind(r, &req); err != nil {
		return nil, err
	}
	f, err := bindFieldCommon(req, *cur)
	if err != nil {
		return nil, err
	}
	_, err = framework.Exec(r.Context(), s.db, `
		UPDATE app_field
		SET label = ?, required = ?, user_editable = ?, default_value = ?, options = ?, sort = ?, status = ?, remark = ?,
		    update_by = ?, update_time = NOW()
		WHERE id = ?`,
		f.Label, f.Required, f.UserEditable, f.DefaultValue, f.Options, f.Sort, f.Status, f.Remark, framework.CurrentUsername(r.Context()), f.ID)
	return nil, err
}

func (s *svc) adminDeleteField(r *http.Request) (any, error) {
	f, err := s.adminPathField(r)
	if err != nil {
		return nil, err
	}
	_, err = framework.Exec(r.Context(), s.db, `DELETE FROM app_field WHERE id = ?`, f.ID)
	return nil, err
}

// ---------- 文本辅助 ----------

func userStatusText(status int) string {
	if status == 1 {
		return "正常"
	}
	return "停用"
}

func enabledText(v any) string {
	if framework.AsInt64(v) == 1 {
		return "启用"
	}
	return "停用"
}

// csvText profile 值的导出文本：布尔显示为 是 / 否，null 为空。
func csvText(v any) any {
	switch x := v.(type) {
	case nil:
		return ""
	case bool:
		if x {
			return "是"
		}
		return "否"
	case json.Number:
		return x.String()
	}
	return v
}
