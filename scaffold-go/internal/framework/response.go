// Package framework 对应 Java 的 scaffold-common + scaffold-framework：
// 统一响应、异常映射、TraceId、认证、权限、数据范围、操作日志、DB 访问。
package framework

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
)

// R 统一响应结构，与 Java R<T> 字段一致。
type R struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
	TraceID string `json:"traceId"`
}

// Error 业务/框架错误：Status 为 HTTP 状态码，Code 为响应体 code。
// Java 的 BusinessException 统一返回 HTTP 400，但 body.code 取异常自身 code。
type Error struct {
	Status  int
	Code    int
	Message string
	// bind 为 true 表示参数绑定/校验类错误（Spring 中发生在切面之前，不记操作日志）。
	bind bool
}

func (e *Error) Error() string { return e.Message }

// Biz 等价于 new BusinessException(message)：HTTP 400 / code 400。
func Biz(message string) *Error { return &Error{Status: 400, Code: 400, Message: message} }

// BizCode 等价于 new BusinessException(code, message)：HTTP 400 / code=code。
func BizCode(code int, message string) *Error {
	return &Error{Status: 400, Code: code, Message: message}
}

// Bizf 带格式化的 Biz。
func Bizf(format string, args ...any) *Error { return Biz(fmt.Sprintf(format, args...)) }

var (
	ErrUnauthorized   = &Error{Status: 401, Code: 401, Message: "未认证或登录已过期"}
	ErrForbidden      = &Error{Status: 403, Code: 403, Message: "没有访问权限"}
	ErrNotFound       = &Error{Status: 404, Code: 404, Message: "资源不存在"}
	ErrBodyUnreadable = &Error{Status: 400, Code: 400, Message: "请求体格式错误", bind: true}
)

// TypeMismatch 对应 MethodArgumentTypeMismatchException。
func TypeMismatch(name string) *Error {
	return &Error{Status: 400, Code: 400, Message: "参数类型错误: " + name, bind: true}
}

// Invalid 对应 MethodArgumentNotValidException：message = field + " " + 约束消息。
func Invalid(field, message string) *Error {
	return &Error{Status: 400, Code: 400, Message: field + " " + message, bind: true}
}

// HandlerFunc 业务处理函数：返回 error 由框架统一映射为响应。
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// Middleware 包装 HandlerFunc。
type Middleware func(HandlerFunc) HandlerFunc

// Handle 把 HandlerFunc 和中间件组装为 http.Handler；mws 中第一个在最外层。
func Handle(h HandlerFunc, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 客户端中途断开不取消业务与事务，与 Servlet 容器行为一致
		r = r.WithContext(context.WithoutCancel(r.Context()))
		if err := h(w, r); err != nil {
			WriteError(w, r, err)
		}
	})
}

// OK 写出成功响应 R.success(data)。
func OK(w http.ResponseWriter, r *http.Request, data any) error {
	return WriteJSON(w, 200, R{Code: 200, Message: "操作成功", Data: data, TraceID: TraceID(r.Context())})
}

// WriteError 对应 GlobalExceptionHandler。
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var e *Error
	if errors.As(err, &e) {
		_ = WriteJSON(w, e.Status, R{Code: e.Code, Message: e.Message, TraceID: TraceID(r.Context())})
		return
	}
	log.Printf("[%s] 未处理异常: %v", TraceID(r.Context()), err)
	_ = WriteJSON(w, 500, R{Code: 500, Message: "系统内部错误", TraceID: TraceID(r.Context())})
}

// WriteJSON 以 application/json 写出任意值。
func WriteJSON(w http.ResponseWriter, status int, v any) error {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// CSV 写出 text/csv 附件，与 Java controller 的 csv() 一致。
func CSV(w http.ResponseWriter, filename, content string) error {
	w.Header().Set("Content-Type", "text/csv;charset=UTF-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(200)
	_, err := w.Write([]byte(content))
	return err
}

// Attachment 写出二进制附件（生成器 ZIP）。
func Attachment(w http.ResponseWriter, contentType, filename string, body []byte) error {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(200)
	_, err := w.Write(body)
	return err
}

// Recover 兜底 panic → 500。
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				WriteError(w, r, fmt.Errorf("panic: %v", p))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

var probeMethods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodHead, http.MethodOptions}

// NotFoundOr405 包装 ServeMux：路径未注册 → 404 JSON；路径存在但方法不匹配 → 405 JSON。
func NotFoundOr405(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := mux.Handler(r); pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		for _, m := range probeMethods {
			if m == r.Method {
				continue
			}
			probe := r.Clone(r.Context())
			probe.Method = m
			if _, p := mux.Handler(probe); p != "" {
				_ = WriteJSON(w, 405, R{Code: 405, Message: "不支持的请求方法: " + r.Method, TraceID: TraceID(r.Context())})
				return
			}
		}
		WriteError(w, r, ErrNotFound)
	})
}
