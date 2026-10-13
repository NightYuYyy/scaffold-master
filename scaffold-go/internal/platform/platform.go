// Package platform 实现用户中心平台：应用、接口密钥、应用用户、扩展字段、数值账户与认证日志。
// 管理接口挂在 /api/platform（走后台登录与权限），开放接口挂在 /open/v1（走应用密钥与用户令牌）。
// 契约见 docs/platform-api.md。
package platform

import (
	"database/sql"
	"net/http"

	"github.com/redis/go-redis/v9"

	"scaffold/internal/framework"
	"scaffold/internal/system"
)

// Deps 平台模块依赖。
type Deps struct {
	DB       *sql.DB
	Redis    *redis.Client
	Security *framework.Security
	Recorder framework.OperationLogRecorder
	// Perm 用于判断当前后台账号是否 super_admin 等。
	Perm *system.PermissionResolver
}

// svc 平台服务：管理接口与开放接口共用同一份数据访问逻辑。
type svc struct {
	db      *sql.DB
	rdb     *redis.Client
	captcha *framework.Captcha
	perm    *system.PermissionResolver
}

func newSvc(d Deps) *svc {
	return &svc{db: d.DB, rdb: d.Redis, captcha: framework.NewCaptcha(d.Redis), perm: d.Perm}
}

// jsonH 把 (data, error) 形式的处理函数包装为 HandlerFunc，成功统一写 R.success(data)。
func jsonH(fn func(r *http.Request) (any, error)) framework.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		data, err := fn(r)
		if err != nil {
			return err
		}
		return framework.OK(w, r, data)
	}
}

// csvH 把 (csv, error) 形式的导出函数包装为 HandlerFunc。
func csvH(filename string, fn func(r *http.Request) (string, error)) framework.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		content, err := fn(r)
		if err != nil {
			return err
		}
		return framework.CSV(w, filename, content)
	}
}

// RegisterAdminRoutes 注册 /api/platform/* 管理接口到后台 mux。
func RegisterAdminRoutes(mux *http.ServeMux, d Deps) {
	s := newSvc(d)
	perm := d.Security.Perm
	oper := func(module, action string) framework.Middleware { return framework.OperLog(d.Recorder, module, action) }

	// ---------- 应用 ----------
	mux.Handle("GET /api/platform/apps", framework.Handle(jsonH(s.adminApps), perm("platform:app:list")))
	mux.Handle("GET /api/platform/apps/options", framework.Handle(jsonH(s.adminAppOptions)))
	mux.Handle("GET /api/platform/apps/export", framework.Handle(csvH("apps.csv", s.adminExportApps), perm("platform:app:list")))
	mux.Handle("GET /api/platform/apps/{id}", framework.Handle(jsonH(s.adminApp), perm("platform:app:list")))
	mux.Handle("POST /api/platform/apps", framework.Handle(jsonH(s.adminCreateApp), oper("应用管理", "新增"), perm("platform:app:add")))
	mux.Handle("PUT /api/platform/apps/{id}", framework.Handle(jsonH(s.adminUpdateApp), oper("应用管理", "修改"), perm("platform:app:edit")))
	mux.Handle("PUT /api/platform/apps/{id}/status", framework.Handle(jsonH(s.adminAppStatus), oper("应用管理", "状态变更"), perm("platform:app:edit")))
	mux.Handle("DELETE /api/platform/apps/{id}", framework.Handle(jsonH(s.adminDeleteApp), oper("应用管理", "删除"), perm("platform:app:remove")))

	// ---------- 接口密钥 ----------
	mux.Handle("GET /api/platform/apps/{id}/keys", framework.Handle(jsonH(s.adminKeys), perm("platform:appkey:list")))
	mux.Handle("POST /api/platform/apps/{id}/keys", framework.Handle(jsonH(s.adminCreateKey), oper("应用密钥", "新增"), perm("platform:appkey:add")))
	mux.Handle("PUT /api/platform/apps/{id}/keys/{keyId}/status", framework.Handle(jsonH(s.adminKeyStatus), oper("应用密钥", "状态变更"), perm("platform:appkey:remove")))
	mux.Handle("DELETE /api/platform/apps/{id}/keys/{keyId}", framework.Handle(jsonH(s.adminDeleteKey), oper("应用密钥", "删除"), perm("platform:appkey:remove")))

	// ---------- 应用用户 ----------
	mux.Handle("GET /api/platform/users", framework.Handle(jsonH(s.adminUsers), perm("platform:user:list")))
	mux.Handle("GET /api/platform/users/export", framework.Handle(csvH("app-users.csv", s.adminExportUsers), perm("platform:user:list")))
	mux.Handle("GET /api/platform/users/{id}", framework.Handle(jsonH(s.adminUser), perm("platform:user:list")))
	mux.Handle("POST /api/platform/users", framework.Handle(jsonH(s.adminCreateUser), oper("应用用户", "新增"), perm("platform:user:add")))
	mux.Handle("PUT /api/platform/users/{id}", framework.Handle(jsonH(s.adminUpdateUser), oper("应用用户", "修改"), perm("platform:user:edit")))
	mux.Handle("PUT /api/platform/users/{id}/status", framework.Handle(jsonH(s.adminUserStatus), oper("应用用户", "状态变更"), perm("platform:user:edit")))
	mux.Handle("PUT /api/platform/users/{id}/password", framework.Handle(jsonH(s.adminResetPassword), oper("应用用户", "重置密码"), perm("platform:user:resetPwd")))
	mux.Handle("DELETE /api/platform/users/{id}", framework.Handle(jsonH(s.adminDeleteUser), oper("应用用户", "删除"), perm("platform:user:remove")))
	mux.Handle("POST /api/platform/users/{id}/counters/{key}/adjust", framework.Handle(jsonH(s.adminAdjustCounter), oper("账户调整", "调整"), perm("platform:counter:adjust")))

	// ---------- 字段配置 ----------
	mux.Handle("GET /api/platform/fields", framework.Handle(jsonH(s.adminFields), perm("platform:field:list")))
	mux.Handle("POST /api/platform/fields", framework.Handle(jsonH(s.adminCreateField), oper("字段配置", "新增"), perm("platform:field:add")))
	mux.Handle("PUT /api/platform/fields/{id}", framework.Handle(jsonH(s.adminUpdateField), oper("字段配置", "修改"), perm("platform:field:edit")))
	mux.Handle("DELETE /api/platform/fields/{id}", framework.Handle(jsonH(s.adminDeleteField), oper("字段配置", "删除"), perm("platform:field:remove")))

	// ---------- 流水与日志 ----------
	mux.Handle("GET /api/platform/counter-logs", framework.Handle(jsonH(s.adminCounterLogs), perm("platform:counter:list")))
	mux.Handle("GET /api/platform/counter-logs/export", framework.Handle(csvH("counter-logs.csv", s.adminExportCounterLogs), perm("platform:counter:list")))
	mux.Handle("GET /api/platform/auth-logs", framework.Handle(jsonH(s.adminAuthLogs), perm("platform:authlog:list")))
	mux.Handle("GET /api/platform/auth-logs/export", framework.Handle(csvH("auth-logs.csv", s.adminExportAuthLogs), perm("platform:authlog:list")))
}

