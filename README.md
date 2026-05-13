[English](README.md) | [中文](README.zh-CN.md) | [한국어](README.ko.md)

# web-exec

A lightweight HTTP-to-script gateway that exposes remote server capabilities as web endpoints — without giving away full SSH access.

When working with AI Agents, you often need them to trigger actions on remote servers (deployments, restarts, status checks). Handing over SSH credentials grants far more access than necessary. **web-exec** lets you wrap specific scripts behind authenticated HTTP routes, so the Agent can only call what you've explicitly configured — nothing more.

## Why web-exec?

**Problem:** Giving an AI Agent SSH access to a server means it can do *anything* — read files, modify configs, install packages, or worse.

**Solution:** With web-exec, you define exactly which operations are available as HTTP endpoints. The Agent calls `POST /exec/deploy` and gets the result. It never sees the shell, never touches the filesystem directly, and never gets more privilege than you intended.

This is the principle of **least privilege** applied to AI Agent integrations: expose only what's needed, protect everything else.

## Features

- **Dynamic routing** — Configure URL paths and their associated scripts through a web UI, no restarts needed
- **Bearer Token authentication** — Protect execution and configuration endpoints with a token; the config page is accessible without auth so admins can enter the token in-browser
- **Self-signed TLS** — One-flag HTTPS with auto-generated ECDSA certificate, ready for internal networks
- **Execution history** — Every script run is recorded (stdout, stderr, exit code, duration), viewable in the UI or via API
- **Script testing** — Test any route's script directly from the config page before going live
- **Cross-platform** — Windows (`cmd /C` + `.bat`) and Unix (`sh` + `.sh`) supported out of the box
- **Time-limited operation** — Auto-exit after a configured duration (`-live 2h`), perfect for temporary tasks
- **Zero dependencies** — Pure Go standard library, nothing else to install
- **Code editor** — In-browser editor with line numbers and tab support for writing scripts

## Installation

### From source

```bash
git clone https://github.com/s-z-z/web-exec.git
cd web-exec
go build ./cmd/web-exec/
```

The binary will be `web-exec` (or `web-exec.exe` on Windows).

### Go install

```bash
go install github.com/s-z-z/web-exec/cmd/web-exec@latest
```

## Quick Start

```bash
# Start with defaults (port 8080, no auth)
web-exec

# Start with TLS and token
web-exec -addr :8080 -tls -token my-secret-token
```

Then open `http://localhost:8080/config` (or `https://...` with TLS) in your browser.

## Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `0.0.0.0:8080` | Listen address for the HTTP server. Use `:8080` for all interfaces, `127.0.0.1:8080` for local only. |
| `-data` | `data` (Linux/macOS); `%LOCALAPPDATA%\web-exec` (Windows) | Directory for persistent storage (`routes.json`, `history.json`). Created automatically. |
| `-timeout` | `30s` | Maximum execution time per script. Scripts exceeding this are killed. Accepts Go duration format (`10s`, `1m`, `2m30s`). |
| `-token` | *(empty)* | Bearer token for authentication. When set, `/exec/*` routes and config API endpoints require `Authorization: Bearer <token>`. Empty means open access. |
| `-tls` | `false` | Enable HTTPS with an auto-generated self-signed ECDSA (P256) certificate (1-year validity). Suitable for development and internal networks. |
| `-live` | *(empty)* | Auto-exit duration. The server saves data and shuts down gracefully after this period. Format: `1d2h30m10s` (days, hours, minutes, seconds — combinable). |

### Examples

```bash
# Basic — open access on port 9090
web-exec -addr :9090

# Authenticated — token required
web-exec -addr :8080 -token my-secret-token

# HTTPS + token — secure internal service
web-exec -addr :8443 -token my-token -tls

# Temporary — auto-shutdown after 2 hours
web-exec -addr :8080 -token my-token -live 2h

# Full security — HTTPS, token, timeout, limited runtime
web-exec -addr :8443 -token my-token -tls -timeout 60s -live 1d8h
```

## Authentication

When `-token` is set, Bearer Token authentication is enforced:

- **`/exec/*`** — All execution requests require `Authorization: Bearer <token>`
- **`/config`** (HTML page) — Accessible without auth; the browser loads the UI and shows a token input field
- **`/config/routes` and `/config/history`** (API) — Require `Authorization: Bearer <token>`

The frontend stores the token in `localStorage` after the first successful auth and includes it on all subsequent API calls.

### curl examples

```bash
# Execute a route with token
curl -X POST -H "Authorization: Bearer my-secret-token" https://localhost:8080/exec/deploy

# Pass request body as stdin to the script
curl -X POST -H "Authorization: Bearer my-secret-token" \
  -d '{"app":"myapp"}' https://localhost:8080/exec/build

# List routes
curl -H "Authorization: Bearer my-secret-token" https://localhost:8080/config/routes

# View execution history
curl -H "Authorization: Bearer my-secret-token" https://localhost:8080/config/history
```

