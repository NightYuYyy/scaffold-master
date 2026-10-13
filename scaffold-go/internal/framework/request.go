package framework

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

// bindEntry 记录一次 @RequestBody 绑定结果，供操作日志按 Java joinPoint.getArgs() 的形态序列化。
type bindEntry struct {
	value any    // 绑定后的指针（struct 或 map）
	raw   []byte // 原始请求体（map 目标保持 Java LinkedHashMap 的键序）
}

type bindHolder struct{ entries []bindEntry }

// withBindHolder 由 OperLog 中间件安装；不在审计范围内的接口没有 holder，Bind 不登记。
func withBindHolder(ctx context.Context) (context.Context, *bindHolder) {
	h := &bindHolder{}
	return context.WithValue(ctx, ctxBind, h), h
}

// Bind 对应 @RequestBody 的 Jackson 默认行为：
// 空体 / 非法 JSON / 字面量 null / 类型无法转换 → 400 "请求体格式错误"；JSON 之后的多余内容忽略；
// 未知字段忽略，字段名大小写敏感；标量宽松转换（小数→整数截断、数字字符串→数字、数字→字符串、空串→null）。
func Bind(r *http.Request, v any) error {
	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 10<<20))
	if err != nil {
		return ErrBodyUnreadable
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil || raw == nil {
		return ErrBodyUnreadable
	}
	buf, err := json.Marshal(coerce(raw, reflect.TypeOf(v)))
	if err != nil {
		return ErrBodyUnreadable
	}
	if err := json.Unmarshal(buf, v); err != nil {
		return ErrBodyUnreadable
	}
	if h, ok := r.Context().Value(ctxBind).(*bindHolder); ok {
		h.entries = append(h.entries, bindEntry{value: v, raw: body})
	}
	return nil
}

// coerce 按目标类型做 Jackson 风格的标量转换；struct 目标只保留与 json tag 精确匹配的键。
func coerce(v any, t reflect.Type) any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		m, ok := v.(map[string]any)
		if !ok {
			return v
		}
		out := make(map[string]any, len(m))
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "" {
				name = f.Name
			}
			if name == "-" {
				continue
			}
			if val, ok := m[name]; ok {
				out[name] = coerce(val, f.Type)
			}
		}
		return out
	case reflect.Slice:
		l, ok := v.([]any)
		if !ok {
			return v
		}
		out := make([]any, len(l))
		for i := range l {
			out[i] = coerce(l[i], t.Elem())
		}
		return out
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		switch x := v.(type) {
		case json.Number:
			if i, err := x.Int64(); err == nil {
				return i
			}
			if f, err := x.Float64(); err == nil {
				return int64(f) // ACCEPT_FLOAT_AS_INT：截断
			}
		case string:
			s := strings.TrimSpace(x)
			if s == "" {
				return nil
			}
			if i, err := strconv.ParseInt(s, 10, 64); err == nil {
				return i
			}
		}
	case reflect.Float32, reflect.Float64:
		if s, ok := v.(string); ok {
			if strings.TrimSpace(s) == "" {
				return nil
			}
			if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
				return f
			}
		}
	case reflect.String:
		switch x := v.(type) {
		case json.Number:
			return x.String()
		case bool:
			return strconv.FormatBool(x)
		}
	case reflect.Bool:
		if s, ok := v.(string); ok {
			if strings.TrimSpace(s) == "" {
				return nil
			}
			if b, err := strconv.ParseBool(strings.TrimSpace(s)); err == nil {
				return b
			}
		}
	}
	return v
}

// PathInt64 对应 @PathVariable Long。
func PathInt64(r *http.Request, name string) (int64, error) {
	v, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		return 0, TypeMismatch(name)
	}
	return v, nil
}

// QueryString 对应 @RequestParam(required=false) String：缺省为 nil，"?k=" 为空串。
func QueryString(r *http.Request, name string) *string {
	if !r.URL.Query().Has(name) {
		return nil
	}
	s := r.URL.Query().Get(name)
	return &s
}

// QueryStringDefault 对应 @RequestParam(defaultValue=...) String。
func QueryStringDefault(r *http.Request, name, def string) string {
	if v := r.URL.Query().Get(name); v != "" {
		return v
	}
	return def
}

// QueryInt 对应 @RequestParam(required=false) Integer。
func QueryInt(r *http.Request, name string) (*int, error) {
	if !r.URL.Query().Has(name) || r.URL.Query().Get(name) == "" {
		return nil, nil
	}
	v, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return nil, TypeMismatch(name)
	}
	return &v, nil
}

// QueryIntDefault 对应 @RequestParam(defaultValue="7") Integer。
func QueryIntDefault(r *http.Request, name string, def int) (int, error) {
	v, err := QueryInt(r, name)
	if err != nil {
		return 0, err
	}
	if v == nil {
		return def, nil
	}
	return *v, nil
}

// QueryInt64 对应 @RequestParam(required=false) Long。
func QueryInt64(r *http.Request, name string) (*int64, error) {
	if !r.URL.Query().Has(name) || r.URL.Query().Get(name) == "" {
		return nil, nil
	}
	v, err := strconv.ParseInt(r.URL.Query().Get(name), 10, 64)
	if err != nil {
		return nil, TypeMismatch(name)
	}
	return &v, nil
}

// PageQuery 对应 Java PageQuery record。
type PageQuery struct {
	PageNum   *int
	PageSize  *int
	SortField *string
	SortOrder *string
}

const (
	DefaultPageNum  = 1
	DefaultPageSize = 10
	MaxPageSize     = 100
)

// ParsePage 从 query 绑定 PageQuery。
func ParsePage(r *http.Request) (PageQuery, error) {
	var q PageQuery
	var err error
	if q.PageNum, err = QueryInt(r, "pageNum"); err != nil {
		return q, err
	}
	if q.PageSize, err = QueryInt(r, "pageSize"); err != nil {
		return q, err
	}
	q.SortField = QueryString(r, "sortField")
	q.SortOrder = QueryString(r, "sortOrder")
	return q, nil
}

func (q PageQuery) NormalizedPageNum() int {
	if q.PageNum == nil || *q.PageNum < 1 {
		return DefaultPageNum
	}
	return *q.PageNum
}

func (q PageQuery) NormalizedPageSize() int {
	if q.PageSize == nil || *q.PageSize < 1 {
		return DefaultPageSize
	}
	return min(*q.PageSize, MaxPageSize)
}

func (q PageQuery) NormalizedSortOrder() string {
	if q.SortOrder != nil && strings.EqualFold(*q.SortOrder, "asc") {
		return "asc"
	}
	return "desc"
}

// Offset = (pageNum-1)*pageSize。
func (q PageQuery) Offset() int { return (q.NormalizedPageNum() - 1) * q.NormalizedPageSize() }

// PageResult 对应 Java PageResult<T>。
type PageResult[T any] struct {
	Rows     []T   `json:"rows"`
	Total    int64 `json:"total"`
	PageNum  int   `json:"pageNum"`
	PageSize int   `json:"pageSize"`
}

// PageOf 构造分页结果；rows 为 nil 时输出 [] 而不是 null。
func PageOf[T any](rows []T, total int64, q PageQuery) PageResult[T] {
	if rows == nil {
		rows = []T{}
	}
	return PageResult[T]{Rows: rows, Total: total, PageNum: q.NormalizedPageNum(), PageSize: q.NormalizedPageSize()}
}

// IsBindError 判断是否为请求参数绑定类错误（Spring 中这类错误发生在切面之前，不记操作日志）。
func IsBindError(err error) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	return e.bind
}
