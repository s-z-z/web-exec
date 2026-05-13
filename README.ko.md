[English](README.md) | [中文](README.zh-CN.md) | [한국어](README.ko.md)

# web-exec

원격 서버의 기능을 HTTP 엔드포인트로 노출하는 경량 HTTP-스크립트 게이트웨이 — 전체 SSH 접근 권한을 넘겨주지 않고도.

AI Agent와 작업할 때, 원격 서버에서 액션을 트리거해야 하는 경우가 많습니다(배포, 재시작, 상태 확인 등). SSH 인증 정보를 넘겨주면 필요한 것보다 훨씬 더 많은 권한을 부여하게 됩니다. **web-exec**는 특정 스크립트를 인증된 HTTP 라우트 뒤에 래핑하여, Agent가 명시적으로 설정한 작업만 호출할 수 있도록 합니다 — 그 외에는 아무것도 할 수 없습니다.

## web-exec가 필요한 이유?

**문제:** AI Agent에 SSH 접근 권한을 부여하면 *모든 것*을 할 수 있습니다 — 파일 읽기, 설정 수정, 패키지 설치, 또는 더 위험한 작업.

**솔루션:** web-exec를 사용하면, 어떤 작업을 HTTP 엔드포인트로 노출할지 정확히 정의합니다. Agent는 `POST /exec/deploy`를 호출하고 결과를 받습니다. Shell을 볼 수 없고, 파일 시스템에 직접 접근할 수 없으며, 의도한 것보다 더 많은 권한을 얻을 수 없습니다.

이것은 AI Agent 통합에 **최소 권한 원칙**을 적용한 것입니다: 필요한 것만 노출하고, 나머지는 모두 보호합니다.

## 기능

- **동적 라우팅** — Web UI를 통해 URL 경로와 스크립트 매핑을 설정, 재시작 없이 변경 가능
- **Bearer Token 인증** — Token으로 실행 및 설정 엔드포인트를 보호; 설정 페이지는 인증 없이 접근 가능하여 관리자가 브라우저에서 Token을 입력할 수 있습니다
- **자체 서명 TLS** — 한 번의 플래그로 HTTPS 활성화, ECDSA 인증서 자동 생성, 내부 네트워크용
- **실행 기록** — 모든 스크립트 실행이 자동 기록(stdout, stderr, 종료 코드, 소요 시간), UI나 API로 확인
- **스크립트 테스트** — 설정 페이지에서 라우트 스크립트를 직접 테스트, 라이브 전에 검증 가능
- **크로스 플랫폼** — Windows(`cmd /C` + `.bat`) 및 Unix(`sh` + `.sh`) 기본 지원
- **시간 제한 실행** — 설정된 기간 후 자동으로 graceful 종료(`-live 2h`), 임시 작업에 적합
- **제로 의존성** — 순수 Go 표준 라이브러리, 외부 설치 필요 없음
- **코드 편집기** — 행 번호와 Tab 지원을 내장한 스크립트 편집기

## 설치

### 소스에서 빌드

```bash
git clone https://github.com/s-z-z/web-exec.git
cd web-exec
go build ./cmd/web-exec/
```

바이너리는 `web-exec`(Windows에서 `web-exec.exe`)입니다.

### Go install

```bash
go install github.com/s-z-z/web-exec/cmd/web-exec@latest
```

## 빠른 시작

```bash
# 기본 시작 (포트 8080, 인증 없음)
web-exec

# TLS와 Token으로 시작
web-exec -addr :8080 -tls -token my-secret-token
```

브라우저에서 `http://localhost:8080/config`(TLS 활성화시 `https://...`)를 열세요.

## 명령줄 플래그

