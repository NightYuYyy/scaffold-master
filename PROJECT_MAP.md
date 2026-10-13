# 用户中心平台项目地图

> 2026-10-13 更新。用于定位页面、接口、SQL 与权限码；接口字段以 `docs/platform-api.md` 为准。

## 一、项目总览

| 端 | 路径 | 技术栈 | 本地入口 |
| --- | --- | --- | --- |
| 后端 | `scaffold-go` | Go 1.26+ 标准库 `net/http` + `database/sql`、MySQL 8、Redis、JWT | `8080`；控制台接口前缀 `/api`，开放接口前缀 `/open/v1` |
| 控制台 | `scaffold-vue` | Vue 3、TypeScript、Vite、Vue Router、GSAP，自有组件 | `5173`，开发代理 `/api` 与 `/open` |
| 数据库 | `sql/` | `V1__init_schema.sql` 21 张表，`V2__seed_data.sql` 初始数据与示例应用 | `go run ./cmd/server init-db` |
| 部署 | `deploy/`、`.github/workflows/docker.yml` | docker compose（MySQL、Redis、后端、前端 nginx）、GHCR 自动构建 | `docker compose up -d` |

## 二、术语词典

| 说法 | 表 | 关键字段 |
| --- | --- | --- |
| 应用（接入的业务项目） | `app_info` | `app_code` 编码（token 的 aud）；`allow_register` 是否开放注册；`captcha_enabled`；`token_expires_seconds` |
| 接口密钥 | `app_api_key` | `api_key` 公开标识（请求头 `X-App-Key`，token 的 kid）；`api_secret` 服务端密钥（`X-App-Secret`，签发用户 token） |
| 应用用户（C 端） | `app_user` | `username` 应用内唯一；`profile` JSON 扩展字段值；`status` |
| 登录身份（预留） | `app_user_identity` | `provider` phone / email / wechat；`identifier` |
| 扩展字段 | `app_field` | `field_key` 键名；`field_type` string / number / bool / date / enum / counter；`required`；`user_editable`；`options` |
| 数值账户与流水 | `app_counter`、`app_counter_log` | `balance` 余额；`amount` 变动；`ref_no` 幂等号；`operator` 操作方 |
| 认证日志 | `app_auth_log` | `event` register / login / logout / refresh / login_failed / register_failed |
| 后台账号 | `sys_user`、`sys_user_role`、`sys_user_app` | 运营人员与应用管理员；`sys_user_app` 限定可管理的应用（super_admin 不受限） |
| 角色 / 菜单 / 权限码 | `sys_role`、`sys_menu`、`sys_role_menu` | `menu_type` dir / menu / btn；`permission` 权限码 |
| 字典 / 参数 | `sys_dict_type`、`sys_dict_data`、`sys_config` | `app_field_type` 字典；`app.user.initPassword` 应用用户默认密码 |
| 后台审计 | `sys_oper_log`、`sys_login_log` | 控制台写操作与登录记录 |

## 三、模块文件地图

| 后端模块 | 文件 | 接口前缀 | 表 |
| --- | --- | --- | --- |
| 控制台认证与路由 | `internal/system/auth.go` | `/api/auth` | `sys_user`、`sys_role`、`sys_menu`、`sys_login_log` |
| 后台账号 / 角色 / 菜单 / 字典 / 参数 / 日志 | `internal/system/management.go`、`management_sql.go` | `/api/system`、`/api/monitor` | `sys_*` |
| 日志统计与泳道 | `internal/system/monitor.go` | `/api/monitor/*/stats` | `sys_oper_log`、`sys_login_log` |
| 工作台 | `internal/system/dashboard.go` | `/api/dashboard/summary` | `app_*`、`sys_user` |
| 平台管理接口 | `internal/platform/apps.go`、`users.go`、`fields.go`、`counters.go`、`logs.go` | `/api/platform` | `app_*`、`sys_user_app` |
| 开放接口 | `internal/platform/open.go`、`openauth.go` | `/open/v1` | `app_*` |
| 代码生成（开发工具） | `internal/generator` | `/api/tool/generator` | `gen_*` |

| 控制台页面 | 路由 | 页面文件 | API 文件 |
| --- | --- | --- | --- |
| 登录 | `/login` | `views/Login.vue` | `api/auth.ts` |
| 工作台 | `/dashboard` | `views/Dashboard.vue` | `api/portal.ts` |
| 应用管理（含密钥抽屉） | `/platform/app` | `views/platform/Apps.vue` | `api/platform.ts` |
| 应用用户（动态字段列、账户调整、详情抽屉） | `/platform/user` | `views/platform/AppUsers.vue` | `api/platform.ts` |
| 字段配置 | `/platform/field` | `views/platform/Fields.vue` | `api/platform.ts` |
| 账户流水 | `/platform/counter-log` | `views/platform/CounterLogs.vue` | `api/platform.ts` |
| 认证日志 | `/platform/auth-log` | `views/platform/AuthLogs.vue` | `api/platform.ts` |
| 后台账号（含可管理应用） | `/system/user` | `views/system/Users.vue` | `api/system.ts` |
| 角色 / 菜单 / 字典 / 参数 | `/system/role` `/system/menu` `/system/dict` `/system/config` | `views/system/*.vue` | `api/system.ts` |
| 操作日志 / 登录日志 | `/monitor/oper-log` `/monitor/login-log` | `views/monitor/*.vue` | `api/system.ts` |

