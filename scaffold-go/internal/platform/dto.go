package platform

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf16"

	"scaffold/internal/framework"
)

// 请求体：装箱字段一律用指针保留 null 语义；标量但需保留精度的字段（金额、默认值、profile）用 json.RawMessage。

type appWriteRequest struct {
	Code                *string `json:"code"`
	Name                *string `json:"name"`
	Status              *int    `json:"status"`
	AllowRegister       *int    `json:"allowRegister"`
	CaptchaEnabled      *int    `json:"captchaEnabled"`
	TokenExpiresSeconds *int64  `json:"tokenExpiresSeconds"`
	Remark              *string `json:"remark"`
}

type statusRequest struct {
	Status *int `json:"status"`
}

type keyCreateRequest struct {
	Name *string `json:"name"`
}

type fieldWriteRequest struct {
	AppID        *int64          `json:"appId"`
	Key          *string         `json:"key"`
	Label        *string         `json:"label"`
	Type         *string         `json:"type"`
	Required     *int            `json:"required"`
	UserEditable *int            `json:"userEditable"`
	DefaultValue json.RawMessage `json:"defaultValue"`
	Options      any             `json:"options"`
	Sort         *int            `json:"sort"`
	Status       *int            `json:"status"`
	Remark       *string         `json:"remark"`
}

// userWriteRequest 同时服务注册 / 服务端建用户 / 后台建用户 / 改资料（各接口只取自己关心的字段）。
type userWriteRequest struct {
	AppID       *int64          `json:"appId"`
	Username    *string         `json:"username"`
	Password    *string         `json:"password"`
	Nickname    *string         `json:"nickname"`
	Avatar      *string         `json:"avatar"`
	Status      *int            `json:"status"`
	Profile     json.RawMessage `json:"profile"`
	Captcha     *string         `json:"captcha"`
	CaptchaUUID *string         `json:"captchaUuid"`
}

type passwordRequest struct {
	Password *string `json:"password"`
}

type changePasswordRequest struct {
	OldPassword *string `json:"oldPassword"`
	NewPassword *string `json:"newPassword"`
}

type introspectRequest struct {
	Token *string `json:"token"`
}

type counterChangeRequest struct {
	Amount json.RawMessage `json:"amount"`
	Reason *string         `json:"reason"`
	RefNo  *string         `json:"refNo"`
}

// ---------- 校验辅助 ----------

var (
	usernameRe = regexp.MustCompile(`^[A-Za-z0-9_.@-]{3,64}$`)
	appCodeRe  = regexp.MustCompile(`^[a-z0-9_]{3,64}$`)
	fieldKeyRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)
)

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// blank 对应 @NotBlank。
func blank(s *string) bool { return s == nil || strings.TrimSpace(*s) == "" }

// utf16Len 对应 String.length()。
func utf16Len(s string) int { return len(utf16.Encode([]rune(s))) }

func requireText(field string, s *string, max int) (string, error) {
	if blank(s) {
		return "", framework.Invalid(field, "不能为空")
	}
	if utf16Len(*s) > max {
		return "", framework.Invalid(field, "长度不能超过"+itoa(max))
	}
	return strings.TrimSpace(*s), nil
}

func checkLen(field string, s *string, max int) error {
	if s != nil && utf16Len(*s) > max {
		return framework.Invalid(field, "长度不能超过"+itoa(max))
	}
	return nil
}

func validateUsername(s *string) (string, error) {
	if s == nil || !usernameRe.MatchString(*s) {
		return "", framework.Invalid("username", "3-64 位，仅字母、数字、_ . @ -")
	}
	return *s, nil
}

func validatePassword(field string, s *string) (string, error) {
	if s == nil {
		return "", framework.Invalid(field, "不能为空")
	}
	if n := utf16Len(*s); n < 6 || n > 64 {
		return "", framework.Invalid(field, "长度必须在6到64位之间")
	}
	return *s, nil
}

// flag 读取 0/1 开关；nil 取默认值。
func flag(field string, v *int, def int) (int, error) {
	if v == nil {
		return def, nil
	}
	if *v != 0 && *v != 1 {
		return 0, framework.Invalid(field, "只能为 0 或 1")
	}
	return *v, nil
}

func requireStatus(v *int) (int, error) {
	if v == nil {
		return 0, framework.Invalid("status", "不能为空")
	}
	return flag("status", v, 0)
}

// like 生成模糊匹配参数；空白返回空串表示不过滤。
func like(p *string) string {
	if p == nil || strings.TrimSpace(*p) == "" {
		return ""
	}
	return "%" + strings.TrimSpace(*p) + "%"
}

func nullIfEmpty(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	return s
}

func itoa(n int) string { return framework.AsString(int64(n)) }

// decodeProfile 解析 profile 请求体（数字保留为 json.Number）；缺省或 null 返回 nil。
func decodeProfile(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	out := map[string]any{}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, framework.Biz("profile 格式不正确")
	}
	return out, nil
}

// rawScalarText 把 JSON 标量转为文本形式；present 表示请求体中给出了该键（含 null）。
func rawScalarText(raw json.RawMessage) (text *string, present bool, err error) {
	if len(raw) == 0 {
		return nil, false, nil
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, true, framework.Biz("默认值格式不正确")
	}
	var s string
	switch x := v.(type) {
	case nil:
		return nil, true, nil
	case string:
		s = x
	case json.Number:
		s = x.String()
	case bool:
		s = "false"
		if x {
			s = "true"
		}
	default:
		return nil, true, framework.Biz("默认值格式不正确")
	}
	return &s, true, nil
}
