<!-- GSD:project-start source:PROJECT.md -->
## Project

**Codex Console Go Migration**

Codex Console is an existing brownfield account-management and registration console whose current product behavior mostly lives in the Python FastAPI/Jinja stack in `webui.py` and `src/`. `backend-go/` already provides a partial Go control plane with PostgreSQL/Redis, jobs, accounts, and native registration components. This project initializes only the remaining migration work needed to replace the Python backend responsibilities with Go while preserving today's API surface, stored data contracts, and critical business behavior.

**Core Value:** The Go backend can take over the current Codex Console backend responsibilities without forcing existing clients, persisted data, or critical registration, payment, and team workflows to change behavior.

### Constraints

- **Compatibility**: Keep current API paths, HTTP methods, JSON fields, status values, and websocket semantics compatible - the existing frontend and automation surface already depends on them.
- **Data Contract**: Preserve current persisted record shapes and migration paths - breaking existing fields would invalidate live data and current operational workflows.
- **Brownfield Scope**: Only plan remaining migration work - already migrated Go foundations are baseline, not a fresh greenfield roadmap.
- **Execution Safety**: Python may remain as a bounded transition aid temporarily, but the final state cannot depend on Python on the critical backend path.
- **Operational Parity**: Registration, payment/bind-card, team, logs, and admin workflows must keep current business behavior so operators do not need a new playbook just because the implementation moved to Go.
<!-- GSD:project-end -->

<!-- GSD:stack-start source:codebase/STACK.md -->
## Technology Stack