## 四、场景剧本

**接入一个新项目**：控制台「应用管理」新建应用 → 「密钥」新增密钥（secret 只显示一次）→ 「字段配置」加字段 → 业务前端带 `X-App-Key` 调 `/open/v1/auth/register`、`/auth/login` → 业务后端用 `api_secret` 本地验签 token，或带 `X-App-Secret` 调 `/open/v1/users/*`。

**用户注册**：`POST /open/v1/auth/register` → 校验应用状态、`allow_register`、验证码、账号规则、字段定义（用户可编辑 / 必填 / 类型 / 枚举）→ 写 `app_user`（`profile`）→ 签发 token（Redis `open:token:<jti>` 与只延长 TTL 的用户撤销索引）→ 写 `app_auth_log` → 返回 `{ token, expiresIn, user }`。

**积分加减**：业务后端 `POST /open/v1/users/{id}/counters/coins/change { amount, reason, refNo }` → 事务内锁 `app_counter` 行 → 校验幂等号、余额下限与上限 → 更新余额 → 写 `app_counter_log` → 返回 `{ balance, logId, duplicated }`。

**应用管理员登录控制台**：`appadmin` 登录 → 菜单只有工作台与应用平台 → 所有平台接口按 `sys_user_app` 过滤 → 访问未绑定应用返回 403。

**边界回归**：`scaffold-go/internal/app/platform_boundary_test.go` 覆盖公开注册只读字段、缩短令牌有效期后的撤销索引、12 请求并发首次入账与幂等；运行 `go test ./internal/app -run 'TestPlatform(RegistrationCannot|ShorterToken|ConcurrentFirst)' -count=3`。

## 五、权限码速查

| 模块 | 权限码 |
| --- | --- |
| 应用 | `platform:app:list/add/edit/remove`，密钥 `platform:appkey:list/add/remove` |
| 应用用户 | `platform:user:list/add/edit/remove/resetPwd`，账户 `platform:counter:adjust/list` |
| 字段 | `platform:field:list/add/edit/remove` |
| 认证日志 | `platform:authlog:list` |
| 后台账号 / 角色 / 菜单 / 字典 / 参数 | `system:user`、`system:role`、`system:menu`、`system:dict`、`system:config`，后缀 `list/add/edit/remove`（账号另有 `resetPwd`） |
| 后台日志 | `monitor:operlog:list/remove`、`monitor:loginlog:list/remove` |
| 代码生成 | `tool:gen:list`（默认不在菜单） |

## 六、改动影响链

| 变更 | 必查位置 |
| --- | --- |
| 开放接口字段 | `docs/platform-api.md` → `internal/platform/open.go` / `dto.go` → `internal/app/platform_*_test.go` → 接入方 |
| 扩展字段类型 | `app_field.field_type` 校验（`fields.go`）→ `/me` 与管理接口的 profile 规范化 → 控制台动态列与表单（`AppUsers.vue`、`Fields.vue`）→ 字典 `app_field_type` |
| 账户规则 | `counters.go`（锁、幂等、下限上限）→ 流水页与导出 → 工作台 `counters` |
| 应用范围 | `sys_user_app` → `internal/platform/scope.go` → 每个平台查询与写入 → 后台账号页「可管理的应用」 |
| 改权限 | `sql/V2` 菜单与权限码 → `sec.Perm` → `PermissionButton` → 低权限账号回归 |
| 改表 | `sql/V1`（幂等）→ SQL 别名 → DTO → 契约文档 → 页面 |
| 改部署 | `scaffold-go/Dockerfile`、`scaffold-vue/Dockerfile` / `nginx.conf.template`、`deploy/docker-compose.yml`、`.github/workflows/docker.yml` → 本地 compose 验证 |

## 七、症状定位表

| 症状 | 第一现场 | 排查重点 |
| --- | --- | --- |
| 开放接口 401 `应用或密钥无效` | `openauth.go` | 密钥或应用停用、应用已删除、请求头名称 |
| 用户 token 验签失败 | `openauth.go`、业务后端验签代码 | `kid` 对应密钥是否停用、`aud` 是否为应用编码、是否用了该密钥的 secret |
| 注册报 `未定义的字段` | `fields.go` | 字段未启用或键名大小写 |
| 账户加减 `幂等号已使用` | `counters.go` | 同一 `refNo` 用了不同金额 |
| 应用管理员看不到数据 | `scope.go`、`sys_user_app` | 账号是否绑定应用、角色菜单授权 |
| 登录成功但没有菜单 | `auth.go`、`NoAccess.vue` | 角色启用状态、菜单授权、组件是否在 `COMPONENT_MAP` |
| 时间差 8 小时 | DSN `time_zone`、容器 `TZ` | `DB_TIMEZONE` 与部署机时区一致 |
