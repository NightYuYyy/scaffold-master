package platform

import (
	"crypto/rand"
	"database/sql"
	"math/big"
	"net/http"

	"scaffold/internal/framework"
)

const appSelect = `
	SELECT a.id, a.app_code AS code, a.app_name AS name, a.status, a.allow_register AS allowRegister,
	       a.captcha_enabled AS captchaEnabled, a.token_expires_seconds AS tokenExpiresSeconds, a.remark,
	       (SELECT COUNT(*) FROM app_user u WHERE u.app_id = a.id AND u.deleted = 0) AS userCount,
	       (SELECT COUNT(*) FROM app_api_key k WHERE k.app_id = a.id) AS keyCount,
	       DATE_FORMAT(a.create_time, '%Y-%m-%d %H:%i') AS createTime
	FROM app_info a`

// appRows 当前账号可见的应用列表（keyword 匹配编码 / 名称）。
func (s *svc) appRows(r *http.Request, offset, limit int) ([]framework.Row, int64, error) {
	ctx := r.Context()
	status, err := framework.QueryInt(r, "status")
	if err != nil {
		return nil, 0, err
	}
	sc, err := s.scope(ctx)
	if err != nil {
		return nil, 0, err
	}
	where, args := sc.clause("a.id")
	where = " WHERE a.deleted = 0" + where
	if kw := like(framework.QueryString(r, "keyword")); kw != "" {
		where += " AND (a.app_code LIKE ? OR a.app_name LIKE ?)"
		args = append(args, kw, kw)
	}
	if status != nil {
		where += " AND a.status = ?"
		args = append(args, *status)
	}
	total, err := framework.SelectInt64(ctx, s.db, "SELECT COUNT(*) FROM app_info a"+where, args...)
	if err != nil {
		return nil, 0, err
	}
	rows, err := framework.SelectRows(ctx, s.db, appSelect+where+" ORDER BY a.id DESC LIMIT ? OFFSET ?", append(args, limit, offset)...)
	return rows, total, err
}

func (s *svc) adminApps(r *http.Request) (any, error) {
	q, err := framework.ParsePage(r)
	if err != nil {
		return nil, err
	}
	rows, total, err := s.appRows(r, q.Offset(), q.NormalizedPageSize())
	if err != nil {
		return nil, err
	}
	return framework.PageOf(rows, total, q), nil
}

func (s *svc) adminAppOptions(r *http.Request) (any, error) {
	sc, err := s.scope(r.Context())
	if err != nil {
		return nil, err
	}
	where, args := sc.clause("id")
	rows, err := framework.SelectRows(r.Context(), s.db, `SELECT id, app_code AS code, app_name AS name FROM app_info WHERE deleted = 0`+where+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *svc) adminExportApps(r *http.Request) (string, error) {
	rows, _, err := s.appRows(r, 0, 5000)
	if err != nil {
		return "", err
	}
	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, []any{row["code"], row["name"], enabledText(row["status"]), row["userCount"], row["keyCount"], row["createTime"]})
	}
	return csvOf([]string{"编码", "名称", "状态", "用户数", "密钥数", "创建时间"}, data), nil
}

// pathApp 路径 {id} 对应的应用：范围外 403，不存在 → 应用不存在。
func (s *svc) pathApp(r *http.Request) (*openApp, error) {
	id, err := framework.PathInt64(r, "id")
	if err != nil {
		return nil, err
	}
	if err := s.requireApp(r.Context(), id); err != nil {
		return nil, err
	}
	app, err := loadApp(r.Context(), s.db, id)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, framework.Biz("应用不存在")
	}
	return app, nil
}

func (s *svc) adminApp(r *http.Request) (any, error) {
	app, err := s.pathApp(r)
	if err != nil {
		return nil, err
	}
	return framework.SelectRow(r.Context(), s.db, appSelect+" WHERE a.id = ? AND a.deleted = 0", app.ID)
}

