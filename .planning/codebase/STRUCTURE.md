# Codebase Structure

**Analysis Date:** 2026-04-05

## Directory Layout

```text
[project-root]/
├── src/                    # Python application code: config, web, core logic, services, database
├── templates/              # Jinja2 HTML templates rendered by `src/web/app.py`
├── static/                 # Browser assets loaded by templates (`static/js`, `static/css`)
├── scripts/                # Operational helper scripts and container bootstrap
├── tests/                  # Python unit/integration tests and frontend node tests
├── backend-go/             # Go API/worker code, migrations, sqlc config, and Go tests
├── docs/superpowers/       # Design notes and implementation plans
├── data/                   # Runtime database/data directory created by `webui.py`
├── logs/                   # Runtime log directory created by `webui.py`
├── tests_runtime/          # Runtime SQLite fixtures created by tests
├── webui.py                # Python entrypoint
├── pyproject.toml          # Python package metadata and test config
├── package.json            # Minimal JS dependency manifest for frontend helpers/tests
├── Dockerfile              # Container image build
├── docker-compose.yml      # Local container orchestration
└── codex_register.spec     # PyInstaller packaging spec
```

## Directory Purposes

**`src/config/`:**
- Purpose: Centralize Python-side settings, constants, and startup notice text.
- Contains: `src/config/settings.py`, `src/config/constants.py`, `src/config/project_notice.py`
- Key files: `src/config/settings.py`, `src/config/constants.py`

**`src/web/`:**
- Purpose: Host the Python Web UI entrypoint, route handlers, WebSocket endpoints, and process-local task coordination.
- Contains: `src/web/app.py`, `src/web/task_manager.py`, feature routes in `src/web/routes/`
- Key files: `src/web/app.py`, `src/web/task_manager.py`, `src/web/routes/__init__.py`, `src/web/routes/registration.py`, `src/web/routes/accounts.py`, `src/web/routes/team.py`, `src/web/routes/websocket.py`

**`src/core/`:**
- Purpose: Hold Python registration, OpenAI integration, proxy, payment, and upload execution logic.
- Contains: `src/core/register.py`, `src/core/openai/*.py`, `src/core/anyauto/*.py`, `src/core/upload/*.py`
- Key files: `src/core/register.py`, `src/core/openai/token_refresh.py`, `src/core/openai/payment.py`, `src/core/upload/cpa_upload.py`

**`src/services/`:**
- Purpose: Implement provider adapters and service-style feature logic.
- Contains: Email provider adapters in `src/services/*.py`, Outlook-specific providers in `src/services/outlook/`, and team automation in `src/services/team/`
- Key files: `src/services/base.py`, `src/services/outlook/service.py`, `src/services/team/runner.py`, `src/services/team/discovery.py`, `src/services/team/sync.py`

**`src/database/`:**
- Purpose: Define Python ORM models, SQLAlchemy sessions, CRUD helpers, and startup migrations/repairs.
- Contains: Session bootstrap, shared models, team models, CRUD helpers
- Key files: `src/database/session.py`, `src/database/models.py`, `src/database/team_models.py`, `src/database/crud.py`, `src/database/team_crud.py`

**`templates/`:**
- Purpose: Store server-rendered page templates and shared fragments.
- Contains: One template per top-level page plus shared partials
- Key files: `templates/index.html`, `templates/accounts.html`, `templates/accounts_overview.html`, `templates/auto_team.html`, `templates/settings.html`, `templates/partials/site_notice.html`

**`static/`:**
- Purpose: Store browser-side CSS and page-scoped JavaScript consumed by templates.
- Contains: Shared CSS in `static/css/style.css`, page scripts in `static/js/*.js`, reusable browser helpers in `static/js/utils.js`, `static/js/registration_log_buffer.js`, `static/js/outlook_account_selector.js`
- Key files: `static/css/style.css`, `static/js/app.js`, `static/js/accounts.js`, `static/js/auto_team.js`, `static/js/utils.js`

**`scripts/`:**
- Purpose: Keep local operational scripts outside the app runtime modules.
- Contains: BitBrowser utility CLI, Docker startup script, Windows test-bundle helper
- Key files: `scripts/bitbrowser_connect.py`, `scripts/docker/start-webui.sh`, `scripts/make_windows_test_bundle.ps1`

**`tests/`:**
- Purpose: Cover Python server logic, data access, service adapters, route behavior, and browser helper modules.
- Contains: Pytest files at `tests/test_*.py` and Node-based frontend tests in `tests/frontend/*.test.mjs`
- Key files: `tests/test_registration_routes.py`, `tests/test_registration_engine.py`, `tests/test_team_routes.py`, `tests/test_account_crud.py`, `tests/frontend/registration_log_buffer.test.mjs`

