[English](README.md) | [中文](README.zh-CN.md) | [한국어](README.ko.md)

# web-exec

一个轻量级的 HTTP 到脚本的网关服务，将远端服务器的能力封装为 Web 接口——而无需开放完整的 SSH 权限。

在使用 AI Agent 时，你经常需要它在远端服务器上触发操作（部署、重启、状态检查等）。直接交给 SSH 凭据意味着授予了远超所需的权限。**web-exec** 让你将特定脚本封装在受认证保护的 HTTP 路由之后，Agent 只能调用你明确配置的操作——别无其他。

## 为什么需要 web-exec？

**问题：** 给 AI Agent SSH 权限意味着它可以做*任何事情*——读取文件、修改配置、安装软件包，甚至更危险的操作。

**解决方案：** 使用 web-exec，你只定义哪些操作可以作为 HTTP 接口对外暴露。Agent 调用 `POST /exec/deploy` 即可获取结果。它永远看不到 Shell，永远不直接接触文件系统，也永远不会获得超出你预期的权限。

这是**最小权限原则**在 AI Agent 集成中的实践：只暴露必要的，保护其余的一切。

## 特性

- **动态路由** — 通过 Web UI 配置 URL 路径与脚本的对应关系，无需重启
- **Bearer Token 认证** — 用 Token 保护执行和配置接口；配置页面无需认证即可访问，方便管理员在浏览器中输入 Token
- **自签 TLS** — 一条命令启用 HTTPS，自动生成 ECDSA 证书，适合内网使用
- **执行历史** — 每次脚本执行都自动记录（stdout、stderr、退出码、耗时），可在 UI 或 API 中查看
- **脚本测试** — 在配置页面直接测试路由脚本，验证后再上线
- **跨平台** — Windows（`cmd /C` + `.bat`）和 Unix（`sh` + `.sh`）原生支持
- **限时运行** — 配置运行时限后自动优雅退出（`-live 2h`），适合临时任务
- **零依赖** — 纯 Go 标准库实现，无需安装任何外部依赖
- **代码编辑器** — 内置行号和 Tab 支持的脚本编辑器

## 安装

### 从源码构建

```bash
git clone https://github.com/s-z-z/web-exec.git
cd web-exec
go build ./cmd/web-exec/
```

生成的二进制文件为 `web-exec`（Windows 上为 `web-exec.exe`）。

### Go install

```bash
go install github.com/s-z-z/web-exec/cmd/web-exec@latest
```

## 快速开始

```bash
# 默认启动（端口 8080，无认证）
web-exec

# 启用 TLS 和 Token
web-exec -addr :8080 -tls -token my-secret-token
```

然后在浏览器中打开 `http://localhost:8080/config`（启用 TLS 时为 `https://...`）。

## 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-addr` | `0.0.0.0:8080` | HTTP 服务监听地址。使用 `:8080` 监听所有接口，`127.0.0.1:8080` 仅限本机。 |
| `-data` | Linux/macOS: `data`；Windows: `%LOCALAPPDATA%\web-exec` | 数据持久化目录（`routes.json`、`history.json`），自动创建。 |
| `-timeout` | `30s` | 每个脚本的最大执行时间。超时的脚本会被终止。接受 Go 时长格式（`10s`、`1m`、`2m30s`）。 |
| `-token` | *(空)* | Bearer Token 认证。设置后，`/exec/*` 路由和配置 API 需要 `Authorization: Bearer <token>`。为空则开放访问。 |
| `-tls` | `false` | 启用 HTTPS，自动生成自签名 ECDSA (P256) 证书（有效期 1 年）。适用于开发和内网。 |
| `-live` | *(空)* | 自动退出时限。到期后服务保存数据并优雅关闭。格式：`1d2h30m10s`（天、时、分、秒可组合）。 |

### 示例

```bash
# 基本模式 — 端口 9090，开放访问
web-exec -addr :9090

# 认证模式 — 需要 Token
web-exec -addr :8080 -token my-secret-token

# HTTPS + Token — 安全内网服务
web-exec -addr :8443 -token my-token -tls

# 临时模式 — 2 小时后自动关闭
web-exec -addr :8080 -token my-token -live 2h

# 全安全模式 — HTTPS、Token、超时、限时运行
web-exec -addr :8443 -token my-token -tls -timeout 60s -live 1d8h
```

## 认证

设置 `-token` 后，启用 Bearer Token 认证：

- **`/exec/*`** — 所有执行请求需要 `Authorization: Bearer <token>`
- **`/config`**（HTML 页面） — 无需认证即可访问；浏览器加载 UI 后展示 Token 输入框
- **`/config/routes` 和 `/config/history`**（API） — 需要 `Authorization: Bearer <token>`

前端在首次认证成功后将 Token 存入 `localStorage`，后续请求自动携带。

### curl 示例

```bash
# 带 Token 执行路由
curl -X POST -H "Authorization: Bearer my-secret-token" https://localhost:8080/exec/deploy

# 请求体作为脚本 stdin 传入
curl -X POST -H "Authorization: Bearer my-secret-token" \
  -d '{"app":"myapp"}' https://localhost:8080/exec/build

# 查看路由列表
curl -H "Authorization: Bearer my-secret-token" https://localhost:8080/config/routes

# 查看执行历史
curl -H "Authorization: Bearer my-secret-token" https://localhost:8080/config/history
```

## API

### 执行接口

**URL:** `/exec/<route-path>`
**方法:** `POST`（推荐）或 `GET`
**请求体:** 可选 — 作为脚本的 stdin 传入
**认证:** Bearer Token（如已配置）

