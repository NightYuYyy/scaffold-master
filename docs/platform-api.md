# 用户中心平台接口契约

两组接口：**开放接口** `/open/v1/*` 给接入方（应用的前端与后端）调用；**管理接口** `/api/platform/*` 给运营控制台调用。响应信封与后台其他接口一致：`{ code, message, data, traceId }`，成功 `code = 200`；错误时 HTTP 400 且 `code` 为业务码，`message` 为中文提示；未认证 401，无权限 403。分页统一 `pageNum` / `pageSize`（默认 1 / 10，最多 100），返回 `{ rows, total, pageNum, pageSize }`。

## 一、概念

| 概念 | 表 | 说明 |
| --- | --- | --- |
| 应用 | `app_info` | 接入平台的一个业务项目；`app_code` 唯一，停用后其开放接口全部拒绝 |
| 接口密钥 | `app_api_key` | 一个应用多把密钥。`api_key` 公开（前端可带），`api_secret` 只给应用后端 |
| 应用用户 | `app_user` | C 端用户，按 `app_id` 隔离；账号在应用内唯一 |
| 扩展字段 | `app_field` | 运营给应用配置的字段：`string / number / bool / date / enum / counter`；非 counter 的值存在 `app_user.profile`（JSON） |
| 数值账户 | `app_counter` + `app_counter_log` | `counter` 类型字段的余额与流水，原子加减，`refNo` 幂等 |
| 认证日志 | `app_auth_log` | 注册、登录、登出、刷新及失败记录 |
| 后台账号可管理的应用 | `sys_user_app` | `super_admin` 不受限；其他账号只看得到绑定的应用 |

## 二、开放接口 `/open/v1`

### 认证方式

| 方式 | 请求头 | 适用 |
| --- | --- | --- |
| 应用标识 | `X-App-Key: <api_key>` | 验证码、注册、登录、字段定义等公开接口；应用前端可直接带 |
| 应用密钥 | `X-App-Key` + `X-App-Secret: <api_secret>` | 服务端接口（用户管理、账户加减、令牌内省）；只能由应用后端调用 |
| 用户令牌 | `Authorization: Bearer <token>` | 用户自己的接口（我的信息、改资料、改密码、登出、刷新） |

规则：密钥或应用停用、应用已删除 → 401 `应用或密钥无效`；需要 secret 的接口 secret 不匹配 → 401 `应用密钥错误`；用户 token 无效、过期或已登出 → 401 `未认证或登录已过期`；用户已停用 → 401 `账号已停用`。

### 用户令牌

HS256 JWT，用登录时所用密钥的 `api_secret` 签名，头部 `kid = api_key`。声明：`iss = "scaffold-platform"`，`aud = app_code`，`sub = 应用用户 ID（字符串）`，`jti`，`iat`，`exp`（`iat + app_info.token_expires_seconds`）。平台在 Redis 保存 `open:token:<jti>` → 用户 ID，TTL 同 exp；登出、刷新、停用用户、重置密码时删除。

用户撤销索引 `open:user:<appId>:<userId>` 的 TTL 只延长、不缩短；调整应用有效期只影响新签发令牌，旧长效令牌仍可被改密、停用等操作撤销。

应用后端可用 `api_secret` 本地验签（校验签名、`exp`、`aud`），`sub` 即用户 ID；需要感知登出的场景调用内省接口。

### 接口清单

| 方法与路径 | 认证 | 说明 |
| --- | --- | --- |
| `GET /open/v1/captcha` | App-Key | 返回 `{ uuid, code }`（与后台一致，由客户端展示；`captcha_enabled = 0` 的应用可不调用） |
| `GET /open/v1/fields` | App-Key | 字段定义列表（启用的），供注册 / 资料表单渲染 |
| `POST /open/v1/auth/register` | App-Key | 注册 |
| `POST /open/v1/auth/login` | App-Key | 登录 |
| `POST /open/v1/auth/logout` | Bearer | 登出，撤销当前 token |
| `POST /open/v1/auth/refresh` | Bearer | 换发新 token，旧 token 立即失效 |
| `GET /open/v1/me` | Bearer | 我的信息 |
| `PUT /open/v1/me` | Bearer | 改昵称、头像与 `user_editable = 1` 的字段 |
| `PUT /open/v1/me/password` | Bearer | 改密码，成功后当前 token 失效需重新登录 |
| `POST /open/v1/auth/introspect` | App-Key + Secret | 令牌内省 |
| `GET /open/v1/users` | App-Key + Secret | 分页查用户：`pageNum`、`pageSize`、`keyword`（账号 / 昵称模糊）、`status` |
| `GET /open/v1/users/{id}` | App-Key + Secret | 用户详情 |
| `POST /open/v1/users` | App-Key + Secret | 服务端建用户（免验证码，可带任意字段） |
| `PUT /open/v1/users/{id}/profile` | App-Key + Secret | 改任意非 counter 字段（不受 `user_editable` 限制） |
| `PUT /open/v1/users/{id}/status` | App-Key + Secret | 启用 / 停用；停用时撤销其全部 token |
| `GET /open/v1/users/{id}/counters` | App-Key + Secret | 该用户全部数值账户余额 |
| `POST /open/v1/users/{id}/counters/{key}/change` | App-Key + Secret | 账户加减 |
| `GET /open/v1/users/{id}/counters/{key}/logs` | App-Key + Secret | 该账户流水，分页 |

