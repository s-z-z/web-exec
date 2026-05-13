# web-exec 使用指南

## 项目概述

web-exec 是一个基于 Go 的动态 HTTP 路由配置服务。它允许你通过 Web UI 配置 URL 路径与脚本的对应关系，当路由被触发时在指定工作目录中执行脚本并返回执行结果。

支持 Bearer Token 认证、执行历史记录、自签 TLS、限时自动退出等特性。零外部依赖，纯 Go 标准库实现。

## 架构

```
web-exec/
├── cmd/web-exec/main.go          # 入口：HTTP 服务启动、路由注册、认证中间件、TLS、信号处理
├── internal/
│   ├── config/
│   │   ├── model.go              # 数据模型：RouteConfig（ID, Path, WorkDir, Script, CreatedAt, UpdatedAt）
│   │   └── store.go              # 存储层：JSON 文件持久化 + CRUD，sync.RWMutex 线程安全，pathToID 双索引
│   ├── exec/
│   │   ├── exec.go               # 执行服务：脚本写入临时文件、执行、捕获输出、超时控制、stdin 管道
│   │   └── exec_test.go          # 跨平台兼容测试（基本执行、超时、stdin、WorkDir、临时文件清理）
│   ├── configui/
│   │   ├── handler.go            # 配置 UI + REST API + 历史 API，embed 内嵌 HTML
│   │   └── static/
│   │       └── index.html        # 单页前端（深色主题、vanilla JS、代码编辑器、历史面板、Token 认证）
│   └── history/
│   │   ├── model.go              # 执行记录模型：ExecRecord（ID, RouteID, RoutePath, Trigger, Stdout, Stderr, ExitCode, DurationMs, Error）
│   │   └── store.go              # 执行历史存储：JSON 持久化，最多保留 100 条，支持按 RouteID 过滤
├── go.mod                         # Go 1.21.8，零外部依赖
├── Makefile
└── readme.md
```

### 核心模块

| 模块 | 职责 |
|------|------|
| `config` | 路由配置的数据模型与持久化存储。RouteConfig 包含 Path（路由路径）、WorkDir（工作目录）、Script（脚本内容）。双索引（ID → RouteConfig + Path → ID）确保快速查找。数据以 JSON 格式存储在 `routes.json`。 |
| `exec` | 脚本执行引擎。查找路由 → 写临时脚本文件到 WorkDir/.web-exec → 在 WorkDir 中执行 → 捕获 stdout/stderr/exitCode/耗时 → 清理临时文件。支持 context 超时取消和 stdin 管道。跨平台：Windows 用 `cmd /C` + `.bat`，Unix 用 `sh` + `.sh`。 |
| `history` | 执行历史记录与持久化。每次脚本执行（Test 或 Remote 触发）自动记录到 `history.json`，最多保留 100 条（新的在前）。支持按 RouteID 过滤查询。 |
| `configui` | 配置管理界面 + REST API + 历史 API。Go embed 内嵌 HTML 单页应用，提供路由 CRUD、脚本测试、历史查看。 |

### 数据模型

**RouteConfig**（路由配置）:
```json
{
  "id": "a1b2c3d4",          // 8位随机hex，crypto/rand 自动生成
  "path": "/deploy",         // HTTP 路由路径，必须以 / 开头
  "workDir": "/path/to/project", // 脚本执行的工作目录
  "script": "echo hello",    // 脚本内容
  "createdAt": "2026-05-13T14:00:00+08:00",
  "updatedAt": "2026-05-13T14:00:00+08:00"
}
```

**ExecRecord**（执行记录）:
```json
{
  "id": "e5f6g7h8",          // 8位随机hex
  "routeId": "a1b2c3d4",     // 关联的路由 ID
  "routePath": "/deploy",    // 路由路径
  "trigger": "exec",         // 触发方式：exec（远程调用）或 test（UI 测试按钮）
  "stdout": "hello\n",
  "stderr": "",
  "exitCode": 0,
  "durationMs": 152,
  "error": "",
  "createdAt": "2026-05-13T14:05:00+08:00"
}
```

## 启动

```bash
go run ./cmd/web-exec/
```

### 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-addr` | `0.0.0.0:8080` | HTTP 服务监听地址 |
| `-data` | Windows: `%LOCALAPPDATA%/web-exec`; 其他: `data` | 数据持久化目录 |
| `-timeout` | `30s` | 脚本执行超时时间 |
| `-token` | (空) | Bearer Token 认证（空 = 无认证，开放访问） |
| `-tls` | `false` | 启用 HTTPS，自动生成自签名证书 |
| `-live` | (空) | 运行时限自动退出，格式如 `1d2h30m10s`（d=天, h=时, m=分, s=秒） |

示例：

```bash
# 基本启动
go run ./cmd/web-exec/ -addr :9090 -data /var/lib/web-exec -timeout 60s

# 带 Token 认证
go run ./cmd/web-exec/ -addr :8080 -token my-secret-token

# 启用 HTTPS + Token + 限时 2 小时
go run ./cmd/web-exec/ -addr :8443 -token my-token -tls -live 2h
```

## 认证

### Bearer Token 认证

