package framework

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// OperationLogEntry 对应 Java OperationLogEntry。
type OperationLogEntry struct {
	Module        string
	Action        string
	RequestMethod string
	RequestURL    string
	RequestParams *string
	Operator      *string
	IP            string
	Cost          int64
	Result        int
	ErrorMessage  *string
}

// OperationLogRecorder 对应 Java OperationLogRecorder，由 system 包实现（写 sys_oper_log）。
type OperationLogRecorder interface {
	Record(ctx context.Context, entry OperationLogEntry)
}

const maxOperLogText = 4000

// OperLog 对应 @OperLog 切面：记录模块/动作/方法/URL/脱敏参数/操作人/IP/耗时/结果/错误。
// 参数与 Java 的 objectMapper.valueToTree(joinPoint.getArgs()) 一致：[路径参数..., 绑定后的请求体]，
// DTO 按字段声明顺序输出全部字段（含 null），敏感字段无论是否为 null 都替换为 "******"。
// 参数绑定失败（Spring 中发生在切面之前）不记录。
func OperLog(recorder OperationLogRecorder, module, action string) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			start := time.Now()
			ctx, holder := withBindHolder(r.Context())
			r = r.WithContext(ctx)
			err := next(w, r)
			if err != nil && IsBindError(err) {
				return err
			}
			entry := OperationLogEntry{
				Module:        module,
				Action:        action,
				RequestMethod: r.Method,
				RequestURL:    r.URL.Path,
				RequestParams: truncateUTF16(buildParams(pathArgs(r), holder.entries), maxOperLogText),
				IP:            ClientIP(r),
				Cost:          time.Since(start).Milliseconds(),
				Result:        1,
			}
			if u := CurrentUsername(r.Context()); u != "" {
				entry.Operator = &u
			}
			if err != nil {
				entry.Result = 0
				entry.ErrorMessage = truncateUTF16(err.Error(), maxOperLogText)
			}
			recorder.Record(r.Context(), entry)
			return err
		}
	}
}

// pathArgs 按 pattern 中 {name} 出现顺序取路径参数，数字转为整数（对应 @PathVariable Long）。
func pathArgs(r *http.Request) []any {
	args := []any{}
	for _, seg := range strings.Split(r.Pattern, "/") {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			name := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(seg, "{"), "}"), "...")
			v := r.PathValue(name)
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				args = append(args, n)
			} else {
				args = append(args, v)
			}
		}
	}
	return args
}

// buildParams 生成 JSON 数组文本：struct 目标按字段顺序序列化，map 目标保留原始键序。
func buildParams(path []any, bodies []bindEntry) string {
	var b bytes.Buffer
	b.WriteByte('[')
	for i, p := range path {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(marshalNoEscape(p))
	}
	for _, e := range bodies {
		if b.Len() > 1 {
			b.WriteByte(',')
		}
		var raw []byte
		if t := reflect.TypeOf(e.value); t != nil && t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Map {
			var compact bytes.Buffer
			if json.Compact(&compact, e.raw) == nil {
				raw = compact.Bytes()
			}
		}
		if raw == nil {
			raw = marshalNoEscape(e.value)
		}
		b.Write(maskJSON(raw))
	}
	b.WriteByte(']')
	return b.String()
}

func marshalNoEscape(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if enc.Encode(v) != nil {
		return []byte("null")
	}
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// maskJSON 在不改变键序的前提下，把敏感字段的值（含嵌套对象）替换为 "******"。
func maskJSON(raw []byte) []byte {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var out bytes.Buffer
	var stack []byte
	expectKey, needComma := false, false
	inObject := func() bool { return len(stack) > 0 && stack[len(stack)-1] == '{' }
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if d, ok := tok.(json.Delim); ok {
			switch d {
			case '{', '[':
				if needComma {
					out.WriteByte(',')
				}
				out.WriteByte(byte(d))
				stack = append(stack, byte(d))
				expectKey, needComma = d == '{', false
			default:
				out.WriteByte(byte(d))
				stack = stack[:len(stack)-1]
				expectKey, needComma = inObject(), true
			}
			continue
		}
		if needComma {
			out.WriteByte(',')
		}
		if expectKey {
			key, _ := tok.(string)
			out.Write(marshalNoEscape(key))
			out.WriteByte(':')
			if isSensitiveField(key) {
				skipValue(dec)
				out.WriteString(`"******"`)
				needComma = true
			} else {
				expectKey, needComma = false, false
			}
			continue
		}
		if n, ok := tok.(json.Number); ok {
			out.WriteString(n.String())
		} else {
			out.Write(marshalNoEscape(tok))
		}
		expectKey, needComma = inObject(), true
	}
	return out.Bytes()
}

// skipValue 消费一个完整的值（标量或嵌套容器）。
func skipValue(dec *json.Decoder) {
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		if d, ok := tok.(json.Delim); ok {
			if d == '{' || d == '[' {
				depth++
			} else {
				depth--
			}
		}
		if depth == 0 {
			return
		}
	}
}

func isSensitiveField(field string) bool {
	lower := strings.ToLower(field)
	return strings.Contains(lower, "password") || strings.Contains(lower, "token") ||
		strings.Contains(lower, "secret") || strings.Contains(lower, "authorization")
}

// truncateUTF16 对应 String.substring(0, max)：按 UTF-16 单元截断。
func truncateUTF16(s string, max int) *string {
	if u := utf16.Encode([]rune(s)); len(u) > max {
		s = string(utf16.Decode(u[:max]))
	}
	return &s
}