**响应：**

```json
{
  "stdout": "deployment complete\n",
  "stderr": "",
  "exitCode": 0,
  "durationMs": 342,
  "error": ""
}
```

| 字段 | 说明 |
|------|------|
| `stdout` | 脚本的标准输出 |
| `stderr` | 脚本的标准错误 |
| `exitCode` | 退出码；`-1` 表示超时或异常终止 |
| `durationMs` | 执行耗时（毫秒） |
| `error` | 非退出码类错误信息（如超时）；为空时省略 |

**错误码：** `400`（路径缺失）、`404`（路由未找到）、`401`（认证失败）、`500`（执行异常）

### 配置 API

| 方法 | 路径 | 说明 | 请求体 | 认证 |
|------|------|------|--------|------|
| GET | `/config/routes` | 获取所有路由 | — | Token |
| POST | `/config/routes` | 创建路由 | `{path, workDir, script}` | Token |
| PUT | `/config/routes/{id}` | 更新路由 | `{path, workDir, script}` | Token |
| DELETE | `/config/routes/{id}` | 删除路由 | — | Token |
| GET | `/config/history` | 获取所有执行历史 | — | Token |
| GET | `/config/history?routeId=xxx` | 按路由过滤历史 | — | Token |

### 路由配置

每条路由包含：

```json
{
  "id": "a1b2c3d4",
  "path": "/deploy",
  "workDir": "/path/to/project",
  "script": "./deploy.sh",
  "createdAt": "2026-05-13T14:00:00+08:00",
  "updatedAt": "2026-05-13T14:00:00+08:00"
}
```

| 字段 | 说明 |
|------|------|
| `id` | 自动生成的 8 位 hex 标识符 |
| `path` | HTTP 路由路径（必须以 `/` 开头） |
| `workDir` | 脚本执行的工作目录 |
| `script` | 要执行的脚本内容 |
| `createdAt` | 创建时间 |
| `updatedAt` | 最后修改时间 |

### 执行记录

```json
{
  "id": "e5f6g7h8",
  "routeId": "a1b2c3d4",
  "routePath": "/deploy",
  "trigger": "exec",
  "stdout": "done\n",
  "stderr": "",
  "exitCode": 0,
  "durationMs": 152,
  "error": "",
  "createdAt": "2026-05-13T14:05:00+08:00"
}
```

| 字段 | 说明 |
|------|------|
| `trigger` | `exec`（远程调用）或 `test`（UI 测试按钮） |
| `routeId` | 执行的路由 ID |
| 其他字段 | 与执行响应相同 + 时间戳 |

## Web UI

在浏览器中打开 `/config`。配置页面提供：

- **路由 CRUD** — 新增、编辑、删除路由
- **脚本测试** — 点击任意路由的 "Test" 按钮，立即执行脚本查看结果
- **执行历史** — 点击 "History" 查看历史执行记录，按路由过滤，展开查看详细输出
- **Token 管理** — 首次访问时输入 Bearer Token；Logout 按钮可清除
- **代码编辑器** — 支持行号和 Tab 缩进的脚本编辑器

## 数据持久化

| 文件 | 内容 | 位置 |
|------|------|------|
| `routes.json` | 路由配置 | `<data-dir>/routes.json` |
| `history.json` | 执行记录（最多 100 条） | `<data-dir>/history.json` |

默认数据目录：Linux/macOS 为 `data/`，Windows 为 `%LOCALAPPDATA%\web-exec\`。

启动时加载，优雅关闭（SIGINT/SIGTERM）或 `-live` 到期时保存。历史记录每次执行后即时写入磁盘。

## TLS

使用 `-tls` 启用。服务在内存中自动生成自签名 ECDSA (P256) 证书，有效期 1 年。浏览器会显示安全警告——适用于开发/内网环境。不适合公网生产环境（需搭配正规证书）。

## 限时运行

使用 `-live` 设置最长运行时间。到期后服务优雅关闭，先保存所有数据。适合临时任务或控制暴露窗口。

```bash
# 运行 30 分钟
web-exec -live 30m

# 运行 1 天 2 小时
web-exec -live 1d2h
```

## 跨平台执行

脚本在路由的 `workDir` 中使用平台原生 Shell 执行：

- **Windows:** 脚本写入 `.bat` 临时文件，用 `cmd /C` 执行
- **Unix (Linux/macOS):** 脚本写入 `.sh` 临时文件，用 `sh` 执行

临时文件放在 `workDir/.web-exec/` 目录下，执行后自动清理。编写脚本时请使用与目标平台兼容的命令。

## 架构

```
web-exec/
├── cmd/web-exec/main.go           # 入口：HTTP 服务、路由、认证、TLS、信号处理
├── internal/
│   ├── config/
│   │   ├── model.go               # RouteConfig 数据模型
│   │   └── store.go               # JSON 持久化，线程安全 CRUD，双索引（ID + path）
│   ├── exec/
│   │   ├── exec.go                # 脚本执行引擎：临时文件、超时、stdin、清理
│   │   └── exec_test.go           # 跨平台测试
│   ├── configui/
│   │   ├── handler.go             # Web UI + REST API + 历史 API（embed 内嵌 HTML）
│   │   └── static/index.html      # 单页前端（深色主题，vanilla JS）
│   └── history/
│   │   ├── model.go               # ExecRecord 数据模型
│   │   └── store.go               # 执行历史持久化（最多 100 条）
├── go.mod                          # Go 1.21+，零外部依赖
```

## 许可证

MIT