| 플래그 | 기본값 | 설명 |
|--------|--------|------|
| `-addr` | `0.0.0.0:8080` | HTTP 서버 수신 주소. 모든 인터페이스는 `:8080`, 로컬만은 `127.0.0.1:8080`. |
| `-data` | Linux/macOS: `data`; Windows: `%LOCALAPPDATA%\web-exec` | 데이터 영속화 디렉토리(`routes.json`, `history.json`), 자동 생성. |
| `-timeout` | `30s` | 스크립트 최대 실행 시간. 초과시 종료. Go duration 형식(`10s`, `1m`, `2m30s`). |
| `-token` | *(비어 있음)* | Bearer Token 인증. 설정시 `/exec/*` 라우트와 설정 API에 `Authorization: Bearer <token>` 필요. 비어 있으면 개방 접근. |
| `-tls` | `false` | HTTPS 활성화, 자체 서명 ECDSA (P256) 인증서 자동 생성(유효기간 1년). 개발 및 내부 네트워크용. |
| `-live` | *(비어 있음)* | 자동 종료 기간. 기간 후 서버가 데이터를 저장하고 graceful 종료. 형식: `1d2h30m10s`(일, 시, 분, 초 — 조합 가능). |

### 예시

```bash
# 기본 — 포트 9090, 개방 접근
web-exec -addr :9090

# 인증 — Token 필요
web-exec -addr :8080 -token my-secret-token

# HTTPS + Token — 안전한 내부 서비스
web-exec -addr :8443 -token my-token -tls

# 임시 — 2시간 후 자동 종료
web-exec -addr :8080 -token my-token -live 2h

# 전체 보안 — HTTPS, Token, timeout, 시간 제한 실행
web-exec -addr :8443 -token my-token -tls -timeout 60s -live 1d8h
```

## 인증

`-token` 설정시 Bearer Token 인증이 활성화됩니다:

- **`/exec/*`** — 모든 실행 요청에 `Authorization: Bearer <token>` 필요
- **`/config`**(HTML 페이지) — 인증 없이 접근 가능; 브라우저가 UI를 로드하고 Token 입력 필드를 보여줍니다
- **`/config/routes` 및 `/config/history`**(API) — `Authorization: Bearer <token>` 필요

프론트엔드는 첫 인증 성공 후 Token을 `localStorage`에 저장하고, 이후 모든 API 호출에 자동으로 포함합니다.

### curl 예시

```bash
# Token으로 라우트 실행
curl -X POST -H "Authorization: Bearer my-secret-token" https://localhost:8080/exec/deploy

# 요청 본문을 스크립트 stdin으로 전달
curl -X POST -H "Authorization: Bearer my-secret-token" \
  -d '{"app":"myapp"}' https://localhost:8080/exec/build

# 라우트 목록 조회
curl -H "Authorization: Bearer my-secret-token" https://localhost:8080/config/routes

# 실행 기록 조회
curl -H "Authorization: Bearer my-secret-token" https://localhost:8080/config/history
```

## API

### 실행 엔드포인트

**URL:** `/exec/<route-path>`
**메서드:** `POST`(추천) 또는 `GET`
**요청 본문:** 옵션 — 스크립트의 stdin으로 전달
**인증:** Bearer Token(설정된 경우)

**응답:**

```json
{
  "stdout": "deployment complete\n",
  "stderr": "",
  "exitCode": 0,
  "durationMs": 342,
  "error": ""
}
```

| 필드 | 설명 |
|------|------|
| `stdout` | 스크립트의 표준 출력 |
| `stderr` | 스크립트의 표준 오류 |
| `exitCode` | 종료 코드; `-1`은 timeout 또는 비정상 종료 |
| `durationMs` | 실행 소요 시간(밀리초) |
| `error` | 비종료 코드 오류 메시지(예: timeout); 비어 있으면 생략 |

**오류 코드:** `400`(경로 누락), `404`(라우트 없음), `401`(인증 실패), `500`(실행 오류)

### 설정 API

| 메서드 | 경로 | 설명 | 본문 | 인증 |
|--------|------|------|------|------|
| GET | `/config/routes` | 모든 라우트 목록 | — | Token |
| POST | `/config/routes` | 라우트 생성 | `{path, workDir, script}` | Token |
| PUT | `/config/routes/{id}` | 라우트 업데이트 | `{path, workDir, script}` | Token |
| DELETE | `/config/routes/{id}` | 라우트 삭제 | — | Token |
| GET | `/config/history` | 모든 실행 기록 | — | Token |
| GET | `/config/history?routeId=xxx` | 라우트별 기록 필터 | — | Token |

### 라우트 설정

각 라우트에는:

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