**`backend-go/cmd/`:**
- Purpose: Provide executable entrypoints for the Go API and worker processes.
- Contains: `backend-go/cmd/api/main.go`, `backend-go/cmd/worker/main.go`
- Key files: `backend-go/cmd/api/main.go`, `backend-go/cmd/worker/main.go`

**`backend-go/internal/`:**
- Purpose: Hold Go domain packages, HTTP adapters, infrastructure adapters, and worker/runtime integrations.
- Contains: `accounts`, `jobs`, `registration`, `nativerunner`, `uploader`, `platform`, and package-local HTTP handlers
- Key files: `backend-go/internal/http/router.go`, `backend-go/internal/jobs/service.go`, `backend-go/internal/registration/executor.go`, `backend-go/internal/nativerunner/default_runner.go`, `backend-go/internal/uploader/builder.go`

**`backend-go/db/`:**
- Purpose: Own PostgreSQL migrations and sqlc query definitions used by Go repositories.
- Contains: SQL migrations in `backend-go/db/migrations/*.sql`, query files in `backend-go/db/query/*.sql`
- Key files: `backend-go/db/migrations/0001_init_jobs.sql`, `backend-go/db/migrations/0002_init_accounts_registration.sql`, `backend-go/db/migrations/0003_extend_registration_service_configs.sql`, `backend-go/db/query/jobs.sql`

**`backend-go/tests/`:**
- Purpose: Hold Go end-to-end compatibility tests that exercise the chi router and exposed HTTP contracts.
- Contains: `backend-go/tests/e2e/*.go`
- Key files: `backend-go/tests/e2e/accounts_flow_test.go`, `backend-go/tests/e2e/healthz_test.go`, `backend-go/tests/e2e/jobs_flow_test.go`

**`docs/superpowers/`:**
- Purpose: Store design specs and plan artifacts that explain migration slices and feature work.
- Contains: `docs/superpowers/specs/*.md`, `docs/superpowers/plans/*.md`
- Key files: `docs/superpowers/specs/2026-04-04-go-architecture-migration-design.md`, `docs/superpowers/plans/2026-04-04-go-phase0-1-bootstrap.md`

## Key File Locations

**Entry Points:**
- `webui.py`: Python launcher that initializes runtime directories, logging, and the FastAPI app.
- `src/web/app.py`: FastAPI app factory and HTML route host.
- `backend-go/cmd/api/main.go`: Go HTTP API bootstrap.
- `backend-go/cmd/worker/main.go`: Go Asynq worker bootstrap.
- `scripts/docker/start-webui.sh`: Container startup wrapper around `webui.py`.

**Configuration:**
- `pyproject.toml`: Python package metadata, console script, optional deps, and pytest config.
- `src/config/settings.py`: Python runtime settings model and database-backed defaults.
- `backend-go/internal/config/config.go`: Go environment loader and validation rules.
- `backend-go/sqlc.yaml`: Go sqlc generation config.
- `backend-go/Makefile`: Go test/run/migration task entrypoints.
- `docker-compose.yml`: Local multi-service container wiring.

**Core Logic:**
- `src/core/register.py`: Python registration engine.
- `src/services/base.py`: Python email-service factory and interface.
- `src/services/team/runner.py`: Python team task scheduler/executor.
- `backend-go/internal/registration/`: Go registration orchestration, execution, HTTP, and websocket packages.
- `backend-go/internal/jobs/`: Go job queue abstraction, repository, HTTP handlers, and worker loop.
- `backend-go/internal/accounts/`: Go account list/upsert service and PostgreSQL repository.

**Testing:**
- `tests/test_*.py`: Python pytest modules, organized by feature and layer.
- `tests/frontend/*.test.mjs`: Node tests for browser helper modules under `static/js/`.
- `backend-go/internal/**/*_test.go`: Go unit tests colocated with the package under test.
- `backend-go/tests/e2e/*.go`: Go compatibility/e2e tests for public HTTP behavior.

## Naming Conventions

