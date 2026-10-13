# scaffold-go

`scaffold-system`（Java / Spring Boot）的 Go 等价实现。接口路径、JSON 字段、错误码与中文提示、CSV / ZIP 响应、权限码与数据范围语义与 Java 版一致，`scaffold-vue` 前端无需改动即可直接切换。

## 目录

| 目录 | 对应 Java 模块 | 内容 |
| --- | --- | --- |
| `internal/framework` | `scaffold-common` + `scaffold-framework` | 统一响应 `R`、异常映射、TraceId、JWT + Redis 会话、验证码、`@RequiresPermission` / `@DataScope` / `@OperLog` 三个中间件、`database/sql` 查询辅助 |
| `internal/system` | `scaffold-system` | 认证与路由、用户 / 角色 / 菜单 / 部门 / 岗位 / 字典 / 参数、操作与登录日志、日志统计与泳道、工作台 |
| `internal/generator` | `scaffold-generator` | 真实表导入、生成配置、7 个文件预览、ZIP 下载 |
| `internal/app` + `cmd/server` | `scaffold-admin` | 环境变量配置、依赖装配、路由注册、启动入口、集成测试 |

依赖只有四个：`go-sql-driver/mysql`、`redis/go-redis`、`golang-jwt/jwt`、`golang.org/x/crypto`（bcrypt）。路由用 Go 1.22+ 标准库 `net/http.ServeMux`，没有 ORM 和 Web 框架。

## 运行

环境变量与 Java 版完全相同（见根目录 README）：`JWT_SECRET`（必填，≥32 字节）、`DB_HOST` / `DB_PORT` / `DB_NAME` / `DB_USERNAME` / `DB_PASSWORD` / `DB_TIMEZONE`、`REDIS_HOST` / `REDIS_PORT` / `REDIS_PASSWORD`。额外支持 `SERVER_PORT`（默认 8080）。数据库沿用 `../scaffold-system/sql/V1__init_schema.sql` 与 `V2__seed_data.sql`，密码哈希、Redis 键名均兼容，可与 Java 版连同一套库。

```bash
export JWT_SECRET="$(openssl rand -hex 32)"
go run ./cmd/server
```

构建单文件二进制：

```bash
go build -o scaffold-server ./cmd/server
```

数据库初始化（对应 Java 的 `DbInit`）：`scaffold-server init-db` 按版本顺序导入 `SQL_DIR`（默认 `../scaffold-system/sql`）下的 `V*__*.sql`，建库不存在时先建库；设置 `DB_AUTO_INIT=true` 则在每次启动前执行，脚本幂等。容器编排见根目录 `deploy/`，镜像 `Dockerfile` 在本目录（构建上下文为仓库根目录）。

## 验证

```bash
# 集成测试：需要本地 MySQL（库 scaffold_system_test 已导入 V1/V2）与 Redis
go test ./internal/app/ -count=1

# 与 Java 版逐接口对照（两边连同一个库和 Redis，Java 跑在 8081）
python scripts/compare-backends.py --java http://localhost:8081 --go http://localhost:8080
```

## 与 Java 版的刻意对应关系

- `BusinessException(code, msg)` → `framework.BizCode`：HTTP 仍为 400，`body.code` 取异常 code，与 `GlobalExceptionHandler` 一致。
- 三个切面的顺序：操作日志在最外层，权限校验在内层；参数绑定 / 校验失败（Spring 中发生在切面之前）不记操作日志。
- 查询结果使用 `map[string]any`，键为 SQL 列别名原样，整数列为 `int64`、DECIMAL 为 `json.Number`、NULL 为 `null`，与 MyBatis `Map<String,Object>` + Jackson 输出一致。
- 代码生成器输出 Go 骨架：`backend/internal/<module>/<table>_model.go`（结构体）、`_sql.go`（计数与分页查询，用 `framework.SelectRows`）、`_service.go`（`List`）、`_handler.go`（`Register<Class>Routes`，带权限码），以及前端 API、页面占位和菜单 SQL；四个 Go 文件经 `go/format` 格式化。导入时记录的字段类型仍沿用原 `java_type` 列与前端「字段映射」展示，渲染时映射为 Go 类型（Long→int64、Integer→int、BigDecimal→float64、LocalDateTime/String→string）。

## 已知差异（前端不会触发）

- 参数绑定失败且调用方无权限时：Spring 在进入权限切面前就完成参数绑定，返回 400 `参数类型错误: x` 且不记操作日志；Go 先校验权限再解析参数，返回 403 并记一条失败操作日志。无权限调用方不再能探测参数校验信息。
- 分页参数非法（如 `pageNum=x`）：Java 返回 Spring 的英文转换错误原文，Go 返回 `参数类型错误: pageNum`；查询参数 Java 会去空白、按 int32 解析，Go 不去空白。
- 请求体 `Content-Type` 不是 JSON 时：Java 因 `HttpMediaTypeNotSupportedException` 落到兜底处理返回 500，Go 仍按 JSON 解析。
- `sql_mode`：Connector/J 会给会话补上 `STRICT_TRANS_TABLES`，Go 沿用服务器默认值；MySQL 5.7+ / 8.0 默认已是严格模式。
- JSON 对象键顺序：Java `LinkedHashMap` 保持插入顺序，Go 按键名排序；键、值、类型完全一致。
- 浮点数文本：Jackson 写 `100.0`，Go 写 `100`；JS 解析后同为 number。
- 500 错误写入操作日志的 `error_msg` 文本为 Go 驱动错误原文，与 MyBatis 异常文本不同。

已按 Java 行为对齐的细节（独立审查后修正）：操作日志 `request_params` 为 `[路径参数..., 绑定后的 DTO]`，DTO 按字段声明顺序输出全部字段（含 null），敏感字段统一 `******`，按 UTF-16 单元截断 4000；`@RequestBody` 复刻 Jackson 的宽松标量转换（小数→整数截断、数字字符串→数字、数字→字符串、空串→null）、字段名大小写敏感、字面量 `null` 体返回 400；JWT 与 jjwt 一样按密钥长度选 HS256 / HS384 / HS512，Java 与 Go 签发的 token 互认；密码超过 72 字节返回 400 `password cannot be more than 72 bytes`；IPv6 来源地址按 `InetAddress.getHostAddress()` 展开（`0:0:0:0:0:0:0:1`）；`@NotBlank` 用 `String.trim()` 语义；客户端中途断开不取消事务；ZIP 条目带当前时间戳；响应附带 Spring Security 默认安全头。

验证记录：`scripts/compare-backends.py` 在同一库上对照 Java 与 Go，管理员 68 个只读接口变体、两个低权限账号各 56 个、匿名 4 个，除上述第一条外零差异；`scaffold-vue/scripts/session-regression.py` 浏览器回归在 Go 后端上通过。
