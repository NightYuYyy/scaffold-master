# scaffold-go

用户中心平台的后端：开放接口（`/open/v1`，给接入的业务项目）、管理接口（`/api/platform`，给控制台）、控制台自身的登录与权限（`/api/system`、`/api/auth`）。标准库 `net/http` 路由 + `database/sql`，依赖只有 MySQL 驱动、go-redis、golang-jwt、bcrypt。

## 目录

| 包 | 内容 |
| --- | --- |
| `internal/framework` | 统一响应、错误映射、TraceId、JWT + Redis 会话、验证码、权限与操作日志中间件、`database/sql` 查询辅助 |
| `internal/system` | 控制台：认证与路由、后台账号（含可管理应用绑定）、角色、菜单、字典、参数、操作与登录日志、日志统计、工作台 |
| `internal/platform` | 应用、接口密钥、应用用户、扩展字段、数值账户与流水、认证日志；开放接口与管理接口 |
| `internal/generator` | 代码生成器（输出本工程风格的 Go 骨架 + 前端骨架 + 菜单 SQL），默认不在菜单 |
| `internal/app` + `cmd/server` | 环境变量配置、依赖装配、路由注册、数据库初始化、集成测试 |

架构与约定见 [docs/backend-architecture.md](../docs/backend-architecture.md)，接口契约见 [docs/platform-api.md](../docs/platform-api.md)。

## 运行

环境变量见根目录 README。

```bash
export JWT_SECRET="$(openssl rand -hex 32)"
go run ./cmd/server init-db     # 建库建表 + 初始数据（幂等）
go run ./cmd/server             # 监听 :8080
```

`DB_AUTO_INIT=true` 可在每次启动前执行初始化（容器编排里默认开启）。构建单文件二进制：`go build -o scaffold-server ./cmd/server`；镜像见本目录 `Dockerfile`（构建上下文为仓库根目录）。

## 验证

```bash
# 需要本地 MySQL（库 scaffold_system_test 已用 ../sql 初始化）与 Redis
go test ./... -count=1
```

集成测试在 `internal/app/*_test.go`，用 `httptest` 启动完整应用，覆盖控制台接口、权限与应用范围、开放接口的注册登录与账户流水。

## 实现细节

- 业务错误统一 `framework.Biz("中文提示")` → HTTP 400 + `code 400`；认证 401；权限与应用范围 403。
- 三个中间件的顺序：操作日志在最外层，权限校验在内层；参数绑定 / 校验失败不记操作日志。
- 请求体按 Jackson 风格宽松转换（小数→整数截断、数字字符串→数字、空串→null），字段名大小写敏感，字面量 `null` 返回 400。
- 查询结果用 `map[string]any`，键为 SQL 列别名，整数列 `int64`、DECIMAL `json.Number`、NULL `null`；空列表输出 `[]`。
- DSN 带 `clientFoundRows=true`：UPDATE 影响行数为匹配行数，`update == 0 → 不存在` 的判断依赖它。
- 用户令牌：控制台用 `JWT_SECRET`（按长度选 HS256 / 384 / 512），应用用户用各自密钥的 `api_secret`（HS256，头部 `kid`），都在 Redis 里保存可撤销的会话。
