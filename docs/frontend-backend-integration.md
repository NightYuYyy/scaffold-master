# 前后端接口约定（控制台）

控制台前端统一通过 `src/api` 请求 `/api`，开发服务器把 `/api` 与 `/open` 代理到本地后端。开放接口（给接入方）与平台管理接口的完整字段见 [platform-api.md](platform-api.md)。

## 响应与分页

普通 JSON 响应为 `code`（结果码）、`message`（提示）、`data`（业务内容）、`traceId`（追踪 ID）；HTTP 与业务码均成功时客户端返回 `data`，错误抛出 `ApiError`。CSV 与 ZIP 使用文件响应。

| 字段 | 含义 |
| --- | --- |
| `pageNum` / `pageSize` | 页码 / 每页数量；默认 1 / 10，每页最多 100 |
| `total` / `rows` | 总记录数 / 当前页数据 |
| `id` / `parentId` | 当前记录 ID / 上级记录 ID |
| `username` / `nickname` | 登录账号 / 姓名或昵称 |
| `status` | 启用或成功为 1，停用或失败为 0 |
| `permission` / `permissions` | 单个操作权限码 / 当前用户权限码列表 |
| `appIds` / `appNames` | 后台账号可管理的应用 ID 列表 / 名称（超级管理员不受限） |
| `roleIds` / `menuIds` | 已选角色 / 菜单 ID 列表 |
| `createTime` / `updateTime` | 创建 / 更新时间，`yyyy-MM-dd HH:mm` |

## 核心调用

| 页面 / 公共能力 | API 文件 | 后端路径（均带 /api 前缀） |
| --- | --- | --- |
| 登录、当前用户、菜单、密码 | `auth.ts` | `/auth/captcha`、`/auth/login`、`/auth/me`、`/auth/routes`、`/auth/logout`、`/auth/password` |
| 后台账号、角色、菜单、字典、参数 | `system.ts` | `/system/users`、`/system/roles`、`/system/menus`、`/system/dict`、`/system/configs` |
| 账号引用选项 | `system.ts` 的 `users.options` | `/system/users/options` → `roles`、`apps`、`initPassword` |
| 应用、密钥、应用用户、字段、流水、认证日志 | `platform.ts` | `/platform/apps`、`/platform/apps/{id}/keys`、`/platform/users`、`/platform/fields`、`/platform/counter-logs`、`/platform/auth-logs` |
| 可见应用下拉 | `platform.ts` 的 `appsOptions` | `/platform/apps/options` |
| 操作日志、登录日志与统计 | `system.ts` | `/monitor/oper-logs`、`/monitor/login-logs`、各自的 `/stats`、`/timeline`、`/heatmap` |
| 工作台 | `portal.ts` | `/dashboard/summary?range=24h|7d|30d` |
| 服务健康（登录页） | `portal.ts` 的 `health` | `/health`（匿名） |

## 登录与权限

登录请求包含 `username`、`password`、`captcha`、`captchaUuid`，成功返回 `token`、`expiresIn`，后续请求附加 Bearer token。`/auth/me` 返回当前账号、启用角色与权限码，`/auth/routes` 返回授权菜单树；前端只对 `COMPONENT_MAP` 注册的组件生成导航与路由，无菜单进入 `/no-access`。

按钮权限与后端 `Perm` 中间件使用同一权限码；应用范围（后台账号绑定的应用）由后端在平台接口里执行，前端的应用下拉只列可见应用。退出、401 与新登录共用清理逻辑；旧 token 的 401 不清除新 token。

## 扩展字段在控制台的呈现

应用用户页按 `GET /platform/fields?appId=` 的定义动态生成列与表单：`string` 文本框、`number` 数字框、`bool` 开关、`date` 日期、`enum` 下拉（`options`）、`counter` 只读余额并通过「账户调整」加减。`profile` 中缺省值由后端按字段默认值补齐，前端不做推断。
