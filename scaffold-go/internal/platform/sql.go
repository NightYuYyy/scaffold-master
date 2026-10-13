package platform

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"scaffold/internal/framework"
)

// ---------- 应用与密钥 ----------

// openApp 应用信息（开放接口认证与管理接口共用）。
type openApp struct {
	ID             int64
	Code           string
	Name           string
	Status         int
	AllowRegister  int
	CaptchaEnabled int
	TokenExpires   int64
}

// openKey 接口密钥。
type openKey struct {
	ID     int64
	AppID  int64
	APIKey string
	Secret string
	Status int
}

func scanApp(row framework.Row) *openApp {
	return &openApp{
		ID:             framework.AsInt64(row["id"]),
		Code:           framework.StrOrEmpty(row["app_code"]),
		Name:           framework.StrOrEmpty(row["app_name"]),
		Status:         int(framework.AsInt64(row["status"])),
		AllowRegister:  int(framework.AsInt64(row["allow_register"])),
		CaptchaEnabled: int(framework.AsInt64(row["captcha_enabled"])),
		TokenExpires:   framework.AsInt64(row["token_expires_seconds"]),
	}
}

func loadApp(ctx context.Context, q framework.Querier, id int64) (*openApp, error) {
	row, err := framework.SelectRow(ctx, q, `
		SELECT id, app_code, app_name, status, allow_register, captcha_enabled, token_expires_seconds
		FROM app_info WHERE id = ? AND deleted = 0`, id)
	if err != nil || row == nil {
		return nil, err
	}
	return scanApp(row), nil
}

// loadKey 按 api_key 取密钥与所属应用；应用已删除视为不存在，状态由调用方判断。
func loadKey(ctx context.Context, q framework.Querier, apiKey string) (*openApp, *openKey, error) {
	row, err := framework.SelectRow(ctx, q, `
		SELECT k.id AS key_id, k.app_id, k.api_key, k.api_secret, k.status AS key_status,
		       a.id, a.app_code, a.app_name, a.status, a.allow_register, a.captcha_enabled, a.token_expires_seconds
		FROM app_api_key k
		JOIN app_info a ON a.id = k.app_id
		WHERE k.api_key = ? AND a.deleted = 0`, apiKey)
	if err != nil || row == nil {
		return nil, nil, err
	}
	key := &openKey{
		ID:     framework.AsInt64(row["key_id"]),
		AppID:  framework.AsInt64(row["app_id"]),
		APIKey: framework.StrOrEmpty(row["api_key"]),
		Secret: framework.StrOrEmpty(row["api_secret"]),
		Status: int(framework.AsInt64(row["key_status"])),
	}
	return scanApp(row), key, nil
}

// ---------- 应用用户 ----------

type appUser struct {
	ID            int64
	AppID         int64
	Username      string
	Password      *string
	Nickname      string
	Avatar        *string
	Status        int
	Profile       map[string]any
	RegisterIP    *string
	LastLoginTime *string
	CreateTime    *string
}

const userSelect = `
	SELECT id, app_id, username, password, nickname, avatar, status, profile, register_ip,
	       DATE_FORMAT(last_login_time, '%Y-%m-%d %H:%i') AS last_login_time,
	       DATE_FORMAT(create_time, '%Y-%m-%d %H:%i') AS create_time
	FROM app_user`

func scanUser(row framework.Row) *appUser {
	return &appUser{
		ID:            framework.AsInt64(row["id"]),
		AppID:         framework.AsInt64(row["app_id"]),
		Username:      framework.StrOrEmpty(row["username"]),
		Password:      strPtr(row["password"]),
		Nickname:      framework.StrOrEmpty(row["nickname"]),
		Avatar:        strPtr(row["avatar"]),
		Status:        int(framework.AsInt64(row["status"])),
		Profile:       decodeJSONObject(row["profile"]),
		RegisterIP:    strPtr(row["register_ip"]),
		LastLoginTime: strPtr(row["last_login_time"]),
		CreateTime:    strPtr(row["create_time"]),
	}
}

func strPtr(v any) *string {
	if v == nil {
		return nil
	}
	s := framework.StrOrEmpty(v)
	return &s
}