**Files:**
- Python modules use `snake_case.py` and are usually named after the feature or adapter they implement: `src/web/routes/team_tasks.py`, `src/services/duck_mail.py`, `src/core/openai/token_refresh.py`.
- Templates mirror page names in lowercase snake case: `templates/accounts_overview.html`, `templates/email_services.html`.
- Frontend scripts mirror the page or component they drive: `static/js/app.js` for `templates/index.html`, `static/js/accounts.js` for `templates/accounts.html`, `static/js/auto_team.js` for `templates/auto_team.html`.
- Go source follows package and feature naming in lowercase with underscores where needed: `backend-go/internal/registration/python_runner.go`, `backend-go/internal/accounts/repository_postgres.go`.
- Tests use `test_*.py`, `*.test.mjs`, and `*_test.go`.

**Directories:**
- Python server code is layered under `src/` by responsibility: `config`, `web`, `core`, `database`, `services`.
- Go code is split by executable in `backend-go/cmd/` and by domain/infrastructure package in `backend-go/internal/`.
- SQL assets stay under `backend-go/db/`, generated Go query code stays under `backend-go/internal/jobs/sqlc/`.

## Where to Add New Code

**New Python Web Feature:**
- Primary code: Add or extend a feature router in `src/web/routes/<feature>.py`.
- HTML page: Add a template in `templates/<feature>.html` and, if it is a top-level page, add the HTML route in `src/web/app.py`.
- Browser logic: Add `static/js/<feature>.js` and reference it from the matching template.
- Tests: Add pytest coverage in `tests/test_<feature>_routes.py` or another relevant `tests/test_*.py`; add `tests/frontend/<module>.test.mjs` if the logic is browser-only.

**New Python Domain/Integration Module:**
- Registration/OpenAI behavior: Extend `src/core/register.py` or add helpers under `src/core/openai/` / `src/core/anyauto/`.
- Email provider: Add the adapter in `src/services/<provider>.py` or `src/services/outlook/providers/<provider>.py`, then register it through `src/services/base.py`.
- Team automation: Add service logic in `src/services/team/<feature>.py` and wire it through `src/web/routes/team.py` or `src/web/routes/team_tasks.py`.
- Persistence changes: Update models in `src/database/models.py` or `src/database/team_models.py`, then extend CRUD in `src/database/crud.py` or `src/database/team_crud.py`.

**New Go API/Worker Feature:**
- Domain package: Create or extend `backend-go/internal/<domain>/`.
- HTTP adapter: Put request decoding and route registration in `backend-go/internal/<domain>/http/handlers.go`.
- Router mount: Register the handler in `backend-go/internal/http/router.go`.
- Bootstrap wiring: Instantiate the service in `backend-go/cmd/api/main.go` or `backend-go/cmd/worker/main.go`.
- Tests: Add unit tests next to the package as `*_test.go` and add compatibility coverage in `backend-go/tests/e2e/` when the HTTP contract changes.

**New Go Database / Queue Behavior:**
- Schema changes: Add a numbered migration under `backend-go/db/migrations/`.
- New sqlc query: Put SQL under `backend-go/db/query/` and regenerate `backend-go/internal/jobs/sqlc/`.
- Repository logic: Keep PostgreSQL-specific code in `backend-go/internal/<domain>/repository_postgres.go`.
- Queue and worker concerns: Put job orchestration in `backend-go/internal/jobs/` and execution/runtime logic in `backend-go/internal/registration/` or `backend-go/internal/nativerunner/`.

**Utilities:**
- Shared Python helpers: `src/core/utils.py`, `src/core/http_client.py`, or a narrowly scoped helper module inside the owning package.
- Shared browser helpers: `static/js/utils.js`, `static/js/registration_log_buffer.js`, or another reusable module under `static/js/`.
- Shared Go infra helpers: `backend-go/internal/platform/` for connection/bootstrap concerns, or a small helper inside the owning internal package.

## Special Directories

**`backend-go/internal/jobs/sqlc/`:**
- Purpose: Generated Go query code for the jobs schema.
- Generated: Yes
- Committed: Yes

**`data/`:**
- Purpose: Python runtime data directory; default SQLite database location when no external database URL is supplied.
- Generated: Yes
- Committed: No

**`logs/`:**
- Purpose: Python runtime file-log directory used by `webui.py` and `src/core/utils.py`.
- Generated: Yes
- Committed: No

**`tests_runtime/`:**
- Purpose: Temporary SQLite files created by tests such as `tests/test_registration_routes.py`.
- Generated: Yes
- Committed: No

**`.planning/codebase/`:**
- Purpose: Generated repository-mapping documents for later planning/execution phases.
- Generated: Yes
- Committed: No

**`docs/superpowers/`:**
- Purpose: Human-authored specs and plans that describe migration slices and feature intent.
- Generated: No
- Committed: Yes

---

*Structure analysis: 2026-04-05*