### 请求与响应字段

**注册** `POST /auth/register`

```json
{ "username": "alice", "password": "secret1", "nickname": "Alice", "captcha": "7N4C", "captchaUuid": "…", "profile": { "inviteCode": "WELCOME" } }
```

- `username`：3–64 位，仅字母、数字、`_ . @ -`；应用内唯一，重复 → `账号已存在`。
- `password`：6–64 位。`nickname` 缺省取 `username`。
- `profile`：只允许已启用、非 counter 且 `userEditable = 1`（允许用户填写）的字段；提交只读字段 → 403 `字段 xxx 不允许用户修改`，不会创建用户。`required = 1` 的字段必须提供（有默认值的除外）；只读必填字段应配置默认值，否则只能由可信服务端 / 后台建用户。未定义的键 → `未定义的字段: xxx`；类型不符 → `字段 xxx 格式不正确`。
- 应用 `allow_register = 0` → `该应用未开放注册`；`captcha_enabled = 1` 时校验验证码 → `验证码错误或已过期`。
- 成功返回同登录。

**登录** `POST /auth/login`：`{ username, password, captcha, captchaUuid }` → 账号或密码错误统一提示 `账号或密码错误`；停用 → `账号已停用`。响应：

```json
{ "token": "…", "expiresIn": 604800, "user": { …同 /me 的 data… } }
```

**我的信息** `GET /me` 响应 `data`：

```json
{
  "id": 1, "username": "demo_user", "nickname": "示例用户", "avatar": null, "status": 1,
  "profile": { "level": "bronze", "inviteCode": "WELCOME", "birthday": null },
  "counters": { "coins": 100 },
  "createTime": "2026-10-13 15:00", "lastLoginTime": "2026-10-13 15:30"
}
```

`profile` 包含该应用全部启用的非 counter 字段：未填的给默认值，无默认值给 `null`；`number` 以数字返回，`bool` 以布尔返回，`date` 为 `yyyy-MM-dd` 字符串。`counters` 为全部 counter 字段的余额（数字），没有记录时为 `0`。

**改资料** `PUT /me`：`{ nickname?, avatar?, profile?: { … } }`，`profile` 只接受 `user_editable = 1` 的字段，其余 → 403 `字段 xxx 不允许用户修改`。

**改密码** `PUT /me/password`：`{ oldPassword, newPassword }` → `旧密码不正确` / `新密码不能与旧密码一致`。

**内省** `POST /auth/introspect`：`{ "token": "…" }` → `{ "active": true, "userId": 1, "username": "…", "expiresAt": "2026-10-20 15:00:00" }`；无效时 `active = false` 其余为 null，HTTP 仍 200。

**字段定义** `GET /fields` 响应 `data` 数组元素：`{ key, label, type, required, userEditable, defaultValue, options, sort }`（`options`：enum 为字符串数组，counter 为 `{ allowNegative, max }`，其他为 null）。

**服务端建用户** `POST /users`：`{ username, password?, nickname?, avatar?, status?, profile? }`；`password` 缺省取参数 `app.user.initPassword`。

**用户列表 / 详情**：元素同 `/me` 的 data（不含 token）。

**账户加减** `POST /users/{id}/counters/{key}/change`

```json
{ "amount": -10, "reason": "兑换商品", "refNo": "order-20261013-001" }
```