## API

### Execution Endpoint

**URL:** `/exec/<route-path>`
**Methods:** `POST` (recommended) or `GET`
**Request body:** Optional — passed as stdin to the script
**Auth:** Bearer Token (if configured)

**Response:**

```json
{
  "stdout": "deployment complete\n",
  "stderr": "",
  "exitCode": 0,
  "durationMs": 342,
  "error": ""
}
```

| Field | Description |
|-------|-------------|
| `stdout` | Standard output from the script |
| `stderr` | Standard error from the script |
| `exitCode` | Exit code; `-1` indicates timeout or abnormal termination |
| `durationMs` | Execution time in milliseconds |
| `error` | Non-exit-code error message (e.g. timeout); omitted when empty |

**Error codes:** `400` (missing path), `404` (route not found), `401` (auth failed), `500` (execution error)

### Configuration API

| Method | Path | Description | Body | Auth |
|--------|------|-------------|------|------|
| GET | `/config/routes` | List all routes | — | Token |
| POST | `/config/routes` | Create a route | `{path, workDir, script}` | Token |
| PUT | `/config/routes/{id}` | Update a route | `{path, workDir, script}` | Token |
| DELETE | `/config/routes/{id}` | Delete a route | — | Token |
| GET | `/config/history` | List all execution records | — | Token |
| GET | `/config/history?routeId=xxx` | Filter history by route | — | Token |

### Route Configuration

Each route has:

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

| Field | Description |
|-------|-------------|
| `id` | Auto-generated 8-char hex identifier |
| `path` | HTTP route path (must start with `/`) |
| `workDir` | Working directory where the script executes |
| `script` | Shell script content to run |
| `createdAt` | Creation timestamp |
| `updatedAt` | Last modification timestamp |

### Execution Record

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

| Field | Description |
|-------|-------------|
| `trigger` | `exec` (remote call) or `test` (UI test button) |
| `routeId` | ID of the executed route |
| Other fields | Same as execution response + timestamps |

## Web UI

Open `/config` in a browser. The configuration page provides:

- **Route CRUD** — Add, edit, and delete routes
- **Script testing** — Click "Test" on any route to execute its script and see results immediately
- **Execution history** — Click "History" to view past runs, filter by route, and expand details
- **Token management** — Enter Bearer Token on first visit; logout button to clear it
- **Code editor** — Syntax-highlighted editor with line numbers and tab indentation

## Data Persistence

| File | Content | Location |
|------|---------|----------|
| `routes.json` | Route configurations | `<data-dir>/routes.json` |
| `history.json` | Execution records (max 100) | `<data-dir>/history.json` |

Default data directory: `data/` on Linux/macOS, `%LOCALAPPDATA%\web-exec\` on Windows.

Data is loaded on startup and saved on graceful shutdown (SIGINT/SIGTERM) or `-live` expiry. History is written to disk after every execution.

## TLS

Enable with `-tls`. The server generates an in-memory self-signed ECDSA (P256) certificate valid for 1 year. Browsers will show a security warning — accept it for development/internal use. Not suitable for public production without a proper certificate.

## Time-Limited Operation

Use `-live` to set a maximum runtime. The server shuts down gracefully after the duration expires, saving all data first. Useful for temporary tasks or controlled exposure windows.

```bash
# Run for exactly 30 minutes
web-exec -live 30m

# Run for 1 day, 2 hours
web-exec -live 1d2h
```

## Cross-Platform Execution

Scripts are executed in the route's `workDir` using the platform's native shell:

- **Windows:** Script written to `.bat` temp file, executed with `cmd /C`
- **Unix (Linux/macOS):** Script written to `.sh` temp file, executed with `sh`

Temp files are placed in `workDir/.web-exec/` and cleaned up after execution. Write scripts using commands compatible with the target platform.

## Architecture

```
web-exec/
├── cmd/web-exec/main.go           # Entry point: server, routing, auth, TLS, signal handling
├── internal/
│   ├── config/
│   │   ├── model.go               # RouteConfig data model
│   │   └── store.go               # JSON persistence, thread-safe CRUD, dual index (ID + path)
│   ├── exec/
│   │   ├── exec.go                # Script execution engine: temp file, timeout, stdin, cleanup
│   │   └── exec_test.go           # Cross-platform tests
│   ├── configui/
│   │   ├── handler.go             # Web UI + REST API + history API (embedded HTML)
│   │   └── static/index.html      # Single-page frontend (dark theme, vanilla JS)
│   └── history/
│       ├── model.go               # ExecRecord data model
│       └── store.go               # Execution history persistence (max 100 records)
├── go.mod                          # Go 1.21+, zero external dependencies
```

## License

MIT