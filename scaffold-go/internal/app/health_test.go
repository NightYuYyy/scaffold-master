package app

// 对应 HealthControllerTest。

import "testing"

func TestHealthEndpointReturnsUnifiedUpResponseWithoutAuthentication(t *testing.T) {
	r := get(t, "/api/health", "")

	if r.Status != 200 {
		t.Fatalf("status %d: %s", r.Status, r.Raw)
	}
	if r.Code() != 200 {
		t.Errorf("code = %d", r.Code())
	}
	if r.Message() != "操作成功" {
		t.Errorf("message = %q", r.Message())
	}
	if r.Data()["status"] != "UP" {
		t.Errorf("data.status = %v", r.Data()["status"])
	}
}