## Languages
- Python 3.10+ - 主应用与 WebUI 运行时，入口在 `webui.py`，主体代码在 `src/`，版本要求定义在 `pyproject.toml`，容器镜像使用 `Dockerfile` 中的 Python 3.11。
- Go 1.25.0 - 独立控制 API、任务 worker、PostgreSQL/Redis 队列处理与原生注册 runner，代码位于 `backend-go/`，版本定义在 `backend-go/go.mod`。
- JavaScript / Node.js - 仅检测到工具链依赖而非前端构建系统，依赖定义在 `package.json` / `package-lock.json`，当前仓库内未检测到对应的项目级 `npm` scripts 或业务源码引用。
- SQL - Python 侧通过 SQLAlchemy 管理 SQLite / PostgreSQL，Go 侧通过 `goose` 迁移和 `sqlc` 查询生成访问 PostgreSQL，相关文件在 `src/database/session.py`、`backend-go/db/migrations/*.sql`、`backend-go/db/query/jobs.sql`、`backend-go/sqlc.yaml`。
- Shell / PowerShell - 构建与容器启动脚本使用 Bash 和 PowerShell，见 `build.sh`、`scripts/docker/start-webui.sh`、`scripts/make_windows_test_bundle.ps1`。
## Runtime
- Python 运行时使用本地解释器或打包二进制，默认入口是 `webui.py`，打包配置在 `codex_register.spec`。
- Docker 运行时基于 `python:3.11-slim`，并附带 Xvfb、Fluxbox、x11vnc、websockify、noVNC 与 Playwright Chromium，定义在 `Dockerfile`。
- Go 运行时分为 API 进程 `backend-go/cmd/api/main.go` 和 worker 进程 `backend-go/cmd/worker/main.go`。
- Node.js 版本未在仓库中固定；锁定依赖中 `@puppeteer/browsers` 要求 `node >=18`，见 `package-lock.json`。
- `uv` - Python 推荐包管理器，仓库存在锁文件 `uv.lock`，安装方式记录在 `README.md`。
- `pip` - Python 兼容安装路径，依赖清单在 `requirements.txt`。
- `npm` - Node 工具依赖管理器，锁文件 `package-lock.json` 存在。
- `go` modules - Go 依赖管理，锁定在 `backend-go/go.mod` / `backend-go/go.sum`。
- Lockfile: `uv.lock`、`package-lock.json`、`backend-go/go.sum` present。
## Frameworks
- FastAPI `>=0.100.0` - WebUI 与 HTTP API 框架，应用装配在 `src/web/app.py`，依赖来源 `pyproject.toml` / `requirements.txt`。
- Uvicorn `>=0.23.0` - ASGI 服务启动器，启动参数与 WebSocket 适配定义在 `webui.py`。
- Jinja2 `>=3.1.0` - 服务端模板渲染，模板目录与全局变量注入见 `src/web/app.py`。
- SQLAlchemy `>=2.0.0` - Python 侧 ORM 与 SQLite/PostgreSQL 访问层，见 `src/database/session.py`。
- Pydantic `>=2.0.0` 与 `pydantic-settings >=2.0.0` - 设置模型与配置转换，见 `src/config/settings.py`。
- `curl-cffi >=0.14.0` - Python 侧统一 HTTP 客户端、OpenAI/Auth/支付/上传/邮箱服务请求基座，见 `src/core/http_client.py` 与 `src/core/openai/*.py`。
- `websockets >=16.0` - Python WebSocket 服务端支持，启用位置在 `webui.py`。
- `go-chi/chi v5.2.3` - Go API 路由器，见 `backend-go/internal/http/router.go`。
- `hibiken/asynq v0.26.0` - Go 后端任务队列与 worker 运行时，见 `backend-go/cmd/api/main.go`、`backend-go/cmd/worker/main.go`。
- `jackc/pgx/v5 v5.9.1` - Go 侧 PostgreSQL 连接池，见 `backend-go/internal/platform/postgres/postgres.go`。
- `redis/go-redis/v9 v9.18.0` - Go 侧 Redis 客户端，见 `backend-go/internal/platform/redis/redis.go`。
- `pytest >=7.0.0` - Python 测试框架，配置在 `pyproject.toml` 的 `[tool.pytest.ini_options]`。
- `httpx >=0.24.0` - Python 测试 HTTP 客户端依赖，列在 `pyproject.toml` 和 `requirements.txt`。
- Go 原生 `go test` - Go 单测、迁移测试、e2e 测试通过 `backend-go/Makefile` 组织。
- Hatchling - Python 包构建后端，定义在 `pyproject.toml`。
- PyInstaller `>=6.19.0` - 桌面二进制打包链路，依赖在 `pyproject.toml`，spec 文件为 `codex_register.spec`，CI 入口在 `.github/workflows/build.yml`。
- Playwright `>=1.40.0` - Python 支付/绑卡自动化与容器浏览器依赖，定义在 `pyproject.toml` 可选依赖、`requirements.txt`、`Dockerfile`。
- `sqlc` - Go 查询代码生成工具，配置在 `backend-go/sqlc.yaml`，命令在 `backend-go/Makefile`。
- `goose v3.27.0` - Go PostgreSQL 迁移工具，依赖在 `backend-go/go.mod`，命令在 `backend-go/Makefile`。
- Docker Buildx / GitHub Actions - 镜像构建与发布链路，见 `.github/workflows/docker-image.yml` 与 `.github/workflows/docker-publish.yml`。
## Key Dependencies
- `curl-cffi` - 所有高仿真浏览器 HTTP 请求、OpenAI/Auth/ChatGPT/邮箱服务/上传服务都基于它实现，关键文件包括 `src/core/http_client.py`、`src/core/openai/oauth.py`、`src/core/openai/payment.py`、`src/services/tempmail.py`。
- `playwright` - 绑卡与浏览器自动化能力依赖它，相关逻辑在 `src/core/openai/browser_bind.py`、`src/core/openai/payment.py`，Docker 镜像会预装 Chromium。
- `sqlalchemy` + `aiosqlite` + `psycopg[binary]` - Python 侧本地 SQLite 与远程 PostgreSQL 双栈数据库支持，见 `src/database/session.py` 与 `src/config/settings.py`。
- `go-chi/chi`, `hibiken/asynq`, `pgx/v5`, `redis/go-redis/v9` - Go API、队列、数据库与缓存核心依赖，分别对应 `backend-go/internal/http/router.go`、`backend-go/cmd/worker/main.go`、`backend-go/internal/platform/postgres/postgres.go`、`backend-go/internal/platform/redis/redis.go`。
- PostgreSQL - Python 可选远程数据库、Go 强制主数据库，连接规则见 `src/database/session.py` 与 `backend-go/internal/config/config.go`。
- SQLite - Python 默认本地数据库，默认文件在 `data/database.db`，见 `src/database/session.py`。
- Redis - Go worker 队列、租约、任务状态依赖项，见 `backend-go/internal/config/config.go` 与 `backend-go/cmd/worker/main.go`。
- Xvfb / Fluxbox / x11vnc / websockify / noVNC - Docker 中提供有界面浏览器运行环境，见 `Dockerfile` 与 `scripts/docker/start-webui.sh`。
- `axios ^1.14.0` 与 `puppeteer-core ^24.40.0` - 仅在 Node 清单中声明，当前仓库未检测到业务级直接引用；将其视为保留型工具依赖，依据为 `package.json` 与 `package-lock.json`。
## Configuration
- Python 主应用以数据库设置为主，环境变量只做覆盖层；实现见 `src/config/settings.py`、`webui.py`。
- 使用 `APP_DATABASE_URL` 或 `DATABASE_URL` 将 Python 默认 SQLite 切换为 PostgreSQL；数据库 URL 规范化逻辑在 `src/config/settings.py` 与 `src/database/session.py`。
- 使用 `APP_HOST`、`APP_PORT`、`APP_ACCESS_PASSWORD` 覆盖 Python WebUI 监听与认证；CLI 入口额外支持 `WEBUI_HOST`、`WEBUI_PORT`、`WEBUI_ACCESS_PASSWORD`、`DEBUG`、`LOG_LEVEL`，见 `webui.py`。
- Go 后端完全通过环境变量驱动，至少要求 `DATABASE_URL` 与 `REDIS_ADDR`；完整列表定义在 `backend-go/internal/config/config.go`。
- Docker 运行时额外依赖 `DISPLAY`、`ENABLE_VNC`、`VNC_PORT`、`NOVNC_PORT` 等图形环境变量，见 `Dockerfile`、`docker-compose.yml`、`scripts/docker/start-webui.sh`。
- Python 构建文件：`pyproject.toml`、`requirements.txt`、`uv.lock`、`codex_register.spec`。
- Docker 构建文件：`Dockerfile`、`docker-compose.yml`、`scripts/docker/start-webui.sh`。
- Go 构建与代码生成文件：`backend-go/go.mod`、`backend-go/Makefile`、`backend-go/sqlc.yaml`。
- CI/CD 文件：`.github/workflows/build.yml`、`.github/workflows/docker-image.yml`、`.github/workflows/docker-publish.yml`。
## Platform Requirements
- Python 3.10+，推荐使用 `uv sync`，也兼容 `pip install -r requirements.txt`；依据 `README.md` 与 `pyproject.toml`。
- 若需要支付/绑卡自动化，本地环境还要具备 Playwright Chromium；安装方式写在 `Dockerfile` 与 `scripts/make_windows_test_bundle.ps1`。
- Go 后端开发需要 Go 1.25.0、PostgreSQL、Redis，以及可选的 `sqlc` / `goose` 命令；依据 `backend-go/go.mod` 与 `backend-go/Makefile`。
- Docker 开发模式需要宿主机开放 `1455`、`6080`，并挂载 `./data` 与 `./logs`；见 `docker-compose.yml`。
- Python 应用可以直接运行 `python webui.py`、以 PyInstaller 二进制发布，或通过 GHCR Docker 镜像运行；发布链路见 `README.md`、`codex_register.spec`、`.github/workflows/build.yml`、`.github/workflows/docker-image.yml`。
- Go 后端部署目标是独立 API + worker 进程，依赖外部 PostgreSQL 与 Redis；入口在 `backend-go/cmd/api/main.go` 与 `backend-go/cmd/worker/main.go`。
- 当前仓库是混合栈而非单一迁移完成态。`backend-go/README.md` 明确标注部分注册与支付链路尚未完全迁移，worker 仍保留 Python bridge，见 `backend-go/internal/registration/python_runner.go` 与 `backend-go/internal/registration/python_runner_script.go`。
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

