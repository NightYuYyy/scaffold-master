# 用户中心平台

一个可以被多个业务项目共用的账号与用户数据平台：业务项目在控制台注册为「应用」，用平台的开放接口完成用户注册登录，并按应用配置扩展字段（等级、邀请码……）和数值账户（积分、币……）。平台自带运营控制台，管理应用、应用用户、字段、流水和日志。

```text
业务项目前端 ──X-App-Key──▶ /open/v1  注册 / 登录 / 我的信息 / 改资料
业务项目后端 ──X-App-Key + X-App-Secret──▶ /open/v1  用户管理 / 账户加减 / 令牌内省
运营人员 ──浏览器──▶ 控制台（scaffold-vue）──▶ /api  应用 / 用户 / 字段 / 流水 / 日志 / 后台账号与权限
```

用户 token 用应用自己的密钥签发（HS256），业务后端可本地验签，不必每次回调平台。

## 开发入口

- [接口契约](docs/platform-api.md)：开放接口与管理接口的字段、错误提示与规则，接入方只需看这一份。
- [项目地图](PROJECT_MAP.md)：按功能定位页面、接口、SQL 和权限码。
- [后端架构](docs/backend-architecture.md)、[前后端接口约定](docs/frontend-backend-integration.md)、[前端架构](docs/admin-redesign-architecture.md)、[设计规范](DESIGN.md)。
- [开发约定](AGENTS.md)、[更新日志](changelog.md)。

## 目录与技术栈

| 目录 | 用途 | 技术栈 |
| --- | --- | --- |
| `scaffold-go` | 后端：开放接口、管理接口、控制台认证与权限 | Go 1.26+ 标准库 `net/http` + `database/sql`、MySQL 8、Redis、JWT、bcrypt；仅 4 个直接使用的第三方库 |
| `scaffold-vue` | 运营控制台 | Vue 3、TypeScript、Vite 6、Vue Router 4、GSAP 3，自有组件库 |
| `sql` | 初始化脚本 | `V1__init_schema.sql` 建表（21 张），`V2__seed_data.sql` 初始数据与示例应用 |
| `deploy` | 一键部署 | `docker-compose.yml` + `.env.example` |
| `.github/workflows` | 自动构建镜像 | 推 `main` 或 `v*` 标签时构建前后端镜像推到 ghcr.io |

## 本地启动

准备 Go 1.26+（与 `go.mod` / Dockerfile 一致）、Node.js 22+、MySQL 8 和 Redis。后端只读进程环境变量：

| 变量 | 说明 | 默认值 |
| --- | --- | --- |
| `JWT_SECRET` | 控制台登录的 JWT 密钥，至少 32 字节 | 必填 |
| `DB_HOST` / `DB_PORT` / `DB_NAME` / `DB_USERNAME` / `DB_PASSWORD` / `DB_TIMEZONE` | MySQL 连接 | `localhost` / `3306` / `scaffold_system` / `root` / `root` / `+08:00` |
| `REDIS_HOST` / `REDIS_PORT` / `REDIS_PASSWORD` | Redis 连接 | `localhost` / `6379` / 空 |
| `SERVER_PORT` | 监听端口 | `8080` |
| `DB_AUTO_INIT` | 启动前执行 `sql/` 下的初始化脚本（幂等） | 空 |
| `SQL_DIR` | 初始化脚本目录 | `../sql` |

```bash
# 初始化数据库（建库 + 建表 + 初始数据；可重复执行）
cd scaffold-go && JWT_SECRET="$(openssl rand -hex 32)" go run ./cmd/server init-db
```

```bash
# 启动后端，监听 8080
cd scaffold-go && JWT_SECRET="$(openssl rand -hex 32)" go run ./cmd/server
```

```bash
# 启动控制台，监听 5173；/api 与 /open 代理到后端
cd scaffold-vue && npm ci && npm run dev
```

控制台种子账号：`admin / admin123`（超级管理员）、`appadmin / admin123`（应用管理员，只能看示例应用）。种子里有一个示例应用 `demo`，密钥 `demo_pk_local` / `demo-secret-change-me-0123456789abcdef0123456789`，示例用户 `demo_user / admin123`。**接入真实环境前请删除示例应用、更换密钥与密码。**

试一下开放接口（验证码与控制台一致，由客户端展示）：

```bash
curl -s -H 'X-App-Key: demo_pk_local' http://localhost:8080/open/v1/captcha
```

## 接入一个新项目

1. 控制台「应用管理」新建应用，进入「密钥」新增一把密钥，记下 `api_key` 与只显示一次的 `api_secret`。
2. 「字段配置」给应用加扩展字段；需要增减的数值（积分、币）选「数值账户」类型。
3. 业务项目前端带 `X-App-Key` 调 `/open/v1/auth/register`、`/auth/login`、`/me`；业务后端带 `X-App-Key` + `X-App-Secret` 调用户管理与账户加减接口，或用 `api_secret` 本地验签用户 token。
4. 需要限制某个运营人员只能管这个应用：「后台账号」里给账号绑定应用并分配「应用管理员」角色。

细节与字段见 [docs/platform-api.md](docs/platform-api.md)。

## 容器部署

服务器上只需要 `deploy/docker-compose.yml` 和由 `deploy/.env.example` 复制出的 `.env`（填 `JWT_SECRET`、`DB_PASSWORD`）：

```bash
docker compose up -d
```

编排包含 MySQL、Redis、后端与前端（nginx 托管页面并把 `/api`、`/open` 转发到后端）。后端首次启动自动建库建表并写入初始数据。访问 `http://<服务器>:<WEB_PORT>`，默认 80。更新版本改 `.env` 的 `IMAGE_TAG` 后 `docker compose pull && docker compose up -d`。

镜像由 `.github/workflows/docker.yml` 自动构建：`ghcr.io/<github 用户名小写>/scaffold-backend` 与 `scaffold-frontend`。本地构建：`docker build -f scaffold-go/Dockerfile -t scaffold-backend .`（上下文为仓库根目录）、`docker build -t scaffold-frontend scaffold-vue`；国内可给后端加 `--build-arg GOPROXY=https://goproxy.cn,direct`。

## 验证

```bash
# 后端：集成测试需要本地 MySQL（库 scaffold_system_test 已用 sql/ 初始化）与 Redis
cd scaffold-go && go test ./... -count=1
```

```bash
# 前端
cd scaffold-vue && npm run test:unit && npm run build
# 已安装 Python Playwright 与 Chromium、前后端均启动时
python scripts/session-regression.py
```

## 许可证

本项目代码采用 [MIT License](LICENSE)。第三方依赖遵循各自许可证；GSAP 使用 [Standard "No Charge" License](https://gsap.com/community/standard-license/)，不属于本项目的 MIT 授权范围。
