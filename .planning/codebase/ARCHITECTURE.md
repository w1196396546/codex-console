# Architecture

**Analysis Date:** 2026-04-05

## Pattern Overview

**Overall:** Hybrid dual-runtime monolith. The repository keeps the existing Python FastAPI + Jinja2 application in `webui.py` and `src/`, while a newer Go control plane lives under `backend-go/` for job orchestration, worker execution, and PostgreSQL/Redis-backed APIs.

**Key Characteristics:**
- Keep the Python app as the primary Web UI, template renderer, and legacy registration executor through `webui.py`, `src/web/app.py`, and `src/core/register.py`.
- Organize Python server code by feature routes in `src/web/routes/`, backed by shared persistence in `src/database/` and domain/integration logic in `src/core/` and `src/services/`.
- Split the Go backend into API and worker binaries in `backend-go/cmd/api/main.go` and `backend-go/cmd/worker/main.go`, with domain packages under `backend-go/internal/`.

## Layers

**Python Bootstrap & Runtime:**
- Purpose: Start the Python process, initialize storage, logging, timezone, and runtime directories, then boot the FastAPI app.
- Location: `webui.py`, `src/config/settings.py`, `src/core/utils.py`, `src/core/timezone_utils.py`, `src/core/db_logs.py`, `src/database/init_db.py`
- Contains: CLI parsing, `.env` loading, runtime directory setup, settings resolution, logging bootstrap, database initialization.
- Depends on: `src/config/`, `src/database/`, `src/web/`
- Used by: Local launches, packaged builds, Docker entrypoint `scripts/docker/start-webui.sh`

**Python Web Interface:**
- Purpose: Expose HTML pages, JSON APIs, and WebSocket streams for registration, accounts, settings, logs, payment, and team workflows.
- Location: `src/web/app.py`, `src/web/routes/`, `src/web/task_manager.py`, `templates/`, `static/`
- Contains: FastAPI app factory, page routes, API routers, WebSocket endpoints, runtime task state, server-rendered templates, page-scoped JavaScript.
- Depends on: `src/database/`, `src/core/`, `src/services/`, `src/config/`
- Used by: Browser clients hitting `/`, `/accounts`, `/api/*`, and `/api/ws/*`

**Python Domain & Integration Logic:**
- Purpose: Execute OpenAI registration/login flows, payment helpers, upload/export flows, email-provider integrations, and team automation.
- Location: `src/core/register.py`, `src/core/openai/*.py`, `src/core/anyauto/*.py`, `src/core/upload/*.py`, `src/services/*.py`, `src/services/outlook/*.py`, `src/services/team/*.py`
- Contains: `RegistrationEngine`, HTTP/OpenAI helpers, email-service adapters, Outlook-specific providers, CPA/Sub2API/TM upload helpers, team discovery/sync/invite logic.
- Depends on: External HTTP services, `src/database/`, `src/config/`
- Used by: `src/web/routes/registration.py`, `src/web/routes/accounts.py`, `src/web/routes/payment.py`, `src/web/routes/team.py`

**Python Persistence:**
- Purpose: Store settings, accounts, registration tasks, team entities, proxies, upload target configs, and app logs.
- Location: `src/database/session.py`, `src/database/models.py`, `src/database/team_models.py`, `src/database/crud.py`, `src/database/team_crud.py`
- Contains: SQLAlchemy engine/session management, ORM models, CRUD helpers, SQLite migration repair logic, team-specific persistence helpers.
- Depends on: SQLAlchemy and the active database URL.
- Used by: Nearly every Python route and service module.

**Go API Layer:**
- Purpose: Serve compatibility and migration APIs for accounts, jobs, registration, batch registration, and WebSocket task streams.
- Location: `backend-go/cmd/api/main.go`, `backend-go/internal/http/router.go`, `backend-go/internal/accounts/http/handlers.go`, `backend-go/internal/jobs/http/handlers.go`, `backend-go/internal/registration/http/handlers.go`, `backend-go/internal/registration/ws/*.go`
- Contains: Dependency bootstrap, chi router assembly, HTTP handlers, manual WebSocket handlers, health endpoint.
- Depends on: `backend-go/internal/accounts`, `backend-go/internal/jobs`, `backend-go/internal/registration`, `backend-go/internal/platform/*`
- Used by: Future frontend/API callers and Go e2e tests in `backend-go/tests/e2e/`