// decodeJSONObject 解析 JSON 列（数字保留为 json.Number）；NULL 或非法时返回空 map。
func decodeJSONObject(v any) map[string]any {
	out := map[string]any{}
	if v == nil {
		return out
	}
	dec := json.NewDecoder(strings.NewReader(framework.StrOrEmpty(v)))
	dec.UseNumber()
	_ = dec.Decode(&out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}

func loadUserByID(ctx context.Context, q framework.Querier, id int64) (*appUser, error) {
	row, err := framework.SelectRow(ctx, q, userSelect+` WHERE id = ? AND deleted = 0`, id)
	if err != nil || row == nil {
		return nil, err
	}
	return scanUser(row), nil
}

func loadAppUser(ctx context.Context, q framework.Querier, appID, id int64) (*appUser, error) {
	row, err := framework.SelectRow(ctx, q, userSelect+` WHERE app_id = ? AND id = ? AND deleted = 0`, appID, id)
	if err != nil || row == nil {
		return nil, err
	}
	return scanUser(row), nil
}

func loadUserByUsername(ctx context.Context, q framework.Querier, appID int64, username string) (*appUser, error) {
	row, err := framework.SelectRow(ctx, q, userSelect+` WHERE app_id = ? AND username = ? AND deleted = 0`, appID, username)
	if err != nil || row == nil {
		return nil, err
	}
	return scanUser(row), nil
}

// countUsername 含已软删除的记录：唯一键 (app_id, username) 覆盖全部行。
func countUsername(ctx context.Context, q framework.Querier, appID int64, username string) (int64, error) {
	return framework.SelectInt64(ctx, q, `SELECT COUNT(*) FROM app_user WHERE app_id = ? AND username = ?`, appID, username)
}

// listUsers 分页查询某应用的用户（keyword 模糊匹配账号 / 昵称）。
func listUsers(ctx context.Context, q framework.Querier, appID int64, keyword *string, status *int, offset, limit int) ([]*appUser, int64, error) {
	where := " WHERE app_id = ? AND deleted = 0"
	args := []any{appID}
	if kw := like(keyword); kw != "" {
		where += " AND (username LIKE ? OR nickname LIKE ?)"
		args = append(args, kw, kw)
	}
	if status != nil {
		where += " AND status = ?"
		args = append(args, *status)
	}
	total, err := framework.SelectInt64(ctx, q, "SELECT COUNT(*) FROM app_user"+where, args...)
	if err != nil {
		return nil, 0, err
	}
	rows, err := framework.SelectRows(ctx, q, userSelect+where+" ORDER BY id DESC LIMIT ? OFFSET ?", append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	users := make([]*appUser, 0, len(rows))
	for _, row := range rows {
		users = append(users, scanUser(row))
	}
	return users, total, nil
}

func insertUser(ctx context.Context, q framework.Querier, appID int64, username, hashed, nickname string, avatar *string, status int, profile string, ip *string, createBy string) (int64, error) {
	return framework.Insert(ctx, q, `
		INSERT INTO app_user (app_id, username, password, nickname, avatar, status, profile, register_ip, create_by, update_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, appID, username, hashed, nickname, avatar, status, profile, ip, createBy, createBy)
}

// updateUser 只更新给出的列（nil 表示不改）。
func updateUser(ctx context.Context, q framework.Querier, id int64, nickname, avatar, profile *string, operator string) (int64, error) {
	sets := []string{"update_by = ?", "update_time = NOW()"}
	args := []any{operator}
	if nickname != nil {
		sets = append(sets, "nickname = ?")
		args = append(args, strings.TrimSpace(*nickname))
	}
	if avatar != nil {
		sets = append(sets, "avatar = ?")
		args = append(args, nullIfEmpty(avatar))
	}
	if profile != nil {
		sets = append(sets, "profile = ?")
		args = append(args, *profile)
	}
	args = append(args, id)
	return framework.Exec(ctx, q, "UPDATE app_user SET "+strings.Join(sets, ", ")+" WHERE id = ? AND deleted = 0", args...)
}

func updateUserStatus(ctx context.Context, q framework.Querier, id int64, status int, operator string) (int64, error) {
	return framework.Exec(ctx, q, `UPDATE app_user SET status = ?, update_by = ?, update_time = NOW() WHERE id = ? AND deleted = 0`, status, operator, id)
}

func updateUserPassword(ctx context.Context, q framework.Querier, id int64, hashed, operator string) (int64, error) {
	return framework.Exec(ctx, q, `UPDATE app_user SET password = ?, update_by = ?, update_time = NOW() WHERE id = ? AND deleted = 0`, hashed, operator, id)
}

func softDeleteUser(ctx context.Context, q framework.Querier, id int64, operator string) (int64, error) {
	return framework.Exec(ctx, q, `UPDATE app_user SET deleted = 1, update_by = ?, update_time = NOW() WHERE id = ? AND deleted = 0`, operator, id)
}

func touchLastLogin(ctx context.Context, q framework.Querier, id int64, ip string) error {
	_, err := framework.Exec(ctx, q, `UPDATE app_user SET last_login_time = NOW(), last_login_ip = ? WHERE id = ?`, ip, id)
	return err
}

// loadCounters 批量取用户的账户余额：map[userId]map[fieldKey]balance。
func loadCounters(ctx context.Context, q framework.Querier, appID int64, userIDs []int64) (map[int64]map[string]json.Number, error) {
	out := map[int64]map[string]json.Number{}
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := framework.SelectRows(ctx, q,
		`SELECT user_id, field_key, balance FROM app_counter WHERE app_id = ? AND user_id IN (`+framework.In(len(userIDs))+`)`,
		append([]any{appID}, framework.Int64Args(userIDs)...)...)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		uid := framework.AsInt64(r["user_id"])
		if out[uid] == nil {
			out[uid] = map[string]json.Number{}
		}
		out[uid][framework.StrOrEmpty(r["field_key"])] = trimDec(r["balance"])
	}
	return out, nil
}

// initPassword 读取参数 app.user.initPassword。
func initPassword(ctx context.Context, q framework.Querier) (string, error) {
	row, err := framework.SelectRow(ctx, q, `SELECT config_value FROM sys_config WHERE config_key = 'app.user.initPassword'`)
	if err != nil {
		return "", err
	}
	if row == nil || strings.TrimSpace(framework.StrOrEmpty(row["config_value"])) == "" {
		return "", framework.Biz("未配置参数 app.user.initPassword")
	}
	return framework.StrOrEmpty(row["config_value"]), nil
}

// insertAuthLog 写认证日志；失败只记录日志，不影响业务流程。
func insertAuthLog(ctx context.Context, q framework.Querier, appID int64, userID *int64, username *string, event string, status int, ip, msg string) {
	_, err := framework.Exec(ctx, q, `
		INSERT INTO app_auth_log (app_id, user_id, username, event, status, ip, msg)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, appID, userID, username, event, status, ip, msg)
	if err != nil {
		log.Printf("认证日志写入失败: %v", err)
	}
}

// isDuplicate 判断 MySQL 唯一键冲突（并发写入时兜底）。
func isDuplicate(err error) bool { return err != nil && strings.Contains(err.Error(), "Error 1062") }