- `key` 必须是该应用 `counter` 类型且启用的字段 → 否则 `账户不存在`。
- `amount`：非 0 数字，最多 4 位小数。`reason` 必填 ≤ 200 字。`refNo` 必填 ≤ 64 字，应用内唯一。
- 幂等：同一 `refNo` 再次提交，若用户、账户字段与 `amount` 均与首次相同则直接返回首次结果（HTTP 200）；不同 → `幂等号已使用`。
- 余额不足（`allowNegative = false` 且结果为负）→ `余额不足`；超过 `max` → `超出账户上限`。
- 响应：`{ "balance": 90, "logId": 12, "duplicated": false }`。
- 事务内先创建或排他锁定账户行（不存在按 0 创建），再 `SELECT … FOR UPDATE`，同一账户的并发首次入账与重复请求串行校验，避免空账户间隙锁死锁。

**流水** `GET /users/{id}/counters/{key}/logs` 元素：`{ id, fieldKey, amount, balanceAfter, reason, refNo, operator, createTime }`，按 id 倒序。

### 认证日志

注册、登录、登出、刷新、登录失败、注册失败都写 `app_auth_log`（含 IP），平台后台可查看。

## 三、管理接口 `/api/platform`

全部需要后台登录；权限码见括号。**应用范围**：`super_admin` 可见全部应用；其他账号只能访问 `sys_user_app` 绑定的应用，列表自动过滤，访问未绑定应用的记录 → 403。

### 应用

| 方法与路径 | 权限 | 说明 |
| --- | --- | --- |
| `GET /apps` | `platform:app:list` | 分页；`keyword`（编码 / 名称）、`status`。元素 `{ id, code, name, status, allowRegister, captchaEnabled, tokenExpiresSeconds, remark, userCount, keyCount, createTime }` |
| `GET /apps/options` | 登录即可 | 当前账号可见的应用 `[{ id, code, name }]`，供各页面的应用筛选下拉 |
| `GET /apps/{id}` | `platform:app:list` | 详情 |
| `POST /apps` | `platform:app:add` | `{ code, name, status?, allowRegister?, captchaEnabled?, tokenExpiresSeconds?, remark? }`；`code` 3–64 位小写字母数字下划线，唯一 → `应用编码已存在`。返回 `{ id }` |
| `PUT /apps/{id}` | `platform:app:edit` | 同上，`code` 不可改 |
| `PUT /apps/{id}/status` | `platform:app:edit` | `{ status }` |
| `DELETE /apps/{id}` | `platform:app:remove` | 软删除；应用下仍有未删除用户 → `应用下仍有用户，不能删除` |
| `GET /apps/{id}/keys` | `platform:appkey:list` | `[{ id, name, apiKey, secretMasked, status, lastUsedTime, createTime }]`，`secretMasked` 形如 `demo-se****6789` |
| `POST /apps/{id}/keys` | `platform:appkey:add` | `{ name }` → `{ id, apiKey, apiSecret }`，**明文 secret 只在此处返回一次** |
| `PUT /apps/{id}/keys/{keyId}/status` | `platform:appkey:remove` | `{ status }`；停用后该密钥签发的 token 全部失效（验签取不到密钥） |
| `DELETE /apps/{id}/keys/{keyId}` | `platform:appkey:remove` | 硬删除 |
| `GET /apps/export` | `platform:app:list` | CSV：编码、名称、状态、用户数、密钥数、创建时间 |

`apiKey` 生成规则：`<code>_<16 位随机小写字母数字>`；`apiSecret`：48 位随机字母数字。

### 应用用户

| 方法与路径 | 权限 | 说明 |
| --- | --- | --- |
| `GET /users` | `platform:user:list` | 分页；`appId`（必填，否则取可见应用的第一个），`keyword`，`status`。元素 `{ id, appId, username, nickname, avatar, status, profile, counters, registerIp, lastLoginTime, createTime }` |
| `GET /users/{id}` | `platform:user:list` | 详情（同元素） |
| `POST /users` | `platform:user:add` | `{ appId, username, password?, nickname?, status?, profile? }`，`password` 缺省取参数 `app.user.initPassword` |
| `PUT /users/{id}` | `platform:user:edit` | `{ nickname?, avatar?, profile? }`（任意非 counter 字段） |
| `PUT /users/{id}/status` | `platform:user:edit` | `{ status }`；停用撤销 token |
| `PUT /users/{id}/password` | `platform:user:resetPwd` | `{ password }` 6–64 位；撤销 token |
| `DELETE /users/{id}` | `platform:user:remove` | 软删除；撤销 token |
| `POST /users/{id}/counters/{key}/adjust` | `platform:counter:adjust` | `{ amount, reason }`，`refNo` 自动生成 `admin-<uuid>`，`operator = admin:<账号>` |
| `GET /users/export` | `platform:user:list` | CSV：账号、昵称、状态、各启用字段（含账户余额）、注册时间、最近登录 |