| 필드 | 설명 |
|------|------|
| `id` | 자동 생성된 8자 hex 식별자 |
| `path` | HTTP 라우트 경로(`/`로 시작해야 함) |
| `workDir` | 스크립트가 실행되는 작업 디렉토리 |
| `script` | 실행할 스크립트 내용 |
| `createdAt` | 생성 시간 |
| `updatedAt` | 최근 수정 시간 |

### 실행 기록

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

| 필드 | 설명 |
|------|------|
| `trigger` | `exec`(원격 호출) 또는 `test`(UI 테스트 버튼) |
| `routeId` | 실행된 라우트 ID |
| 다른 필드 | 실행 응답과 동일 + 타임스탬프 |

## Web UI

브라우저에서 `/config`를 열면 설정 페이지에서 제공하는 기능:

- **라우트 CRUD** — 추가, 편집, 삭제
- **스크립트 테스트** — 라우트의 "Test" 버튼을 클릭하여 스크립트를 즉시 실행하고 결과 확인
- **실행 기록** — "History" 버튼을 클릭하여 과거 실행 조회, 라우트별 필터, 디테일 확장
- **Token 관리** — 첫 방문시 Bearer Token 입력; Logout 버튼으로 삭제
- **코드 편집기** — 행 번호와 Tab 들여쓰기를 지원하는 스크립트 편집기

## 데이터 영속화

| 파일 | 내용 | 위치 |
|------|------|------|
| `routes.json` | 라우트 설정 | `<data-dir>/routes.json` |
| `history.json` | 실행 기록(최대 100개) | `<data-dir>/history.json` |

기본 데이터 디렉토리: Linux/macOS는 `data/`, Windows는 `%LOCALAPPDATA%\web-exec\`.

시작시 로드, graceful 종료(SIGINT/SIGTERM) 또 `-live` 만료시 저장. 실행 기록은 각 실행 후 즉시 디스크에 기록.

## TLS

`-tls`로 활성화. 서버가 메모리 내에서 자체 서명 ECDSA (P256) 인증서를 자동 생성(유효기간 1년). 브라우저는 보안 경고를 표시합니다 — 개발/내부 네트워크용으로 확인. 공개 프로덕션에는 적합하지 않습니다(정식 인증서 필요).

## 시간 제한 실행

`-live`로 최대 실행 시간을 설정. 기간 후 서버가 모든 데이터를 저장하고 graceful 종료합니다. 임시 작업 또는 제한된 노출 창에 유용.

```bash
# 정확히 30분 실행
web-exec -live 30m

# 1일 2시간 실행
web-exec -live 1d2h
```

## 크로스 플랫폼 실행

스크립트는 라우트의 `workDir`에서 플랫폼 기본 Shell로 실행:

- **Windows:** 스크립트를 `.bat` 임시 파일로 작성, `cmd /C`로 실행
- **Unix (Linux/macOS):** 스크립트를 `.sh` 임시 파일로 작성, `sh`로 실행

임시 파일은 `workDir/.web-exec/` 디렉토리에 배치되고, 실행 후 자동 정리. 스크립트를 작성할 때 대상 플랫폼과 호환되는 명령을 사용하세요.

## 아키텍처

```
web-exec/
├── cmd/web-exec/main.go           # 진입점: HTTP 서버, 라우팅, 인증, TLS, signal 처리
├── internal/
│   ├── config/
│   │   ├── model.go               # RouteConfig 데이터 모델
│   │   └── store.go               # JSON 영속화, thread-safe CRUD, 이중 인덱스(ID + path)
│   ├── exec/
│   │   ├── exec.go                # 스크립트 실행 엔진: 임시 파일, timeout, stdin, 정리
│   │   └── exec_test.go           # 크로스 플랫폼 테스트
│   ├── configui/
│   │   ├── handler.go             # Web UI + REST API + 기록 API(embed HTML)
│   │   └── static/index.html      # 단일 페이지 프론트엔드(다크 테마, vanilla JS)
│   └── history/
│       ├── model.go               # ExecRecord 데이터 모델
│       └── store.go               # 실행 기록 영속화(최대 100개)
├── go.mod                          # Go 1.21+, 외부 의존성 없음
```

## 라이선스

MIT