## Naming Patterns
- Python application files use `snake_case.py`; route modules follow resource-oriented names such as `src/web/routes/team.py` and `src/web/routes/team_tasks.py`.
- Python tests use `tests/test_*.py`; one standalone diagnostic script exists at `test_bitbrowser_checkout.py`, but it sits outside the configured pytest `testpaths`.
- Go source uses lowercase package directories under `backend-go/internal/...`; tests live beside the package as `*_test.go`, for example `backend-go/internal/accounts/service_test.go`.
- Python functions and methods use `snake_case`; internal helpers are prefixed with `_`, for example `_enqueue_team_write_task` in `src/web/routes/team.py`, `_build_account_lookup` in `src/services/team/sync.py`, and `_run_with_sqlite_lock_retry` in `src/database/crud.py`.
- FastAPI route handlers are verb-led `async def` functions such as `run_team_discovery`, `sync_team`, and `get_team_detail` in `src/web/routes/team.py`.
- Go exports use PascalCase (`NewService`, `ListAccounts`, `UpsertAccount` in `backend-go/internal/accounts/service.go`); package-private helpers use mixedCase (`mergeAccount`, `removeTemporaryAccountExtraData`).
- Python module constants use uppercase names with underscores (`_TEAM_API_BASE_URL`, `STATIC_DIR`, `GHOST_SUCCESS_WINDOW_SECONDS` in `src/services/team/client.py`, `src/web/app.py`, and `src/services/team/sync.py`).
- Python module loggers are consistently named `logger` (`src/web/app.py`, `src/services/team/runner.py`, `src/core/openai/payment.py`).
- Go locals are short and descriptive (`req`, `resp`, `repo`, `svc`); exported struct fields stay PascalCase to match package API types (`backend-go/internal/accounts/types.go`, `backend-go/internal/accounts/service.go`).
- Python request/response models, settings structs, enums, and exceptions use PascalCase: `TeamDiscoveryRunRequest` in `src/web/routes/team.py`, `SettingDefinition` and `SettingCategory` in `src/config/settings.py`, `TeamSyncNotFoundError` in `src/services/team/sync.py`.
- Go structs and interfaces use PascalCase for exports and capability-style names for boundaries: `Repository`, `Service`, `accountsService`, and `outlookRouteService` in `backend-go/internal/accounts/service.go` and `backend-go/internal/http/router.go`.
## Code Style
- No repository-wide formatter or editor config was detected. `.editorconfig`, `ruff`, `black`, `flake8`, `mypy`, and `golangci-lint` config files are absent from the project root.
- Python follows 4-space indentation, blank lines between import groups, and frequent module/function docstrings in Chinese (`src/web/app.py`, `src/config/settings.py`, `src/services/outlook/base.py`).
- Preserve file-local typing style. Newer Python modules use `from __future__ import annotations` plus built-in generics and `|` unions (`src/services/team/client.py`, `src/services/team/sync.py`, `src/web/routes/team.py`). Older modules still use `Optional`, `List`, and `Dict` from `typing` (`src/web/app.py`, `src/database/crud.py`, `src/config/settings.py`).
- Go follows `gofmt` layout: grouped imports, tab indentation, constructors named `New...`, and early returns (`backend-go/internal/accounts/service.go`, `backend-go/internal/http/router.go`, `backend-go/internal/accounts/http/handlers.go`).
- Not detected as enforced tooling. Keep edits aligned with the surrounding file instead of normalizing the whole repository.
- Python uses local exceptions to the dominant style when needed, for example `# pragma: no cover` around optional dependency fallback in `src/services/team/client.py`.
## Import Organization
- Python has no custom alias config. Service/data modules often import from the project root package (`src.services...`, `src.database...`), as seen in `src/services/team/sync.py`.
- Python web modules mix package-relative imports inside `src/web/...`, for example `src/web/app.py` imports `..config.settings` and `.routes`.
- Go imports use full module paths rooted at `github.com/dou-jiang/codex-console/backend-go/...` and add aliases when names would collide, such as `internalhttp`, `accountshttp`, `registrationhttp`, and `accountspkg` in `backend-go/internal/http/router.go`, `backend-go/internal/accounts/http/handlers_test.go`, and `backend-go/tests/e2e/accounts_flow_test.go`.
## Error Handling
- Python validates early and raises domain exceptions or `HTTPException` at route boundaries (`src/services/team/client.py`, `src/services/team/sync.py`, `src/web/routes/team.py`).
- Python write paths wrap `commit` and `rollback` explicitly rather than hiding transaction state, as in `src/database/crud.py`.
- Python retries transient SQLite lock failures through small helpers instead of open-coded loops (`src/database/crud.py`).
- Go service and repository code returns `(value, error)` and wraps lower-level failures with `fmt.Errorf("context: %w", err)` (`backend-go/internal/accounts/service.go`).
- Go HTTP adapters translate decode and service errors with `http.Error` and centralize JSON writing in helper functions (`backend-go/internal/accounts/http/handlers.go`).
## Logging
- Python operational modules define `logger = logging.getLogger(__name__)` at module scope (`src/web/app.py`, `src/core/http_client.py`, `src/services/team/runner.py`).
- Python log messages are operator-facing and often Chinese; keep that tone for new runtime logs.
- Go logging is concentrated in command entrypoints such as `backend-go/cmd/api/main.go` and `backend-go/cmd/worker/main.go`. The sampled internal packages are mostly logger-free, so keep domain/service packages quiet unless the sibling code already logs.
## Comments
- Use short Chinese module docstrings and targeted inline comments for compatibility branches, environment differences, or state-machine edge cases (`src/web/app.py`, `src/config/settings.py`, `src/services/outlook_legacy_mail.py`).
- Prefer comments that explain why a branch exists, not what a straightforward line already states.
- Keep compatibility contracts explicit when a module preserves legacy or upstream response shapes (`src/services/team/client.py`, `backend-go/internal/http/router.go`).
- Not applicable in this repository.
- Python docstrings are common on modules, classes, and non-trivial helpers. Go relies more on names and short inline comments than on exported docblocks in the sampled packages.
## Function Design
- Python route modules favor many small private helpers plus thin endpoints (`src/web/routes/team.py`, `src/web/routes/__init__.py`).
- Longer Python service modules still isolate parsing and merge logic into small helpers before the main workflow (`src/services/team/sync.py`, `src/database/crud.py`).
- Go service files keep orchestration methods short and move merge/normalization logic into package-private helpers (`backend-go/internal/accounts/service.go`).
- Python switches to keyword-only parameters once a function grows beyond one or two control inputs, for example `_enqueue_team_write_task` in `src/web/routes/team.py` and `_fetch_member_pages` in `src/services/team/sync.py`.
- FastAPI request bodies are modeled as `BaseModel` subclasses close to the route handler (`src/web/routes/team.py`).
- Go passes explicit request structs through service layers (`ListAccountsRequest`, `UpsertAccountRequest` in `backend-go/internal/accounts/service.go`) and keeps interfaces narrow (`backend-go/internal/accounts/http/handlers.go`).
- Python commonly returns plain `dict[str, Any]` payloads, ORM models, or Pydantic models instead of dedicated DTO wrappers (`src/services/team/client.py`, `src/web/routes/team.py`).
- Go services return typed structs and errors; `map[string]any` is mostly reserved for compatibility JSON payloads and tests (`backend-go/internal/accounts/service.go`, `backend-go/internal/jobs/http/handlers.go`).
## Module Design
- Python exposes package APIs through `__init__.py` re-exports and `__all__`, including lazy exports in `src/services/team/__init__.py`.
- FastAPI route mounting is centralized in `src/web/routes/__init__.py`; add a new router module there after implementing it.
- Go packages expose constructors and request/response types from the package root, while HTTP adapters live in nested `http` subpackages such as `backend-go/internal/accounts/http`.
- Python barrel files are used and should be updated when public package APIs change: `src/services/team/__init__.py`, `src/services/outlook/__init__.py`, and `src/web/routes/__init__.py`.
- Go does not use barrel files. Package boundaries are the organizational unit.
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

