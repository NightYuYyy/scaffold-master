package framework

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

type ctxKey int

const (
	ctxTraceID ctxKey = iota
	ctxPrincipal
	ctxBind
)

const TraceHeader = "X-Trace-Id"

// Trace 对应 TraceIdFilter：读取或生成 traceId，写入响应头和 context。
func Trace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get(TraceHeader))
		if id == "" {
			id = UUIDHex()
		}
		w.Header().Set(TraceHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxTraceID, id)))
	})
}

// TraceID 取当前请求 traceId；无则空串。
func TraceID(ctx context.Context) string {
	v, _ := ctx.Value(ctxTraceID).(string)
	return v
}

// UUIDHex 等价于 UUID.randomUUID().toString().replace("-", "")。
func UUIDHex() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b)
}
