package platform

import (
	"context"
	"encoding/json"
	"math/big"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"scaffold/internal/framework"
)

var fieldTypes = []string{"string", "number", "bool", "date", "enum", "counter"}

// fieldDef 对应 app_field 一行。
type fieldDef struct {
	ID           int64
	AppID        int64
	Key          string
	Label        string
	Type         string
	Required     int
	UserEditable int
	DefaultValue *string
	Options      *string // 原始 JSON 文本
	Sort         int
	Status       int
	Remark       *string
	CreateTime   *string
}

const fieldSelect = `
	SELECT id, app_id, field_key, label, field_type, required, user_editable, default_value, options, sort, status, remark,
	       DATE_FORMAT(create_time, '%Y-%m-%d %H:%i') AS create_time
	FROM app_field`

func scanField(row framework.Row) fieldDef {
	return fieldDef{
		ID:           framework.AsInt64(row["id"]),
		AppID:        framework.AsInt64(row["app_id"]),
		Key:          framework.StrOrEmpty(row["field_key"]),
		Label:        framework.StrOrEmpty(row["label"]),
		Type:         framework.StrOrEmpty(row["field_type"]),
		Required:     int(framework.AsInt64(row["required"])),
		UserEditable: int(framework.AsInt64(row["user_editable"])),
		DefaultValue: strPtr(row["default_value"]),
		Options:      strPtr(row["options"]),
		Sort:         int(framework.AsInt64(row["sort"])),
		Status:       int(framework.AsInt64(row["status"])),
		Remark:       strPtr(row["remark"]),
		CreateTime:   strPtr(row["create_time"]),
	}
}

// loadFields 取应用字段定义，按 sort, id。
func loadFields(ctx context.Context, q framework.Querier, appID int64, enabledOnly bool) ([]fieldDef, error) {
	where := " WHERE app_id = ?"
	if enabledOnly {
		where += " AND status = 1"
	}
	rows, err := framework.SelectRows(ctx, q, fieldSelect+where+" ORDER BY sort, id", appID)
	if err != nil {
		return nil, err
	}
	out := make([]fieldDef, 0, len(rows))
	for _, row := range rows {
		out = append(out, scanField(row))
	}
	return out, nil
}

func loadField(ctx context.Context, q framework.Querier, id int64) (*fieldDef, error) {
	row, err := framework.SelectRow(ctx, q, fieldSelect+" WHERE id = ?", id)
	if err != nil || row == nil {
		return nil, err
	}
	f := scanField(row)
	return &f, nil
}

// loadCounterField 取启用的 counter 字段；不存在返回 nil。
func loadCounterField(ctx context.Context, q framework.Querier, appID int64, key string) (*fieldDef, error) {
	row, err := framework.SelectRow(ctx, q, fieldSelect+" WHERE app_id = ? AND field_key = ? AND field_type = 'counter' AND status = 1", appID, key)
	if err != nil || row == nil {
		return nil, err
	}
	f := scanField(row)
	return &f, nil
}

func (f fieldDef) enumOptions() []string {
	var out []string
	if f.Options != nil {
		_ = json.Unmarshal([]byte(*f.Options), &out)
	}
	return out
}

// counterOptions 解析 counter 的 {allowNegative, max}；max 为 1e4 定点整数，nil 表示不限。
func (f fieldDef) counterOptions() (allowNegative bool, max *big.Int) {
	var o struct {
		AllowNegative bool        `json:"allowNegative"`
		Max           json.Number `json:"max"`
	}
	if f.Options != nil {
		_ = json.Unmarshal([]byte(*f.Options), &o)
	}
	if o.Max != "" {
		if n, ok := parseDec4(o.Max.String()); ok {
			max = n
		}
	}
	return o.AllowNegative, max
}

// optionsView enum → 字符串数组；counter → {allowNegative, max}；其他 → null。
func (f fieldDef) optionsView() any {
	switch f.Type {
	case "enum":
		opts := f.enumOptions()
		if opts == nil {
			opts = []string{}
		}
		return opts
	case "counter":
		allow, max := f.counterOptions()
		var m any
		if max != nil {
			m = formatDec4(max)
		}
		return framework.Row{"allowNegative": allow, "max": m}
	}
	return nil
}

// defaultTyped 默认值按类型转换；无默认值或无法转换时为 nil。
func (f fieldDef) defaultTyped() any {
	if f.DefaultValue == nil {
		return nil
	}
	v, err := f.normalize(*f.DefaultValue)
	if err != nil {
		return nil
	}
	return v
}

// normalize 校验并规范化一个 profile 值（输入来自 UseNumber 解码：string / json.Number / bool / nil）。
func (f fieldDef) normalize(v any) (any, error) {
	bad := framework.Biz("字段 " + f.Key + " 格式不正确")
	if v == nil {
		return nil, nil
	}
	switch f.Type {
	case "string":
		s, ok := v.(string)
		if !ok || utf8.RuneCountInString(s) > 500 {
			return nil, bad
		}
		return s, nil
	case "number":
		switch x := v.(type) {
		case json.Number:
			return x, nil
		case string:
			var n json.Number
			if json.Unmarshal([]byte(strings.TrimSpace(x)), &n) == nil && n != "" {
				return n, nil
			}
		}
		return nil, bad
	case "bool":
		switch x := v.(type) {
		case bool:
			return x, nil
		case string:
			if x == "true" || x == "false" {
				return x == "true", nil
			}
		}
		return nil, bad
	case "date":
		s, ok := v.(string)
		if !ok || len(s) != 10 {
			return nil, bad
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return nil, bad
		}
		return s, nil
	case "enum":
		s, ok := v.(string)
		if !ok || !slices.Contains(f.enumOptions(), s) {
			return nil, bad
		}
		return s, nil
	}
	return nil, bad
}