// OpenHandler 返回 /open/v1/* 的处理器（自带 TraceId / 兜底 / 应用与用户认证）。
func OpenHandler(d Deps) http.Handler {
	s := newSvc(d)
	mux := http.NewServeMux()

	// App-Key
	mux.Handle("GET /open/v1/captcha", framework.Handle(jsonH(s.openCaptcha), s.appKey))
	mux.Handle("GET /open/v1/fields", framework.Handle(jsonH(s.openFields), s.appKey))
	mux.Handle("POST /open/v1/auth/register", framework.Handle(jsonH(s.openRegister), s.appKey))
	mux.Handle("POST /open/v1/auth/login", framework.Handle(jsonH(s.openLogin), s.appKey))

	// Bearer
	mux.Handle("POST /open/v1/auth/logout", framework.Handle(jsonH(s.openLogout), s.userToken))
	mux.Handle("POST /open/v1/auth/refresh", framework.Handle(jsonH(s.openRefresh), s.userToken))
	mux.Handle("GET /open/v1/me", framework.Handle(jsonH(s.openMe), s.userToken))
	mux.Handle("PUT /open/v1/me", framework.Handle(jsonH(s.openUpdateMe), s.userToken))
	mux.Handle("PUT /open/v1/me/password", framework.Handle(jsonH(s.openChangePassword), s.userToken))

	// App-Key + Secret
	mux.Handle("POST /open/v1/auth/introspect", framework.Handle(jsonH(s.openIntrospect), s.appSecret))
	mux.Handle("GET /open/v1/users", framework.Handle(jsonH(s.openUsers), s.appSecret))
	mux.Handle("GET /open/v1/users/{id}", framework.Handle(jsonH(s.openUser), s.appSecret))
	mux.Handle("POST /open/v1/users", framework.Handle(jsonH(s.openCreateUser), s.appSecret))
	mux.Handle("PUT /open/v1/users/{id}/profile", framework.Handle(jsonH(s.openUpdateProfile), s.appSecret))
	mux.Handle("PUT /open/v1/users/{id}/status", framework.Handle(jsonH(s.openUserStatus), s.appSecret))
	mux.Handle("GET /open/v1/users/{id}/counters", framework.Handle(jsonH(s.openUserCounters), s.appSecret))
	mux.Handle("POST /open/v1/users/{id}/counters/{key}/change", framework.Handle(jsonH(s.openChangeCounter), s.appSecret))
	mux.Handle("GET /open/v1/users/{id}/counters/{key}/logs", framework.Handle(jsonH(s.openCounterLogs), s.appSecret))

	return framework.Trace(framework.Recover(framework.NotFoundOr405(mux)))
}