**Go Worker & Execution Layer:**
- Purpose: Run queued registration jobs, execute native or Python-backed registration flows, and trigger post-registration uploads.
- Location: `backend-go/cmd/worker/main.go`, `backend-go/internal/jobs/*.go`, `backend-go/internal/registration/*.go`, `backend-go/internal/nativerunner/*.go`, `backend-go/internal/uploader/*.go`
- Contains: Asynq worker bootstrap, job service, registration executor/orchestrator, native runner bridge, Python runner bridge, uploader payload builders and senders.
- Depends on: PostgreSQL, Redis, Asynq, `backend-go/internal/accounts`
- Used by: Jobs enqueued through the Go API.

**Go Infrastructure & Schema Layer:**
- Purpose: Open database/Redis connections and define the PostgreSQL schema/query boundary used by Go services.
- Location: `backend-go/internal/platform/postgres/postgres.go`, `backend-go/internal/platform/redis/redis.go`, `backend-go/db/migrations/*.sql`, `backend-go/db/query/jobs.sql`, `backend-go/internal/jobs/sqlc/*.go`, `backend-go/sqlc.yaml`
- Contains: Connection bootstrap, SQL migrations, sqlc query definitions, generated query models and executors.
- Depends on: Environment variables, PostgreSQL, Redis, `sqlc`
- Used by: `backend-go/cmd/api/main.go`, `backend-go/cmd/worker/main.go`, repository packages.

## Data Flow

**Python Registration Flow:**

1. `templates/index.html` renders the registration UI and loads `static/js/app.js`.
2. `static/js/app.js` submits JSON to `/api/registration/start` or batch endpoints exposed by `src/web/routes/registration.py`.
3. `src/web/routes/registration.py` persists a `RegistrationTask` through `src/database/crud.py`, then schedules `run_registration_task()` as a FastAPI background task.
4. `run_registration_task()` hands synchronous work to the thread pool owned by `src/web/task_manager.py`, which also buffers logs and WebSocket state.
5. `_run_sync_registration_task()` in `src/web/routes/registration.py` delegates to `src/core/register.py` and email-service factories in `src/services/base.py` / `src/services/*.py`.
6. `RegistrationEngine.save_to_database()` and CRUD helpers in `src/database/crud.py` persist accounts, task status, and related metadata.
7. `src/web/routes/websocket.py` streams runtime status/logs from `task_manager`, while the browser reconnect logic in `static/js/app.js` keeps the page in sync.

**Python Team Automation Flow:**

1. `templates/auto_team.html` renders team-management UI and loads `static/js/auto_team.js`.
2. Requests land in `src/web/routes/team.py` and task-inspection endpoints in `src/web/routes/team_tasks.py`.
3. `src/web/routes/team.py` creates `TeamTask` records through `src/services/team/tasks.py` and schedules execution via `src/services/team/runner.py`.
4. `src/services/team/runner.py` dispatches to `src/services/team/discovery.py`, `src/services/team/sync.py`, `src/services/team/invite.py`, and related helpers.
5. Team state is persisted in `src/database/team_models.py` and `src/database/team_crud.py`, while `src/web/task_manager.py` carries live task status.

**Python Page Rendering Flow:**

1. `src/web/app.py` builds the FastAPI app, mounts `/static`, and configures `Jinja2Templates` for `templates/`.
2. Page handlers in `src/web/app.py` guard most HTML routes with cookie auth, then render `templates/*.html`.
3. Each page-specific script in `static/js/*.js` calls matching API routes in `src/web/routes/*.py`.
4. Static asset cache busting is driven by `_build_static_asset_version()` in `src/web/app.py`.

**Go Registration / Job Flow:**