## Pattern Overview
- Keep the Python app as the primary Web UI, template renderer, and legacy registration executor through `webui.py`, `src/web/app.py`, and `src/core/register.py`.
- Organize Python server code by feature routes in `src/web/routes/`, backed by shared persistence in `src/database/` and domain/integration logic in `src/core/` and `src/services/`.
- Split the Go backend into API and worker binaries in `backend-go/cmd/api/main.go` and `backend-go/cmd/worker/main.go`, with domain packages under `backend-go/internal/`.
## Layers
- Purpose: Start the Python process, initialize storage, logging, timezone, and runtime directories, then boot the FastAPI app.
- Location: `webui.py`, `src/config/settings.py`, `src/core/utils.py`, `src/core/timezone_utils.py`, `src/core/db_logs.py`, `src/database/init_db.py`
- Contains: CLI parsing, `.env` loading, runtime directory setup, settings resolution, logging bootstrap, database initialization.
- Depends on: `src/config/`, `src/database/`, `src/web/`
- Used by: Local launches, packaged builds, Docker entrypoint `scripts/docker/start-webui.sh`
- Purpose: Expose HTML pages, JSON APIs, and WebSocket streams for registration, accounts, settings, logs, payment, and team workflows.
- Location: `src/web/app.py`, `src/web/routes/`, `src/web/task_manager.py`, `templates/`, `static/`
- Contains: FastAPI app factory, page routes, API routers, WebSocket endpoints, runtime task state, server-rendered templates, page-scoped JavaScript.
- Depends on: `src/database/`, `src/core/`, `src/services/`, `src/config/`
- Used by: Browser clients hitting `/`, `/accounts`, `/api/*`, and `/api/ws/*`
- Purpose: Execute OpenAI registration/login flows, payment helpers, upload/export flows, email-provider integrations, and team automation.
- Location: `src/core/register.py`, `src/core/openai/*.py`, `src/core/anyauto/*.py`, `src/core/upload/*.py`, `src/services/*.py`, `src/services/outlook/*.py`, `src/services/team/*.py`
- Contains: `RegistrationEngine`, HTTP/OpenAI helpers, email-service adapters, Outlook-specific providers, CPA/Sub2API/TM upload helpers, team discovery/sync/invite logic.
- Depends on: External HTTP services, `src/database/`, `src/config/`
- Used by: `src/web/routes/registration.py`, `src/web/routes/accounts.py`, `src/web/routes/payment.py`, `src/web/routes/team.py`
- Purpose: Store settings, accounts, registration tasks, team entities, proxies, upload target configs, and app logs.
- Location: `src/database/session.py`, `src/database/models.py`, `src/database/team_models.py`, `src/database/crud.py`, `src/database/team_crud.py`
- Contains: SQLAlchemy engine/session management, ORM models, CRUD helpers, SQLite migration repair logic, team-specific persistence helpers.
- Depends on: SQLAlchemy and the active database URL.
- Used by: Nearly every Python route and service module.
- Purpose: Serve compatibility and migration APIs for accounts, jobs, registration, batch registration, and WebSocket task streams.
- Location: `backend-go/cmd/api/main.go`, `backend-go/internal/http/router.go`, `backend-go/internal/accounts/http/handlers.go`, `backend-go/internal/jobs/http/handlers.go`, `backend-go/internal/registration/http/handlers.go`, `backend-go/internal/registration/ws/*.go`
- Contains: Dependency bootstrap, chi router assembly, HTTP handlers, manual WebSocket handlers, health endpoint.
- Depends on: `backend-go/internal/accounts`, `backend-go/internal/jobs`, `backend-go/internal/registration`, `backend-go/internal/platform/*`
- Used by: Future frontend/API callers and Go e2e tests in `backend-go/tests/e2e/`
- Purpose: Run queued registration jobs, execute native or Python-backed registration flows, and trigger post-registration uploads.
- Location: `backend-go/cmd/worker/main.go`, `backend-go/internal/jobs/*.go`, `backend-go/internal/registration/*.go`, `backend-go/internal/nativerunner/*.go`, `backend-go/internal/uploader/*.go`
- Contains: Asynq worker bootstrap, job service, registration executor/orchestrator, native runner bridge, Python runner bridge, uploader payload builders and senders.
- Depends on: PostgreSQL, Redis, Asynq, `backend-go/internal/accounts`
- Used by: Jobs enqueued through the Go API.
- Purpose: Open database/Redis connections and define the PostgreSQL schema/query boundary used by Go services.
- Location: `backend-go/internal/platform/postgres/postgres.go`, `backend-go/internal/platform/redis/redis.go`, `backend-go/db/migrations/*.sql`, `backend-go/db/query/jobs.sql`, `backend-go/internal/jobs/sqlc/*.go`, `backend-go/sqlc.yaml`
- Contains: Connection bootstrap, SQL migrations, sqlc query definitions, generated query models and executors.
- Depends on: Environment variables, PostgreSQL, Redis, `sqlc`
- Used by: `backend-go/cmd/api/main.go`, `backend-go/cmd/worker/main.go`, repository packages.
## Data Flow
- Python request/response state is stateless at the HTTP layer, but long-running task state lives in `src/web/task_manager.py` and durable records live in `src/database/models.py` / `src/database/team_models.py`.
- Browser pages maintain page-local mutable state inside `static/js/app.js`, `static/js/accounts.js`, `static/js/auto_team.js`, and related helpers, combining REST polling with WebSocket updates.
- Go treats PostgreSQL as the source of truth for jobs/accounts/registration data, Redis/Asynq as the execution queue and lease store, and `backend-go/internal/registration/batch_service.go` as in-memory aggregation for batch websocket state.
## Key Abstractions
- Purpose: Encapsulate the Python-side OpenAI signup/login/token acquisition workflow.
- Examples: `src/core/register.py`, `tests/test_registration_engine.py`, `tests/test_anyauto_register_flow.py`
- Pattern: Stateful orchestration object with many private workflow steps and a final persistence handoff.
- Purpose: Central runtime coordinator for Python registration/team task logs, pause/resume/cancel flags, and WebSocket subscriber bookkeeping.
- Examples: `src/web/task_manager.py`, `src/web/routes/websocket.py`, `src/web/routes/registration.py`
- Pattern: Process-local singleton with thread-safe maps plus async broadcast helpers.
- Purpose: Normalize mailbox-provider adapters behind one creation API.
- Examples: `src/services/base.py`, `src/services/duck_mail.py`, `src/services/temp_mail.py`, `src/services/outlook/service.py`, `src/services/outlook/providers/*.py`
- Pattern: Factory + adapter pattern over provider-specific implementations.
- Purpose: Provide engine/session lifecycle, SQLite pragmas, and lightweight in-place schema repair for the Python app.
- Examples: `src/database/session.py`, `src/database/init_db.py`
- Pattern: Shared infrastructure wrapper with context-managed sessions and startup migration hooks.
- Purpose: Isolate job creation, queueing, status transitions, and log persistence from the HTTP and worker layers.
- Examples: `backend-go/internal/jobs/service.go`, `backend-go/internal/jobs/repository_runtime.go`, `backend-go/internal/jobs/http/handlers.go`
- Pattern: Application service over repository + queue interfaces.
- Purpose: Separate registration request preparation from execution and persistence.
- Examples: `backend-go/internal/registration/orchestrator.go`, `backend-go/internal/registration/executor.go`, `backend-go/internal/registration/python_runner.go`, `backend-go/internal/registration/native_runner.go`
- Pattern: Service orchestration with dependency-injected preparation, runner, persistence, and upload hooks.
- Purpose: Translate normalized account records into CPA/Sub2API/Team Manager payloads.
- Examples: `src/core/upload/*.py`, `backend-go/internal/uploader/builder.go`, `backend-go/internal/uploader/sender.go`
- Pattern: Builder + sender split, keeping payload construction separate from HTTP transport.
## Entry Points
- Location: `webui.py`
- Triggers: `python webui.py`, packaged executable entrypoint, `codex-console` console script from `pyproject.toml`
- Responsibilities: Load environment overrides, initialize data/log dirs, initialize database, configure logging, and run Uvicorn.
- Location: `src/web/app.py`
- Triggers: Imported by Uvicorn from `webui.py`
- Responsibilities: Mount static assets, register routers, expose HTML pages, configure template globals, and start background maintenance jobs.
- Location: `backend-go/cmd/api/main.go`
- Triggers: `go run ./cmd/api`, `make run-api`
- Responsibilities: Load env config, open PostgreSQL/Redis, construct services, wire HTTP and WebSocket handlers, start `http.ListenAndServe`.
- Location: `backend-go/cmd/worker/main.go`
- Triggers: `go run ./cmd/worker`, `make run-worker`
- Responsibilities: Load env config, open PostgreSQL/Redis, start Asynq worker server, run registration executor, and dispatch uploads.
- Location: `scripts/docker/start-webui.sh`
- Triggers: Docker image startup
- Responsibilities: Optionally start Xvfb/Fluxbox/x11vnc/noVNC, then exec `python webui.py`.
- Location: `scripts/bitbrowser_connect.py`
- Triggers: Manual CLI use and `tests/test_bitbrowser_connect_script.py`
- Responsibilities: Query the local BitBrowser API and emit normalized JSON connection metadata.
## Error Handling
- Python APIs validate request models with Pydantic classes inside route files such as `src/web/routes/registration.py` and `src/web/routes/accounts.py`, then raise `HTTPException` for invalid inputs.
- Python persistence retries transient SQLite lock failures in `src/database/crud.py`, while `src/database/session.py` enables WAL and busy timeouts for concurrent access.
- Python background tasks convert execution failures into task status/log updates through `src/web/task_manager.py`, `src/web/routes/registration.py`, and `src/services/team/runner.py`.
- Go services return typed errors such as `ErrBatchNotFound`, `ErrQueueNotConfigured`, and config parse errors from `backend-go/internal/registration/batch_service.go`, `backend-go/internal/jobs/service.go`, and `backend-go/internal/config/config.go`.
- Go handlers keep request decoding thin and map service failures inside `backend-go/internal/registration/http/handlers.go` and `backend-go/internal/accounts/http/handlers.go`.
## Cross-Cutting Concerns
<!-- GSD:architecture-end -->

<!-- GSD:skills-start source:skills/ -->
## Project Skills

No project skills found. Add skills to any of: `.claude/skills/`, `.agents/skills/`, `.cursor/skills/`, or `.github/skills/` with a `SKILL.md` index file.
<!-- GSD:skills-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd-quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd-debug` for investigation and bug fixing
- `/gsd-execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->



<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd-profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