// view 字段定义的接口形态；admin 为 true 时附带后台字段。
func (f fieldDef) view(admin bool) framework.Row {
	row := framework.Row{
		"key": f.Key, "label": f.Label, "type": f.Type, "required": f.Required, "userEditable": f.UserEditable,
		"defaultValue": f.DefaultValue, "options": f.optionsView(), "sort": f.Sort,
	}
	if admin {
		row["id"], row["appId"], row["status"], row["remark"], row["createTime"] = f.ID, f.AppID, f.Status, f.Remark, f.CreateTime
	}
	return row
}

// ---------- profile 校验与视图 ----------

type profileMode int

const (
	profileCreate    profileMode = iota // 可信服务端 / 后台建用户：校验必填
	profileUserEdit                     // /me：只允许 user_editable = 1
	profileAdminEdit                    // 服务端 / 后台改资料：任意非 counter 字段
	profileRegister                     // 公开注册：校验必填，且只允许用户可编辑字段
)

// applyProfile 校验输入并合并到 current，返回合并后的 map（不修改 current）。
func applyProfile(fields []fieldDef, current, input map[string]any, mode profileMode) (map[string]any, error) {
	byKey := map[string]fieldDef{}
	for _, f := range fields {
		if f.Status == 1 && f.Type != "counter" {
			byKey[f.Key] = f
		}
	}
	out := make(map[string]any, len(current)+len(input))
	for k, v := range current {
		out[k] = v
	}
	keys := make([]string, 0, len(input))
	for k := range input {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		f, ok := byKey[k]
		if !ok {
			return nil, framework.Biz("未定义的字段: " + k)
		}
		if (mode == profileUserEdit || mode == profileRegister) && f.UserEditable != 1 {
			return nil, &framework.Error{Status: 403, Code: 403, Message: "字段 " + k + " 不允许用户修改"}
		}
		v, err := f.normalize(input[k])
		if err != nil {
			return nil, err
		}
		if v == nil {
			delete(out, k)
		} else {
			out[k] = v
		}
	}
	if mode == profileCreate || mode == profileRegister {
		for _, f := range fields {
			if f.Status == 1 && f.Type != "counter" && f.Required == 1 && f.DefaultValue == nil && out[f.Key] == nil {
				return nil, framework.Biz("字段 " + f.Key + " 不能为空")
			}
		}
	}
	return out, nil
}

// profileView 全部启用的非 counter 字段：未填的给默认值，无默认值给 null。
func profileView(fields []fieldDef, stored map[string]any) framework.Row {
	out := framework.Row{}
	for _, f := range fields {
		if f.Status != 1 || f.Type == "counter" {
			continue
		}
		if v, ok := stored[f.Key]; ok && v != nil {
			out[f.Key] = v
		} else {
			out[f.Key] = f.defaultTyped()
		}
	}
	return out
}

// countersView 全部启用的 counter 字段余额，没有记录时为 0。
func countersView(fields []fieldDef, balances map[string]json.Number) framework.Row {
	out := framework.Row{}
	for _, f := range fields {
		if f.Status != 1 || f.Type != "counter" {
			continue
		}
		if b, ok := balances[f.Key]; ok {
			out[f.Key] = b
		} else {
			out[f.Key] = json.Number("0")
		}
	}
	return out
}

func marshalProfile(m map[string]any) (string, error) {
	if m == nil {
		m = map[string]any{}
	}
	b, err := json.Marshal(m)
	return string(b), err
}

// ---------- 字段定义校验（后台） ----------

// validateOptions 按类型校验并规范化 options，返回入库 JSON（非 enum / counter 为 nil）。
func validateOptions(ftype string, options any) (*string, error) {
	switch ftype {
	case "enum":
		list, _ := options.([]any)
		opts := make([]string, 0, len(list))
		for _, o := range list {
			s, ok := o.(string)
			if !ok || strings.TrimSpace(s) == "" {
				return nil, framework.Biz("枚举字段必须配置可选值")
			}
			opts = append(opts, s)
		}
		if len(opts) == 0 {
			return nil, framework.Biz("枚举字段必须配置可选值")
		}
		b, _ := json.Marshal(opts)
		s := string(b)
		return &s, nil
	case "counter":
		out := map[string]any{"allowNegative": false, "max": nil}
		if options != nil {
			m, ok := options.(map[string]any)
			if !ok {
				return nil, framework.Biz("账户选项格式不正确")
			}
			if b, ok := m["allowNegative"].(bool); ok {
				out["allowNegative"] = b
			} else if m["allowNegative"] != nil {
				return nil, framework.Biz("账户选项格式不正确")
			}
			if m["max"] != nil {
				raw, _ := json.Marshal(m["max"])
				n, ok := parseDec4(strings.Trim(string(raw), `"`))
				if !ok || n.Sign() <= 0 {
					return nil, framework.Biz("账户上限必须为正数且最多 4 位小数")
				}
				out["max"] = formatDec4(n)
			}
		}
		b, _ := json.Marshal(out)
		s := string(b)
		return &s, nil
	}
	return nil, nil
}

// validateDefault 默认值须能按字段类型解释；空串视为无默认值。
func validateDefault(f fieldDef, text *string) (*string, error) {
	if text == nil || strings.TrimSpace(*text) == "" {
		return nil, nil
	}
	if _, err := f.normalize(*text); err != nil {
		return nil, framework.Biz("默认值格式不正确")
	}
	return text, nil
}
