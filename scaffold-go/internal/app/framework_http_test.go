package app

// 对应 FrameworkHttpBehaviorTest。
// businessExceptionReturnsUnifiedErrorResponse 在 Java 中依赖仅测试态的 /api/test/business-error，
// Go 版无此端点，改用真实 BusinessException 路径：错误密码登录。

import (
	"net/http"
	"strings"
	"testing"
)

func TestResponseContainsTraceIdInHeaderAndBody(t *testing.T) {
	r := get(t, "/api/health", "")

	headerTraceID := r.Header.Get("X-Trace-Id")

	if r.Status != 200 {
		t.Fatalf("status %d: %s", r.Status, r.Raw)
	}
	if strings.TrimSpace(headerTraceID) == "" {
		t.Fatalf("X-Trace-Id header blank")
	}
	if r.Body["traceId"] != headerTraceID {
		t.Errorf("body traceId = %v, header = %q", r.Body["traceId"], headerTraceID)
	}
}

func TestBusinessExceptionReturnsUnifiedErrorResponse(t *testing.T) {
	r := loginResponse(t, "admin", "definitely-wrong-password")

	if r.Status != http.StatusBadRequest {
		t.Fatalf("status %d: %s", r.Status, r.Raw)
	}
	if r.Code() != 400 {
		t.Errorf("code = %d", r.Code())
	}
	if r.Message() != "账号或密码错误" {
		t.Errorf("message = %q", r.Message())
	}
	if s, _ := r.Body["traceId"].(string); s == "" {
		t.Errorf("traceId missing: %s", r.Raw)
	}
}