// bindApp 校验应用写请求的公共字段（code 由新增单独校验）。
func bindApp(req appWriteRequest) (name string, status, allowRegister, captchaEnabled *int, expires *int64, remark *string, err error) {
	if req.Name != nil {
		if name, err = requireText("name", req.Name, 100); err != nil {
			return
		}
	}
	for _, f := range []struct {
		name string
		v    *int
	}{{"status", req.Status}, {"allowRegister", req.AllowRegister}, {"captchaEnabled", req.CaptchaEnabled}} {
		if _, err = flag(f.name, f.v, 0); err != nil {
			return
		}
	}
	if req.TokenExpiresSeconds != nil && *req.TokenExpiresSeconds <= 0 {
		err = framework.Invalid("tokenExpiresSeconds", "必须大于 0")
		return
	}
	if err = checkLen("remark", req.Remark, 500); err != nil {
		return
	}
	return name, req.Status, req.AllowRegister, req.CaptchaEnabled, req.TokenExpiresSeconds, req.Remark, nil
}

func intOr(v *int, def int) int {
	if v == nil {
		return def
	}
	return *v
}

func (s *svc) adminCreateApp(r *http.Request) (any, error) {
	ctx := r.Context()
	var req appWriteRequest
	if err := framework.Bind(r, &req); err != nil {
		return nil, err
	}
	if req.Code == nil || !appCodeRe.MatchString(*req.Code) {
		return nil, framework.Invalid("code", "3-64 位小写字母、数字或下划线")
	}
	if blank(req.Name) {
		return nil, framework.Invalid("name", "不能为空")
	}
	name, status, allowRegister, captchaEnabled, expires, remark, err := bindApp(req)
	if err != nil {
		return nil, err
	}
	n, err := framework.SelectInt64(ctx, s.db, `SELECT COUNT(*) FROM app_info WHERE app_code = ?`, *req.Code)
	if err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, framework.Biz("应用编码已存在")
	}
	super, err := s.isSuperAdmin(ctx)
	if err != nil {
		return nil, err
	}
	operator := framework.CurrentUsername(ctx)
	tokenExpires := int64(604800)
	if expires != nil {
		tokenExpires = *expires
	}
	var id int64
	err = framework.Tx(ctx, s.db, func(tx *sql.Tx) error {
		var err error
		id, err = framework.Insert(ctx, tx, `
			INSERT INTO app_info (app_code, app_name, status, allow_register, captcha_enabled, token_expires_seconds, remark, create_by, update_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			*req.Code, name, intOr(status, 1), intOr(allowRegister, 1), intOr(captchaEnabled, 1), tokenExpires, remark, operator, operator)
		if err != nil {
			return err
		}
		// 非超级管理员创建的应用自动绑定到创建者，否则创建后自己看不到
		if !super {
			_, err = framework.Exec(ctx, tx, `INSERT IGNORE INTO sys_user_app (user_id, app_id) SELECT id, ? FROM sys_user WHERE username = ? AND deleted = 0`, id, operator)
		}
		return err
	})
	if isDuplicate(err) {
		return nil, framework.Biz("应用编码已存在")
	}
	if err != nil {
		return nil, err
	}
	return framework.Row{"id": id}, nil
}

func (s *svc) adminUpdateApp(r *http.Request) (any, error) {
	app, err := s.pathApp(r)
	if err != nil {
		return nil, err
	}
	var req appWriteRequest
	if err := framework.Bind(r, &req); err != nil {
		return nil, err
	}
	name, status, allowRegister, captchaEnabled, expires, remark, err := bindApp(req)
	if err != nil {
		return nil, err
	}
	var namePtr *string
	if req.Name != nil {
		namePtr = &name
	}
	_, err = framework.Exec(r.Context(), s.db, `
		UPDATE app_info
		SET app_name = COALESCE(?, app_name), status = COALESCE(?, status), allow_register = COALESCE(?, allow_register),
		    captcha_enabled = COALESCE(?, captcha_enabled), token_expires_seconds = COALESCE(?, token_expires_seconds),
		    remark = COALESCE(?, remark), update_by = ?, update_time = NOW()
		WHERE id = ? AND deleted = 0`,
		namePtr, status, allowRegister, captchaEnabled, expires, remark, framework.CurrentUsername(r.Context()), app.ID)
	return nil, err
}

func (s *svc) adminAppStatus(r *http.Request) (any, error) {
	app, err := s.pathApp(r)
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
	_, err = framework.Exec(r.Context(), s.db, `UPDATE app_info SET status = ?, update_by = ?, update_time = NOW() WHERE id = ? AND deleted = 0`,
		status, framework.CurrentUsername(r.Context()), app.ID)
	return nil, err
}

func (s *svc) adminDeleteApp(r *http.Request) (any, error) {
	app, err := s.pathApp(r)
	if err != nil {
		return nil, err
	}
	n, err := framework.SelectInt64(r.Context(), s.db, `SELECT COUNT(*) FROM app_user WHERE app_id = ? AND deleted = 0`, app.ID)
	if err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, framework.Biz("应用下仍有用户，不能删除")
	}
	_, err = framework.Exec(r.Context(), s.db, `UPDATE app_info SET deleted = 1, update_by = ?, update_time = NOW() WHERE id = ? AND deleted = 0`,
		framework.CurrentUsername(r.Context()), app.ID)
	return nil, err
}

// ---------- 接口密钥 ----------

const (
	lowerAlnum = "abcdefghijklmnopqrstuvwxyz0123456789"
	mixedAlnum = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

func randString(alphabet string, n int) (string, error) {
	b := make([]byte, n)
	limit := big.NewInt(int64(len(alphabet)))
	for i := range b {
		index, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		b[i] = alphabet[index.Int64()]
	}
	return string(b), nil
}

// maskSecret 形如 demo-se****6789：前 7 位 + **** + 后 4 位。
func maskSecret(s string) string {
	if len(s) <= 11 {
		return s[:min(7, len(s))] + "****"
	}
	return s[:7] + "****" + s[len(s)-4:]
}

func (s *svc) adminKeys(r *http.Request) (any, error) {
	app, err := s.pathApp(r)
	if err != nil {
		return nil, err
	}
	rows, err := framework.SelectRows(r.Context(), s.db, `
		SELECT id, key_name AS name, api_key AS apiKey, api_secret, status,
		       DATE_FORMAT(last_used_time, '%Y-%m-%d %H:%i') AS lastUsedTime,
		       DATE_FORMAT(create_time, '%Y-%m-%d %H:%i') AS createTime
		FROM app_api_key WHERE app_id = ? ORDER BY id DESC`, app.ID)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		row["secretMasked"] = maskSecret(framework.StrOrEmpty(row["api_secret"]))
		delete(row, "api_secret")
	}
	return rows, nil
}

func (s *svc) adminCreateKey(r *http.Request) (any, error) {
	app, err := s.pathApp(r)
	if err != nil {
		return nil, err
	}
	var req keyCreateRequest
	if err := framework.Bind(r, &req); err != nil {
		return nil, err
	}
	name, err := requireText("name", req.Name, 100)
	if err != nil {
		return nil, err
	}
	suffix, err := randString(lowerAlnum, 16)
	if err != nil {
		return nil, err
	}
	apiKey := app.Code + "_" + suffix
	secret, err := randString(mixedAlnum, 48)
	if err != nil {
		return nil, err
	}
	operator := framework.CurrentUsername(r.Context())
	id, err := framework.Insert(r.Context(), s.db, `
		INSERT INTO app_api_key (app_id, key_name, api_key, api_secret, status, create_by, update_by)
		VALUES (?, ?, ?, ?, 1, ?, ?)`, app.ID, name, apiKey, secret, operator, operator)
	if err != nil {
		return nil, err
	}
	return framework.Row{"id": id, "apiKey": apiKey, "apiSecret": secret}, nil
}

func (s *svc) adminKeyStatus(r *http.Request) (any, error) {
	app, err := s.pathApp(r)
	if err != nil {
		return nil, err
	}
	keyID, err := framework.PathInt64(r, "keyId")
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
	n, err := framework.Exec(r.Context(), s.db, `UPDATE app_api_key SET status = ?, update_by = ?, update_time = NOW() WHERE id = ? AND app_id = ?`,
		status, framework.CurrentUsername(r.Context()), keyID, app.ID)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, framework.Biz("密钥不存在")
	}
	return nil, nil
}

func (s *svc) adminDeleteKey(r *http.Request) (any, error) {
	app, err := s.pathApp(r)
	if err != nil {
		return nil, err
	}
	keyID, err := framework.PathInt64(r, "keyId")
	if err != nil {
		return nil, err
	}
	n, err := framework.Exec(r.Context(), s.db, `DELETE FROM app_api_key WHERE id = ? AND app_id = ?`, keyID, app.ID)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, framework.Biz("密钥不存在")
	}
	return nil, nil
}
