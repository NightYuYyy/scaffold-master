package app

// 集成测试公共脚手架，对应 Java 的 @SpringBootTest(RANDOM_PORT) + TestRestTemplate + JdbcTemplate。
// 运行前提：本地 MySQL（库 scaffold_system_test 已导入 V1/V2）与 Redis；环境变量可覆盖 DB_* / REDIS_*。
// 执行：go test ./internal/app/ -count=1

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"scaffold/internal/framework"
)

var (
	testServer *httptest.Server
	testDB     *sql.DB
	testApp    *App
)

func TestMain(m *testing.M) {
	if os.Getenv("JWT_SECRET") == "" {
		os.Setenv("JWT_SECRET", "scaffold-go-integration-test-secret-0123456789")
	}
	if os.Getenv("DB_NAME") == "" {
		os.Setenv("DB_NAME", "scaffold_system_test")
	}
	a, err := New(ConfigFromEnv())
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法启动测试应用（需要本地 MySQL 与 Redis）: %v\n", err)
		os.Exit(1)
	}
	testApp, testDB = a, a.DB
	testServer = httptest.NewServer(a.Handler)
	code := m.Run()
	testServer.Close()
	a.Close()
	os.Exit(code)
}

// resp 为一次 HTTP 调用的结果：状态码、响应头、原始体与解析后的 JSON（非 JSON 时 Body 为 nil）。
type resp struct {
	Status int
	Header http.Header
	Raw    string
	Body   map[string]any
}

// Code 取 body.code（不存在返回 -1）。
func (r resp) Code() int {
	if v, ok := r.Body["code"]; ok {
		return int(framework.AsFloat64(v))
	}
	return -1
}

// Message 取 body.message。
func (r resp) Message() string { return fmt.Sprint(r.Body["message"]) }

// Data 取 body.data 为对象；不是对象时返回 nil。
func (r resp) Data() map[string]any {
	d, _ := r.Body["data"].(map[string]any)
	return d
}

// DataList 取 body.data 为数组。
func (r resp) DataList() []any {
	d, _ := r.Body["data"].([]any)
	return d
}

// Rows 取分页 data.rows。
func (r resp) Rows() []map[string]any {
	rows, _ := r.Data()["rows"].([]any)
	out := make([]map[string]any, 0, len(rows))
	for _, x := range rows {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// call 发起请求；body 为 nil 不发送体，为 string 原样发送，否则 JSON 编码。
func call(t testing.TB, method, path, token string, body any) resp {
	t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	default:
		buf, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
		rd = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, testServer.URL+path, rd)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := testServer.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{Status: res.StatusCode, Header: res.Header, Raw: string(raw)}
	if strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") {
		_ = json.Unmarshal(raw, &out.Body)
	}
	return out
}

func get(t testing.TB, path, token string) resp { return call(t, http.MethodGet, path, token, nil) }
func post(t testing.TB, path, token string, body any) resp {
	return call(t, http.MethodPost, path, token, body)
}
func put(t testing.TB, path, token string, body any) resp {
	return call(t, http.MethodPut, path, token, body)
}
func del(t testing.TB, path, token string) resp { return call(t, http.MethodDelete, path, token, nil) }

// loginResponse 对应 TestAuthClient.loginResponse：先取验证码再登录。
func loginResponse(t testing.TB, username, password string) resp {
	t.Helper()
	captcha := get(t, "/api/auth/captcha", "")
	if captcha.Status != 200 {
		t.Fatalf("captcha failed: %d %s", captcha.Status, captcha.Raw)
	}
	return post(t, "/api/auth/login", "", map[string]any{
		"username":    username,
		"password":    password,
		"captcha":     captcha.Data()["code"],
		"captchaUuid": captcha.Data()["uuid"],
	})
}

// login 对应 TestAuthClient.login：断言成功并返回 token。
func login(t testing.TB, username, password string) string {
	t.Helper()
	r := loginResponse(t, username, password)
	if r.Status != 200 || r.Code() != 200 {
		t.Fatalf("login %s failed: %d %s", username, r.Status, r.Raw)
	}
	token, _ := r.Data()["token"].(string)
	if token == "" || r.Data()["expiresIn"] == nil {
		t.Fatalf("login response missing token/expiresIn: %s", r.Raw)
	}
	return token
}

func loginAsAdmin(t testing.TB) string { return login(t, "admin", "admin123") }

// exec 对应 JdbcTemplate.update。
func exec(t testing.TB, query string, args ...any) {
	t.Helper()
	if _, err := testDB.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// queryInt64 对应 JdbcTemplate.queryForObject(..., Long.class)。
func queryInt64(t testing.TB, query string, args ...any) int64 {
	t.Helper()
	var n sql.NullInt64
	if err := testDB.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return n.Int64
}

// queryString 对应 JdbcTemplate.queryForObject(..., String.class)；NULL 返回空串。
func queryString(t testing.TB, query string, args ...any) string {
	t.Helper()
	var s sql.NullString
	if err := testDB.QueryRow(query, args...).Scan(&s); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return s.String
}

// queryMap 对应 JdbcTemplate.queryForMap。
func queryMap(t testing.TB, query string, args ...any) framework.Row {
	t.Helper()
	row, err := framework.SelectRow(t.Context(), testDB, query, args...)
	if err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return row
}

// uniqueSuffix 对应 System.nanoTime()。
func uniqueSuffix() string { return fmt.Sprint(time.Now().UnixNano()) }

// num 把 JSON 数字转为 int64。
func num(v any) int64 { return int64(framework.AsFloat64(v)) }

// list 取 map 中的数组字段。
func list(m map[string]any, key string) []any {
	l, _ := m[key].([]any)
	return l
}

// containsString 判断 []any 中是否含某字符串。
func containsString(l []any, s string) bool {
	for _, v := range l {
		if fmt.Sprint(v) == s {
			return true
		}
	}
	return false
}

// hash 对应测试里的 passwordEncoder.encode。
func hash(raw string) string {
	h, err := framework.HashPassword(raw)
	if err != nil {
		panic(err)
	}
	return h
}