当 `-token` 非空时启用：
- `/exec/*` 路由：所有请求需携带 `Authorization: Bearer <token>` 头
- `/config` HTML 页面：无需认证（允许浏览器直接访问）
- `/config/routes` 和 `/config/history` API：需 Bearer Token

前端在首次遇到 401 时弹出 Token 输入框，Token 存入 localStorage，后续请求自动携带。支持 Logout 按钮。

```bash
# 带 Token 的 curl 请求
curl -X POST -H "Authorization: Bearer my-token" http://localhost:8080/exec/deploy
curl -H "Authorization: Bearer my-token" http://localhost:8080/config/routes
```

## 接口

### 执行接口

**URL**: `/exec/<route-path>`
**方法**: POST（推荐）或 GET
**请求体**: 可选，将作为脚本的标准输入（stdin）传入
**认证**: Bearer Token（如果启用）

**响应**:

```json
{
  "stdout": "hello\n",
  "stderr": "",
  "exitCode": 0,
  "durationMs": 152,
  "error": ""
}
```

| 字段 | 说明 |
|------|------|
| `stdout` | 脚本标准输出 |
| `stderr` | 脚本标准错误 |
| `exitCode` | 退出码，-1 表示超时或异常 |
| `durationMs` | 执行耗时（毫秒） |
| `error` | 非退出码类错误信息（如超时），omitempty |

**错误码**:
- `400` - 路由路径缺失
- `404` - 路由未找到
- `401` - Token 认证失败
- `500` - 执行异常

示例：

```bash
curl -X POST http://localhost:8080/exec/deploy
curl -X POST -d '{"app":"myapp"}' http://localhost:8080/exec/build
```

### 配置接口

**URL**: `/config` 或 `/`（根路径自动跳转到 `/config`）

浏览器访问即可打开配置管理界面。功能：
- 路由 CRUD（增删改查）
- 每行 **Test** 按钮：实时执行脚本，显示 stdout/stderr/exitCode/耗时
- **History** 按钮：查看执行历史，按路由过滤，展开查看详细输出
- Bearer Token 输入与 Logout

### REST API

| 方法 | 路径 | 说明 | 请求体 | 认证 |
|------|------|------|--------|------|
| GET | `/config/routes` | 获取所有路由列表 | - | Token |
| POST | `/config/routes` | 创建路由 | `{path, workDir, script}` | Token |
| PUT | `/config/routes/{id}` | 更新路由 | `{path, workDir, script}` | Token |
| DELETE | `/config/routes/{id}` | 删除路由 | - | Token |
| GET | `/config/history` | 获取所有执行历史 | - | Token |
| GET | `/config/history?routeId=xxx` | 按路由 ID 过滤历史 | - | Token |

创建示例：

```bash
curl -X POST http://localhost:8080/config/routes \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer my-token" \
  -d '{"path":"/deploy","workDir":"/home/user/project","script":"./deploy.sh"}'
```

## 数据持久化

| 文件 | 说明 | 位置 |
|------|------|------|
| `routes.json` | 路由配置 | Windows: `%LOCALAPPDATA%/web-exec/routes.json`; 其他: `data/routes.json` |
| `history.json` | 执行历史（最多 100 条） | 同上目录 |

启动时自动加载，收到 SIGINT/SIGTERM 或 `-live` 到期时自动保存。历史每次执行后即时写入磁盘。

手动备份：

```bash
cp data/routes.json data/routes.json.bak
cp data/history.json data/history.json.bak
```

## TLS

`-tls` 启用 HTTPS，自动生成内存中的自签名 ECDSA (P256) 证书，有效期 1 年。适用于开发与内网使用。浏览器会提示不信任，需手动确认。

## 限时运行

`-live` 参数设置运行时限，到期后自动保存数据并优雅关闭。适合临时任务场景。格式支持组合：`1d2h30m10s`（1天2小时30分10秒）。

## 跨平台

脚本执行跨平台兼容：
- Windows：临时文件 `.bat`，用 `cmd /C` 执行
- Unix：临时文件 `.sh`，用 `sh` 执行

编写脚本时注意使用目标平台兼容的命令。临时文件存放在 `WorkDir/.web-exec/` 目录，执行后自动清理。

## 需求演进与实现状态

| 版本 | 需求 | 状态 |
|------|------|------|
| **v1** | 基本路由配置 + 脚本执行 + Web UI | ✅ 已完成 |
| **v3** | 配置页面每个路由增加测试按钮 | ✅ 已完成 |
| **v4** | exec 和 config 加 bearer token 验证 | ✅ 已完成 |
| **额外** | 执行历史记录（history 模块 + 前端 History 面板） | ✅ 已完成 |
| **额外** | 自签 TLS 支持（`-tls`） | ✅ 已完成 |
| **额外** | 限时运行（`-live`） | ✅ 已完成 |
| **额外** | 代码编辑器（行号、Tab 支持） | ✅ 已完成 |

## Makefile

| 命令 | 说明 |
|------|------|
| `make build` | 编译二进制 |
| `make run` | 编译并运行（带 Token） |
| `make dev` | go run 直接运行（带 Token） |
| `make test` | 运行测试 |
| `make vet` | go vet 检查 |
| `make fmt` | 格式化代码 |
| `make lint` | vet + fmt |
| `make clean` | 清理二进制和 data 目录 |