1. `backend-go/internal/registration/http/handlers.go` accepts `/api/registration/start`, `/api/registration/batch`, and Outlook batch requests.
2. `backend-go/internal/registration/service.go` or `backend-go/internal/registration/batch_service.go` converts the request into jobs via `backend-go/internal/jobs/service.go`.
3. `backend-go/internal/jobs/service.go` persists jobs and logs through its repository, then enqueues work on Asynq.
4. `backend-go/cmd/worker/main.go` boots an Asynq server and wires `backend-go/internal/registration/executor.go`.
5. The executor prepares runtime inputs through `backend-go/internal/registration/orchestrator.go`, then runs either `backend-go/internal/registration/native_runner.go` (wrapping `backend-go/internal/nativerunner/`) or `backend-go/internal/registration/python_runner.go`.
6. Successful registration persists account state through `backend-go/internal/accounts/service.go` and `backend-go/internal/accounts/repository_postgres.go`, then optional upload dispatchers route to `backend-go/internal/uploader/`.
7. `backend-go/internal/registration/ws/task_socket.go` and `backend-go/internal/registration/ws/batch_socket.go` poll the jobs service and emit task/batch snapshots to WebSocket clients.

**State Management:**
- Python request/response state is stateless at the HTTP layer, but long-running task state lives in `src/web/task_manager.py` and durable records live in `src/database/models.py` / `src/database/team_models.py`.
- Browser pages maintain page-local mutable state inside `static/js/app.js`, `static/js/accounts.js`, `static/js/auto_team.js`, and related helpers, combining REST polling with WebSocket updates.
- Go treats PostgreSQL as the source of truth for jobs/accounts/registration data, Redis/Asynq as the execution queue and lease store, and `backend-go/internal/registration/batch_service.go` as in-memory aggregation for batch websocket state.

## Key Abstractions

**RegistrationEngine:**
- Purpose: Encapsulate the Python-side OpenAI signup/login/token acquisition workflow.
- Examples: `src/core/register.py`, `tests/test_registration_engine.py`, `tests/test_anyauto_register_flow.py`
- Pattern: Stateful orchestration object with many private workflow steps and a final persistence handoff.

**TaskManager:**
- Purpose: Central runtime coordinator for Python registration/team task logs, pause/resume/cancel flags, and WebSocket subscriber bookkeeping.
- Examples: `src/web/task_manager.py`, `src/web/routes/websocket.py`, `src/web/routes/registration.py`
- Pattern: Process-local singleton with thread-safe maps plus async broadcast helpers.

**Email Service Factory:**
- Purpose: Normalize mailbox-provider adapters behind one creation API.
- Examples: `src/services/base.py`, `src/services/duck_mail.py`, `src/services/temp_mail.py`, `src/services/outlook/service.py`, `src/services/outlook/providers/*.py`
- Pattern: Factory + adapter pattern over provider-specific implementations.

**Database Session Manager:**
- Purpose: Provide engine/session lifecycle, SQLite pragmas, and lightweight in-place schema repair for the Python app.
- Examples: `src/database/session.py`, `src/database/init_db.py`
- Pattern: Shared infrastructure wrapper with context-managed sessions and startup migration hooks.

**Go Jobs Service:**
- Purpose: Isolate job creation, queueing, status transitions, and log persistence from the HTTP and worker layers.
- Examples: `backend-go/internal/jobs/service.go`, `backend-go/internal/jobs/repository_runtime.go`, `backend-go/internal/jobs/http/handlers.go`
- Pattern: Application service over repository + queue interfaces.

**Go Registration Executor & Orchestrator:**
- Purpose: Separate registration request preparation from execution and persistence.
- Examples: `backend-go/internal/registration/orchestrator.go`, `backend-go/internal/registration/executor.go`, `backend-go/internal/registration/python_runner.go`, `backend-go/internal/registration/native_runner.go`
- Pattern: Service orchestration with dependency-injected preparation, runner, persistence, and upload hooks.

