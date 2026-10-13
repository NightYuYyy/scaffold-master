# 通用后台管理前端

Vue 3 + TypeScript + Vite + Vue Router + GSAP 的后台开发底座，使用珍珠玻璃主题和自有组件库；已接入 Go 用户中心平台后端。

## 开发

```bash
npm ci
npm run dev
npm run test:unit
npm run build
```

默认地址为 `http://localhost:5173`，`/api` 代理到 `http://localhost:8080`，可通过 `VITE_API_PROXY_TARGET` 指定其他本地后端。后端与数据库准备见 [根目录 README](../README.md)。

种子管理员 `admin` 的开发密码为 `admin123`，验证码必须使用当前页面显示的字符。非管理员菜单来自角色授权；无菜单账号显示明确的权限分配提示。

## 目录

| 路径 | 中文职责 |
| --- | --- |
| `src/api` | 统一 HTTP、会话存储、认证、系统管理、用户中心平台与生成器接口 |
| `src/router/index.ts` | 静态入口、动态菜单路由、登录守卫与会话失效跳转 |
| `src/composables/auth.ts` | 当前用户、权限、动态菜单与会话版本 |
| `src/components/ui` | 按钮、输入、下拉、表格、弹窗、抽屉、提示等基础控件 |
| `src/components/business` | ListPage、SearchForm、FormDialog、AppContext、PermissionButton 及字典选择与标签 |
| `src/components/layout` | 外壳、侧栏、顶栏、命令搜索、外观与密码弹窗 |
| `src/composables` | 列表、字典缓存、应用选择、外观、动效、焦点与确认框 |
| `src/styles` | 令牌、基础、材质、控件、外壳、页面六层样式 |
| `src/views` | 登录、工作台、5 个应用平台页、5 个系统管理页、2 个监控页、无权限与未匹配页面 |
| `scripts/session-regression.py` | 真实浏览器会话、路由竞态与低权限回归 |
| `scripts/platform-context-regression.py` | 当前应用跨页选择、表单归属、重置 / 导出范围与切换竞态回归 |

## 扩展约定

新列表先复用 `ListPage` 与列表 composable，新表单先复用 `FormDialog`，权限动作使用 `PermissionButton`；接口复用 `api/http.ts`。在 `auth.ts` 的 `COMPONENT_MAP` 注册页面，再通过菜单管理配置路径、组件标识与权限码。

应用级页面先用 `AppContext` 选择「当前应用」，一次只查看一个应用的数据。应用用户、字段、流水和认证日志共用选择，重置筛选不切换应用；新增自动归属当前应用，弹窗以只读项明确显示所属应用。`useAppSelect` 统一管理选择与可见应用缓存，使用 `toParams` 注入应用范围，不要把 `appId` 当作可重置的普通筛选字段。

退出和 401 均通过统一会话清理撤销动态路由与缓存；不得在页面直接修改认证存储或路由就绪标记。旧请求只能更新发起时仍有效的会话。

主题、密度和减少动效规则见 [DESIGN.md](../DESIGN.md)，前后端链路见 [PROJECT_MAP.md](../PROJECT_MAP.md)。

## 浏览器回归

已有 Python Playwright 与 Chromium、前后端均启动时，可运行：

```bash
python3 scripts/session-regression.py
python3 scripts/platform-context-regression.py --base-url http://localhost:5173
```

建议连接独立本地校验库；脚本创建并删除自身的角色 / 用户夹具，不清空任何表。管理员密码可用 `SCAFFOLD_TEST_PASSWORD` 覆盖；脚本需要 Vite 开发服务器来访问应用实际加载的 HTTP 模块。

应用上下文回归还会创建 A / B 应用、字段及用户并通过正式接口清理；可以对 Vite 或生产构建运行，传 `--screenshots <目录>` 可保存桌面 / 手机暗色截图。

历史原型和工作流页面已移除；历史改动见根目录更新日志。

本项目代码采用 [MIT License](../LICENSE)；GSAP 依赖遵循其 [Standard "No Charge" License](https://gsap.com/community/standard-license/)。