### 字段配置

| 方法与路径 | 权限 | 说明 |
| --- | --- | --- |
| `GET /fields?appId=` | `platform:field:list` | 该应用全部字段（含停用），按 `sort, id`。元素 `{ id, appId, key, label, type, required, userEditable, defaultValue, options, sort, status, remark, createTime }` |
| `POST /fields` | `platform:field:add` | `{ appId, key, label, type, required?, userEditable?, defaultValue?, options?, sort?, status?, remark? }`；`key` 1–64 位字母数字下划线且字母开头，应用内唯一 → `字段键已存在`；`type` 不在六种之内 → `字段类型不正确`；enum 必须给非空 `options` 数组；counter 的 `userEditable` 强制为 0 |
| `PUT /fields/{id}` | `platform:field:edit` | 同上；`key` 与 `type` 不可改 |
| `DELETE /fields/{id}` | `platform:field:remove` | 硬删除定义；用户 profile 里的旧值保留不动，counter 的余额与流水保留 |

### 流水与日志

| 方法与路径 | 权限 | 说明 |
| --- | --- | --- |
| `GET /counter-logs` | `platform:counter:list` | 分页；`appId`（必填）、`userId?`、`fieldKey?`、`keyword?`（原因 / 幂等号 / 账号）。元素 `{ id, appId, userId, username, fieldKey, amount, balanceAfter, reason, refNo, operator, createTime }` |
| `GET /counter-logs/export` | `platform:counter:list` | CSV |
| `GET /auth-logs` | `platform:authlog:list` | 分页；`appId`（必填）、`event?`、`status?`、`keyword?`（账号 / IP）。元素 `{ id, appId, userId, username, event, status, ip, msg, eventTime }` |
| `GET /auth-logs/export` | `platform:authlog:list` | CSV |

### 后台账号扩展

`/api/system/users` 的新增 / 编辑请求和列表行增加 `appIds: number[]`（可管理的应用），`users/options` 增加 `apps: [{ id, code, name }]`。超级管理员忽略该字段。

### 工作台

`GET /api/dashboard/summary?range=24h|7d|30d` 响应 `data`：

```json
{
  "stats": [
    { "key": "apps", "label": "应用", "value": 3, "sub": "已启用 3" },
    { "key": "users", "label": "应用用户", "value": 1280, "sub": "较上一周期 +12.5%", "delta": 12.5, "up": true, "fresh": false },
    { "key": "registrations", "label": "新注册", "value": 36, "sub": "本周期", "delta": 8.0, "up": false, "fresh": false },
    { "key": "logins", "label": "登录次数", "value": 420, "sub": "成功率 98.1%", "delta": 3.2, "up": true, "fresh": false },
    { "key": "operators", "label": "后台账号", "value": 2, "sub": "已启用 2" }
  ],
  "range": "24h",
  "trend": { "labels": ["13:00", "…"], "registrations": [0, 1, …], "logins": [3, 5, …] },
  "apps": [{ "id": 1, "code": "demo", "name": "示例应用", "users": 1280, "registrations": 36, "logins": 420 }],
  "activities": [{ "app": "示例应用", "who": "demo_user", "action": "登录", "time": "2026-10-13 15:30", "tint": "ok" }],
  "counters": [{ "app": "示例应用", "fieldKey": "coins", "label": "积分", "total": 1200, "changes": 15 }]
}
```

`range = 24h` 按小时 24 桶（标签 `HH:00`），`7d` / `30d` 按天（标签 `MM-dd`）；时间边界取数据库时钟。非超级管理员只统计可见应用。`delta` 为与上一等长周期的环比百分比，上一周期为 0 时 `fresh = true`。

## 四、错误码约定

开放接口与管理接口的业务错误统一 HTTP 400，`code = 400`，`message` 为中文；认证类 401；权限与应用范围 403；资源不存在用业务提示（如 `用户不存在`）。未捕获异常 500 `系统内部错误`。