**Uploader Payload Builders:**
- Purpose: Translate normalized account records into CPA/Sub2API/Team Manager payloads.
- Examples: `src/core/upload/*.py`, `backend-go/internal/uploader/builder.go`, `backend-go/internal/uploader/sender.go`
- Pattern: Builder + sender split, keeping payload construction separate from HTTP transport.

## Entry Points

**Python CLI / Web UI Launcher:**
- Location: `webui.py`
- Triggers: `python webui.py`, packaged executable entrypoint, `codex-console` console script from `pyproject.toml`
- Responsibilities: Load environment overrides, initialize data/log dirs, initialize database, configure logging, and run Uvicorn.

**Python FastAPI App Factory:**
- Location: `src/web/app.py`
- Triggers: Imported by Uvicorn from `webui.py`
- Responsibilities: Mount static assets, register routers, expose HTML pages, configure template globals, and start background maintenance jobs.

**Go Control API:**
- Location: `backend-go/cmd/api/main.go`
- Triggers: `go run ./cmd/api`, `make run-api`
- Responsibilities: Load env config, open PostgreSQL/Redis, construct services, wire HTTP and WebSocket handlers, start `http.ListenAndServe`.

**Go Worker:**
- Location: `backend-go/cmd/worker/main.go`
- Triggers: `go run ./cmd/worker`, `make run-worker`
- Responsibilities: Load env config, open PostgreSQL/Redis, start Asynq worker server, run registration executor, and dispatch uploads.

**Container Bootstrap:**
- Location: `scripts/docker/start-webui.sh`
- Triggers: Docker image startup
- Responsibilities: Optionally start Xvfb/Fluxbox/x11vnc/noVNC, then exec `python webui.py`.

**BitBrowser Helper Script:**
- Location: `scripts/bitbrowser_connect.py`
- Triggers: Manual CLI use and `tests/test_bitbrowser_connect_script.py`
- Responsibilities: Query the local BitBrowser API and emit normalized JSON connection metadata.

## Error Handling

**Strategy:** Persist long-running task status, keep runtime logs append-only, and use transport-layer validation near the edges.

**Patterns:**
- Python APIs validate request models with Pydantic classes inside route files such as `src/web/routes/registration.py` and `src/web/routes/accounts.py`, then raise `HTTPException` for invalid inputs.
- Python persistence retries transient SQLite lock failures in `src/database/crud.py`, while `src/database/session.py` enables WAL and busy timeouts for concurrent access.
- Python background tasks convert execution failures into task status/log updates through `src/web/task_manager.py`, `src/web/routes/registration.py`, and `src/services/team/runner.py`.
- Go services return typed errors such as `ErrBatchNotFound`, `ErrQueueNotConfigured`, and config parse errors from `backend-go/internal/registration/batch_service.go`, `backend-go/internal/jobs/service.go`, and `backend-go/internal/config/config.go`.
- Go handlers keep request decoding thin and map service failures inside `backend-go/internal/registration/http/handlers.go` and `backend-go/internal/accounts/http/handlers.go`.

## Cross-Cutting Concerns

**Logging:** Python configures file logging plus database-backed app logs through `src/core/utils.py` and `src/core/db_logs.py`; task/batch runtime logs are also buffered in `src/web/task_manager.py`. Go uses the standard library `log` package at process boundaries and persists job logs through `backend-go/internal/jobs/repository_runtime.go`.

**Validation:** Python uses request/response models in route modules such as `src/web/routes/registration.py`, `src/web/routes/accounts.py`, and `src/web/routes/team.py`. Go validates env and request shape in `backend-go/internal/config/config.go`, `backend-go/internal/accounts/http/handlers.go`, and `backend-go/internal/registration/http/handlers.go`.

**Authentication:** Python HTML pages use cookie-based gatekeeping in `src/web/app.py` with HMAC tokens derived from `webui_secret_key` and `webui_access_password` in `src/config/settings.py`. A comparable auth layer is not detected in the Go router at `backend-go/internal/http/router.go`.

---

*Architecture analysis: 2026-04-05*
