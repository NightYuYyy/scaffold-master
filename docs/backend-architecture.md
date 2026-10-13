# 后端架构说明

后端为 Go 单体服务（`scaffold-go`），标准库 `net/http` 路由，`database/sql` 直写 SQL，没有 ORM 和 Web 框架。两条请求链：

```text
/api/**      控制台接口：TraceId → 兜底 → 后台 JWT 认证（Redis 会话）→ 路由 → [操作日志] → [权限码] → 处理器
/open/v1/**  开放接口：  TraceId → 兜底 → 路由 → 应用密钥 / 用户令牌认证 → 处理器
```

| Go 包 | 职责 | 主要入口 |
| --- | --- | --- |
| `internal/framework` | 统一响应 `R`、错误映射、TraceId、JWT + Redis 会话、验证码、权限与操作日志中间件、`database/sql` 查询辅助 | `response.go`、`security.go`、`audit.go`、`db.go` |
| `internal/system` | 控制台：登录与路由、后台账号（含可管理应用绑定）、角色、菜单、字典、参数、操作与登录日志、日志统计、工作台 | `auth.go`、`management.go`、`monitor.go`、`dashboard.go` |
| `internal/platform` | 平台：应用、接口密钥、应用用户、扩展字段、数值账户与流水、认证日志；开放接口与管理接口 | `platform.go`、`open.go`、`openauth.go`、`counters.go` |
| `internal/generator` | 代码生成（开发工具，默认不在菜单） | `generator.go`、`templates.go` |
| `internal/app` + `cmd/server` | 配置、装配、路由注册、数据库初始化、集成测试 | `app.go`、`dbinit.go` |

## 认证与权限

- **控制台**：登录成功签发 JWT（算法按 `JWT_SECRET` 长度选 HS256 / 384 / 512），Redis `auth:token:<token>` 保存会话，登出撤销；`@RequiresPermission` 等价的 `sec.Perm(code)` 中间件按角色菜单的权限码放行，`super_admin` 直接放行。
- **应用范围**：`super_admin` 可见全部应用；其他后台账号只能访问 `sys_user_app` 里绑定的应用，平台模块的每个查询都按可见应用过滤，越权访问返回 403。
- **开放接口**：`X-App-Key` 识别应用与密钥；服务端接口再校验 `X-App-Secret`。用户令牌是用该密钥 `api_secret` 签发的 HS256 JWT（头部 `kid = api_key`，`aud = app_code`，`sub = 用户 ID`），Redis `open:token:<jti>` 做撤销；停用用户、重置密码、停用密钥都会使令牌失效。业务后端可用 `api_secret` 本地验签。

## 数据

- `sys_*`：控制台自身（账号、角色、菜单、字典、参数、日志）。
- `app_*`：应用、密钥、应用用户（`profile` JSON 存非账户字段）、身份（为手机号 / 微信登录预留）、字段定义、数值账户与流水、认证日志。
- 账户加减在事务内 `SELECT … FOR UPDATE`，`ref_no` 应用内唯一保证幂等；余额与流水用 `DECIMAL(20,4)`。
- 初始化：`sql/V1__init_schema.sql` + `V2__seed_data.sql`，幂等；`scaffold-server init-db` 或 `DB_AUTO_INIT=true` 执行。

## 约定

- 业务错误一律 `framework.Biz("中文提示")`：HTTP 400、`code 400`；认证 401；权限 403；未捕获 500 `系统内部错误`。
- 写操作放在 `framework.Tx` 里；带 `OperLog` 的接口记录脱敏后的请求参数、操作人、IP、耗时与结果。
- 查询结果为 `map[string]any`，键即 SQL 列别名，也就是接口返回的字段名；空列表输出 `[]`。
- 新增业务模块：在 `internal/<module>` 下按模型 / SQL / 服务 / 路由四个文件组织（代码生成器可生成骨架），在 `internal/app/app.go` 注册路由，在 `sql/V2` 增加菜单与权限码。
