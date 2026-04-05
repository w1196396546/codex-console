# Phase 1: Compatibility Baseline - Research

**Researched:** 2026-04-05
**Domain:** Brownfield Python-to-Go compatibility baseline for route surface, client dependencies, shared persistence, and runtime ownership. [VERIFIED: .planning/phases/01-compatibility-baseline/01-CONTEXT.md, .planning/ROADMAP.md, src/web/routes/__init__.py, backend-go/internal/http/router.go]
**Confidence:** MEDIUM

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Treat the current Python backend behavior as the compatibility reference for every not-yet-migrated domain.
- **D-02:** Route path, HTTP method, JSON field names, status values, websocket semantics, and critical side effects are all part of parity; none may drift silently.
- **D-03:** Count existing Go jobs, registration/task APIs, websocket streams, account-listing baseline, uploader persistence, and native runner foundations as already-migrated baseline.
- **D-04:** Phase 1 must only document and instrument the remaining migration delta; it must not re-plan or re-implement already existing Go foundations.
- **D-05:** Use current templates and `static/js` clients as first-class compatibility consumers when building the parity matrix.
- **D-06:** Capture shared schema/runtime rules before moving more domains so later phases can migrate against an explicit contract instead of ad hoc behavior.

### Claude's Discretion
The agent may choose the exact artifact format for parity matrices, compatibility checklists, and regression fixtures as long as they are reviewable, traceable, and directly usable by later planning/execution phases.

### Deferred Ideas (OUT OF SCOPE)
- Actual migration of management, payment, and team domains belongs to later phases.
- Security hardening beyond what is necessary to define migration-safe contracts remains outside this phase.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| COMP-01 | Existing clients can call every remaining Python-only backend capability through a Go-owned route using the same path and HTTP method. [VERIFIED: .planning/REQUIREMENTS.md] | Phase 1 needs a route-plus-client parity matrix covering Python route modules, mounted page scripts, and current Go coverage. [VERIFIED: src/web/routes/__init__.py, src/web/app.py, static/js/app.js, static/js/accounts.js, static/js/settings.js, static/js/payment.js, static/js/auto_team.js, backend-go/internal/http/router.go] |
| COMP-02 | Existing clients receive compatible JSON field names, status values, and error semantics from migrated Go endpoints. [VERIFIED: .planning/REQUIREMENTS.md] | Phase 1 needs DTO/status fixtures taken from Python handlers, task/websocket payloads, and existing Go compat tests before later domain migration starts. [VERIFIED: src/web/routes/registration.py, src/web/routes/accounts.py, src/web/routes/team_tasks.py, src/web/routes/websocket.py, backend-go/internal/registration/http/handlers.go, backend-go/tests/e2e/jobs_flow_test.go] |
| DATA-01 | Existing persisted records for accounts, settings, upload services, proxies, bind-card tasks, app logs, and team entities remain readable and writable after Go takeover without manual reshaping. [VERIFIED: .planning/REQUIREMENTS.md] | Phase 1 needs a shared schema contract that compares Python ORM tables and columns against Go migrations and repositories, including Python-only tables that have no Go schema yet. [VERIFIED: src/database/models.py, src/database/team_models.py, backend-go/db/migrations/0001_init_jobs.sql, backend-go/db/migrations/0002_init_accounts_registration.sql, backend-go/db/migrations/0003_extend_registration_service_configs.sql] |
| OPS-01 | Go backend operational controls are sufficient to replace Python backend duties safely in production for all migrated domains. [VERIFIED: .planning/REQUIREMENTS.md] | Phase 1 needs migration safety rails: auth/runtime boundary notes, migration verification commands, environment prerequisites, and compatibility fixtures that later phases can reuse. [VERIFIED: src/web/app.py, backend-go/Makefile, backend-go/tests/e2e/*.go, backend-go/db/migrations/migrations_test.go, backend-go/docs/phase1-runbook.md] |
</phase_requirements>

## Summary

The current cutover blocker is not “missing one more Go handler”; it is missing contract documentation around a much larger Python surface. Python currently exposes 162 HTTP routes across registration, accounts, settings, email services, payment, logs, team, and upload-config domains, plus 2 WebSocket channels, while the audited Go surface covers `/api/accounts`, registration compatibility endpoints, job controls, and the same 2 WebSocket channels. [VERIFIED: src/web/routes/accounts.py, src/web/routes/registration.py, src/web/routes/settings.py, src/web/routes/email.py, src/web/routes/payment.py, src/web/routes/logs.py, src/web/routes/team.py, src/web/routes/team_tasks.py, src/web/routes/upload/cpa_services.py, src/web/routes/upload/sub2api_services.py, src/web/routes/upload/tm_services.py, src/web/routes/websocket.py, backend-go/internal/http/router.go, backend-go/internal/accounts/http/handlers.go, backend-go/internal/jobs/http/handlers.go, backend-go/internal/registration/http/handlers.go]

The highest data risk is not in the already-shared `accounts` table; it is in the mixed runtime around what is still Python-only. Python ORM metadata defines 14 tables, Go migrations define 9 tables, only 6 table names overlap, and Python still owns `registration_tasks`, `bind_card_tasks`, `app_logs`, `proxies`, and all `team_*` tables. [VERIFIED: src/database/models.py, src/database/team_models.py, backend-go/db/migrations/0001_init_jobs.sql, backend-go/db/migrations/0002_init_accounts_registration.sql, backend-go/db/migrations/0003_extend_registration_service_configs.sql] Even inside shared tables there are compatibility gaps, such as Python `settings.description/category/updated_at` and `email_services.last_used`, which are not present in the current Go migrations. [VERIFIED: src/database/models.py, src/config/settings.py, backend-go/db/migrations/0002_init_accounts_registration.sql, backend-go/db/migrations/0003_extend_registration_service_configs.sql]

The main runtime risk is semantic drift between Python’s in-process task control and Go’s queue-backed job model. Python pause/resume/cancel/log replay semantics still live in `src/web/task_manager.py` and `src/web/routes/websocket.py`, while Go derives equivalent task and batch state from jobs plus `BatchService` projections; Phase 1 needs those semantics frozen as fixtures before Phase 2 or later management-domain work can be planned safely. [VERIFIED: src/web/task_manager.py, src/web/routes/websocket.py, src/web/routes/registration.py, backend-go/internal/registration/batch_service.go, backend-go/internal/registration/http/handlers.go, backend-go/tests/e2e/jobs_flow_test.go]

**Primary recommendation:** Make Phase 1 produce three hard artifacts before any new cutover plan: a route/client parity matrix, a shared schema/runtime contract manifest, and an executable compatibility harness that reuses existing pytest, frontend, migration, and Go e2e fixtures. [VERIFIED: .planning/ROADMAP.md, tests/test_registration_routes.py, tests/test_accounts_routes.py, tests/test_settings_routes.py, tests/test_payment_routes.py, tests/test_team_routes.py, tests/frontend/registration_log_buffer.test.mjs, backend-go/tests/e2e/jobs_flow_test.go, backend-go/db/migrations/migrations_test.go]

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Python reference surface | `Python >=3.10`, `fastapi>=0.100.0`, `jinja2>=3.1.0`, `sqlalchemy>=2.0.0` [VERIFIED: pyproject.toml] | Keep Python route, template, and ORM behavior as the compatibility oracle for non-migrated domains. [VERIFIED: src/web/app.py, src/web/routes/__init__.py, src/database/models.py] | Current templates and `static/js` files already depend on this behavior, so later plans should measure parity against it instead of redesigning it. [VERIFIED: templates/index.html, templates/accounts.html, templates/settings.html, templates/payment.html, templates/auto_team.html, static/js/app.js, static/js/accounts.js, static/js/settings.js, static/js/payment.js, static/js/auto_team.js] |
| Go cutover baseline | `go 1.25.0`, `chi v5.2.3`, `asynq v0.26.0`, `pgx/v5 v5.9.1`, `go-redis/v9 v9.18.0` [VERIFIED: backend-go/go.mod] | Extend the existing Go API, worker, queue, and PostgreSQL/Redis baseline instead of re-planning it. [VERIFIED: backend-go/cmd/api/main.go, backend-go/cmd/worker/main.go, backend-go/internal/http/router.go] | The roadmap explicitly treats existing Go jobs, registration/task APIs, websocket streams, account-listing baseline, uploader persistence, and native runner foundations as baseline. [VERIFIED: .planning/phases/01-compatibility-baseline/01-CONTEXT.md] |
| Shared contract layer | Python ORM + Go migrations/repositories on top of PostgreSQL-compatible shapes. [VERIFIED: src/database/session.py, src/database/models.py, src/database/team_models.py, backend-go/db/migrations/*.sql, backend-go/internal/accounts/repository_postgres.go] | Phase 1 needs one authoritative table/column/status manifest that later plans can target. [VERIFIED: src/database/models.py, src/database/team_models.py, backend-go/db/migrations/*.sql] | Shared table names already hide drift, so explicit contract work is cheaper than later rollback work. [VERIFIED: src/database/models.py, backend-go/db/migrations/0002_init_accounts_registration.sql, backend-go/db/migrations/0003_extend_registration_service_configs.sql] |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| Python/Node compatibility tests | `pytest>=7.0.0`, `httpx>=0.24.0`, local `node v20.19.4`, local `npm 10.8.2` [VERIFIED: pyproject.toml, local env] | Reuse existing Python route tests and frontend helper tests as compatibility fixtures. [VERIFIED: tests/test_registration_routes.py, tests/test_accounts_routes.py, tests/test_settings_routes.py, tests/test_payment_routes.py, tests/test_team_routes.py, tests/frontend/*.test.mjs] | Use whenever a Phase 1 artifact claims current client or route semantics. [VERIFIED: same] |
| Go verification harness | `go test`, migration tests, Go e2e tests under `backend-go/tests/e2e` [VERIFIED: backend-go/Makefile, backend-go/tests/e2e/*.go, backend-go/db/migrations/migrations_test.go] | Reuse current Go-side compatibility coverage rather than inventing a separate proof path. [VERIFIED: same] | Use for any Phase 1 contract that later phases expect Go to satisfy. [VERIFIED: backend-go/tests/e2e/accounts_flow_test.go, backend-go/tests/e2e/jobs_flow_test.go] |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Backend-first redesign | Python behavior as the explicit contract source | Redesign work would hide the exact parity gaps this phase is supposed to expose. [VERIFIED: .planning/ROADMAP.md, .planning/phases/01-compatibility-baseline/01-CONTEXT.md] |
| Implicit “shared schema” assumptions | A versioned table/column/runtime manifest | The manifest costs a little more Phase 1 effort but prevents silent SQLite/PostgreSQL drift later. [VERIFIED: src/database/session.py, backend-go/internal/config/config.go, src/database/models.py, backend-go/db/migrations/*.sql] |

**Installation:**
```bash
# No new packages are recommended for Phase 1.
# Reuse the dependencies already pinned in pyproject.toml, package.json, and backend-go/go.mod.
```

No net-new dependency is required to plan this phase; the blocking issue is environment parity and contract coverage, not library selection. [VERIFIED: pyproject.toml, package.json, backend-go/go.mod]

**Version verification:**
- Python dependency minima come from `pyproject.toml`, and Go runtime/library versions come from `backend-go/go.mod`. [VERIFIED: pyproject.toml, backend-go/go.mod]
- The local machine currently has `go1.24.3`, which is lower than the repo-declared `go 1.25.0`; planner should not treat live Go execution as a guaranteed gate until that mismatch is addressed. [VERIFIED: backend-go/go.mod, local env]

## Architecture Patterns

### Recommended Project Structure
```text
.planning/phases/01-compatibility-baseline/
├── 01-RESEARCH.md
├── route-parity.md          # Python route -> Go owner -> client consumer
├── client-dependencies.md   # template/page -> static/js -> API/WebSocket usage
├── schema-contract.md       # shared tables, Python-only tables, field/status notes
├── runtime-semantics.md     # task, batch, websocket, auth, and rollout boundaries
└── verification-checklist.md# commands, fixtures, and cutover smoke steps
```

### Pattern 1: Route + Client Parity Matrix
**What:** Build one matrix from four anchors: Python route mounts, Go route mounts, template-to-script links, and `static/js` API/WebSocket calls. [VERIFIED: src/web/routes/__init__.py, src/web/app.py, backend-go/internal/http/router.go, templates/*.html, static/js/*.js]
**When to use:** Use this first for every later domain plan, because the browser clients are already bound to exact paths and methods. [VERIFIED: static/js/app.js, static/js/accounts.js, static/js/settings.js, static/js/payment.js, static/js/auto_team.js]
**Example:**
```python
# Source: src/web/routes/__init__.py
api_router.include_router(accounts_router, prefix="/accounts", tags=["accounts"])
api_router.include_router(registration_router, prefix="/registration", tags=["registration"])
api_router.include_router(settings_router, prefix="/settings", tags=["settings"])
api_router.include_router(payment_router, prefix="/payment", tags=["payment"])
api_router.include_router(team_router, prefix="/team", tags=["team"])
```

```go
// Source: backend-go/internal/http/router.go
if accountsService != nil {
	accountshttp.NewHandler(accountsService).RegisterRoutes(r)
}
if registrationService != nil && jobService != nil {
	registrationhttp.NewHandler(...).RegisterRoutes(r)
}
r.Get("/api/ws/task/{task_uuid}", taskSocketHandler.HandleTaskSocket)
```

### Pattern 2: Shared Schema Contract Manifest
**What:** Separate shared tables from Python-only tables, then record column-level drift inside the shared tables. [VERIFIED: src/database/models.py, src/database/team_models.py, backend-go/db/migrations/*.sql]
**When to use:** Use this before any management, payment, or team cutover plan, because later work depends on whether data is already in PostgreSQL, still only in SQLite, or only exposed through Python ORM helpers. [VERIFIED: src/database/session.py, backend-go/internal/config/config.go]
**Example:**
```python
# Source: src/database/models.py
class Setting(Base):
    __tablename__ = "settings"
    key = Column(String(100), primary_key=True)
    value = Column(Text)
    description = Column(Text)
    category = Column(String(50), default="general")
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)
```

```sql
-- Source: backend-go/db/migrations/0002_init_accounts_registration.sql
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT
);
```

### Pattern 3: Runtime Semantics Ledger
**What:** Treat task state, batch state, log replay, pause/resume/cancel behavior, and auth boundary as first-class compatibility contracts. [VERIFIED: src/web/task_manager.py, src/web/routes/websocket.py, src/web/routes/registration.py, src/web/app.py, backend-go/internal/registration/batch_service.go]
**When to use:** Use this before planning Phase 2 and before any UI-facing cutover, because these behaviors are easy to drift even when paths and schemas match. [VERIFIED: .planning/ROADMAP.md, static/js/app.js, backend-go/tests/e2e/jobs_flow_test.go]
**Example:**
```python
# Source: src/web/routes/websocket.py
elif data.get("type") == "pause":
    ...
    await websocket.send_json({
        "type": "status",
        "task_uuid": task_uuid,
        "status": "paused",
    })
```

```go
// Source: backend-go/internal/registration/batch_service.go
return BatchStatusResponse{
	BatchID:       batchID,
	Status:        stats.status,
	LogOffset:     offset,
	LogNextOffset: len(stats.logs),
	Progress:      fmt.Sprintf("%d/%d", stats.completed, record.count),
}, nil
```

### Anti-Patterns to Avoid
- **Treating current Go registration coverage as “backend mostly done”:** Current Go compatibility matches registration plus one account-list endpoint and the two websocket channels, but settings, email services, payment, logs, team, upload config, and most account-management routes are still Python-only. [VERIFIED: src/web/routes/*.py, backend-go/internal/http/router.go, backend-go/internal/{accounts,jobs,registration}/http/handlers.go]
- **Using shared table names as proof of data parity:** `settings`, `email_services`, and `accounts` already show column and semantics drift, and 8 Python tables have no Go schema at all. [VERIFIED: src/database/models.py, src/database/team_models.py, backend-go/db/migrations/*.sql]
- **Leaving auth/runtime ownership implicit until later phases:** HTML route auth, `/payment` page exposure, and `/api` mount behavior are already inconsistent enough that later cutover planning can drift without a Phase 1 decision. [VERIFIED: src/web/app.py]

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Route inventory | A planner-only checklist typed from memory | A generated parity matrix from `src/web/routes`, `backend-go/internal`, `templates`, and `static/js` | The source of truth already exists in tracked files and can be regenerated when routes move. [VERIFIED: src/web/routes/__init__.py, backend-go/internal/http/router.go, templates/*.html, static/js/*.js] |
| Schema comparison | A prose-only “looks compatible” note | A table/column manifest backed by ORM models, SQL migrations, and repository code | Shared names currently hide missing tables and column drift. [VERIFIED: src/database/models.py, src/database/team_models.py, backend-go/db/migrations/*.sql, backend-go/internal/accounts/repository_postgres.go] |
| Runtime parity proof | Manual browser clicking and screenshots | Existing pytest, frontend tests, Go e2e tests, and migration tests as reusable fixtures | The repo already contains executable expectations for task logs, batch control, Outlook selection, team routes, and migration repairs. [VERIFIED: tests/test_registration_routes.py, tests/test_registration_task_binding.py, tests/test_team_routes.py, tests/frontend/*.test.mjs, backend-go/tests/e2e/jobs_flow_test.go, backend-go/db/migrations/migrations_test.go] |
| SQLite swap/import validation | One-off shell copy logic | The existing Python import/backup semantics plus an explicit Phase 1 checklist | The current settings route already encodes backup-before-import and WAL/SHM cleanup behavior that later plans must preserve or retire explicitly. [VERIFIED: src/web/routes/settings.py, src/database/session.py] |

**Key insight:** The expensive bugs in this phase are contract bugs, not algorithm bugs; Phase 1 should harvest the behavior already encoded in routes, scripts, models, and tests instead of inventing a new abstraction layer first. [VERIFIED: src/web/routes/*.py, static/js/*.js, src/database/*.py, tests/*.py, backend-go/tests/e2e/*.go]

## Runtime State Inventory

| Category | Items Found | Action Required |
|----------|-------------|-----------------|
| Stored data | Python ORM defines 14 tables, Go migrations define 9 tables, and only 6 table names overlap: `accounts`, `email_services`, `settings`, `cpa_services`, `sub2api_services`, `tm_services`. [VERIFIED: src/database/models.py, src/database/team_models.py, backend-go/db/migrations/*.sql] Python-only persisted domains are `registration_tasks`, `bind_card_tasks`, `app_logs`, `proxies`, `teams`, `team_memberships`, `team_tasks`, and `team_task_items`. [VERIFIED: src/database/models.py, src/database/team_models.py, backend-go/db/migrations/*.sql] | Phase 1 should publish a table-by-table ownership matrix and decide whether each Python-only table is migrated, shadow-written, or kept Python-owned until a later phase. [VERIFIED: .planning/ROADMAP.md, src/database/models.py, src/database/team_models.py, backend-go/db/migrations/*.sql] |
| Live service config | Runtime configuration already lives inside database-backed tables and settings keys: `email_services`, `settings`, `cpa_services`, `sub2api_services`, `tm_services`, and `proxies`. [VERIFIED: src/database/models.py, src/config/settings.py, src/web/routes/settings.py, src/web/routes/email.py, src/web/routes/upload/cpa_services.py, src/web/routes/upload/sub2api_services.py, src/web/routes/upload/tm_services.py] No additional out-of-git admin UI config was discoverable from tracked repo files alone. [ASSUMED] | Freeze config key names, masking rules, and table ownership before later cutovers, and ask for deployment-specific confirmation on any external control planes not represented in git. [VERIFIED: src/config/settings.py, src/web/routes/email.py, src/web/routes/upload/*.py] |
| OS-registered state | Tracked entrypoints still center on Python runtime launchers such as `webui.py`, `scripts/docker/start-webui.sh`, and `codex_register.spec`; no systemd, launchd, or pm2 manifests were found in tracked files. [VERIFIED: webui.py, scripts/docker/start-webui.sh, codex_register.spec, rg repo audit] Production host registrations were not inspected in this session. [ASSUMED] | Phase 1 should require a deployment inventory before Phase 5 planning so cutover does not miss process-manager, service, or packaging registrations outside git. [VERIFIED: .planning/ROADMAP.md] |
| Secrets/env vars | Python can default to local SQLite and consumes `APP_DATABASE_URL`/`DATABASE_URL` plus `APP_*` / `WEBUI_*` overrides, while Go hard-requires `DATABASE_URL` and `REDIS_ADDR`. [VERIFIED: src/database/session.py, webui.py, backend-go/internal/config/config.go] Cookie auth is generated in Python with HMAC over `webui_access_password`, and `/api` routers do not add a shared auth dependency at mount time. [VERIFIED: src/web/app.py] | Phase 1 should publish a dual-runtime env matrix and make an explicit auth boundary decision before later domain cutover plans assume either “open API” or “all admin APIs authenticated.” [VERIFIED: src/web/app.py, backend-go/internal/config/config.go] |
| Build artifacts | Generated runtime directories already exist for `data/`, `logs/`, and `tests_runtime/`, and Go keeps generated sqlc code in `backend-go/internal/jobs/sqlc/`. [VERIFIED: .planning/codebase/STRUCTURE.md, backend-go/internal/jobs/sqlc] No system-installed packages or external build artifacts were inspected in this session. [ASSUMED] | Phase 1 compatibility harness should isolate runtime data directories during tests and treat generated sqlc output as a derived artifact that must be regenerated when schema/query contracts change. [VERIFIED: .planning/codebase/STRUCTURE.md, backend-go/Makefile, backend-go/sqlc.yaml] |

## Common Pitfalls

### Pitfall 1: Assuming Registration Parity Means Backend Parity
**What goes wrong:** Later plans start from the current Go registration surface and underestimate how much of the UI still calls Python-only settings, payment, team, upload-config, logs, and detailed account-management routes. [VERIFIED: static/js/accounts.js, static/js/settings.js, static/js/payment.js, static/js/auto_team.js, src/web/routes/*.py, backend-go/internal/http/router.go]
**Why it happens:** Registration endpoints and websocket channels already exist in Go, so the repo feels closer to cutover than it is. [VERIFIED: backend-go/internal/registration/http/handlers.go, backend-go/internal/http/router.go]
**How to avoid:** Phase 1 should publish route families by page/script consumer, not only by backend package. [VERIFIED: templates/*.html, static/js/*.js]
**Warning signs:** A plan says “management later” without first naming the exact `/api/settings/*`, `/api/email-services/*`, `/api/payment/*`, `/api/team/*`, and upload-config calls that current pages already make. [VERIFIED: static/js/settings.js, static/js/email_services.js, static/js/payment.js, static/js/auto_team.js, static/js/accounts.js]

### Pitfall 2: Treating Shared Table Names as Full Data Compatibility
**What goes wrong:** Planner assumes that because `accounts`, `settings`, and `email_services` exist on both sides, later cutover can write directly without a contract pass. [VERIFIED: src/database/models.py, backend-go/db/migrations/0002_init_accounts_registration.sql, backend-go/db/migrations/0003_extend_registration_service_configs.sql]
**Why it happens:** The overlap hides missing columns like `settings.description/category/updated_at` and `email_services.last_used`, plus Python-only tables that still hold live runtime state. [VERIFIED: src/database/models.py, src/config/settings.py, backend-go/db/migrations/0002_init_accounts_registration.sql]
**How to avoid:** Freeze a table/column/status manifest and mark each field as shared, Python-only, or transitional. [VERIFIED: src/database/models.py, src/database/team_models.py, backend-go/db/migrations/*.sql]
**Warning signs:** A plan mentions “schema already migrated” without naming `registration_tasks`, `bind_card_tasks`, `app_logs`, `proxies`, or `team_*` tables. [VERIFIED: src/database/models.py, src/database/team_models.py, backend-go/db/migrations/*.sql]

### Pitfall 3: Re-Implementing Task Semantics from Memory
**What goes wrong:** Pause/resume/cancel behavior, incremental log offsets, websocket ping/pong, and batch progress semantics drift even when the path names stay the same. [VERIFIED: src/web/routes/websocket.py, src/web/routes/registration.py, src/web/task_manager.py, backend-go/internal/registration/batch_service.go, backend-go/tests/e2e/jobs_flow_test.go]
**Why it happens:** Python task control is partly in durable tables and partly in process memory, while Go derives state from jobs plus `BatchService`; the shapes look similar but are not sourced the same way. [VERIFIED: src/web/task_manager.py, src/database/models.py, backend-go/internal/jobs/service.go, backend-go/internal/registration/batch_service.go]
**How to avoid:** Promote the current pytest and Go e2e task semantics into Phase 1 compatibility fixtures before Phase 2 starts. [VERIFIED: tests/test_registration_routes.py, tests/test_registration_task_binding.py, backend-go/tests/e2e/jobs_flow_test.go]
**Warning signs:** A plan describes “equivalent websocket behavior” without citing `log_offset`, `log_next_offset`, pause/cancel messages, or the current state transitions. [VERIFIED: src/web/routes/registration.py, src/web/routes/websocket.py, backend-go/internal/registration/batch_service.go, backend-go/internal/registration/stats.go]

### Pitfall 4: Leaving Auth Boundary Decisions for Later
**What goes wrong:** Later cutover phases accidentally widen or tighten access in inconsistent ways because current behavior is already mixed. [VERIFIED: src/web/app.py]
**Why it happens:** Most HTML pages check `_is_authenticated`, `/payment` does not, and `/api` routers are mounted without a shared dependency or middleware auth gate. [VERIFIED: src/web/app.py]
**How to avoid:** Phase 1 should explicitly document current exposure and decide whether parity means preserving it temporarily or normalizing it before cutover work continues. [VERIFIED: src/web/app.py, .planning/ROADMAP.md]
**Warning signs:** Planner treats auth as “hardening later” while Phase 3 or Phase 4 moves privileged APIs such as settings import, account export, payment tasks, or service-config endpoints. [VERIFIED: src/web/routes/settings.py, src/web/routes/accounts.py, src/web/routes/payment.py, src/web/routes/email.py, src/web/routes/upload/*.py]

## Code Examples

Verified patterns from the current codebase:

### Python Route Composition Is the Compatibility Source
```python
# Source: src/web/routes/__init__.py
api_router.include_router(accounts_router, prefix="/accounts", tags=["accounts"])
api_router.include_router(registration_router, prefix="/registration", tags=["registration"])
api_router.include_router(settings_router, prefix="/settings", tags=["settings"])
api_router.include_router(email_services_router, prefix="/email-services", tags=["email-services"])
api_router.include_router(payment_router, prefix="/payment", tags=["payment"])
```

### Existing Frontend Code Calls the Compatibility Surface Directly
```javascript
// Source: static/js/settings.js
const data = await api.get('/settings');
await api.post('/settings/webui', payload);
const data = await api.get('/email-services');
const data = await api.get('/settings/proxies');
```

```javascript
// Source: static/js/payment.js
const data = await api.post("/payment/bind-card/tasks", payload);
const data = await api.get(`/payment/bind-card/tasks?${params.toString()}`);
await api.delete(`/payment/bind-card/tasks/${taskId}`);
```

### Shared Table Names Still Need Column-Level Contract Checks
```python
# Source: src/database/models.py
class EmailService(Base):
    __tablename__ = 'email_services'
    config = Column(JSONEncodedDict, nullable=False)
    enabled = Column(Boolean, default=True)
    priority = Column(Integer, default=0)
    last_used = Column(DateTime)
```

```sql
-- Source: backend-go/db/migrations/0002_init_accounts_registration.sql
CREATE TABLE IF NOT EXISTS email_services (
    id SERIAL PRIMARY KEY,
    service_type TEXT NOT NULL,
    name TEXT NOT NULL,
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    priority INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Python process-local task control in `task_manager` plus SQLite-backed `registration_tasks`. [VERIFIED: src/web/task_manager.py, src/database/models.py] | Go queue-backed jobs and `BatchService` provide the emerging cutover model for registration state. [VERIFIED: backend-go/internal/jobs/service.go, backend-go/internal/registration/batch_service.go] | Current mixed-runtime repo state. [VERIFIED: .planning/codebase/ARCHITECTURE.md] | Phase 1 must freeze semantics before Phase 2 replaces Python critical-path execution. [VERIFIED: .planning/ROADMAP.md] |
| Python defaults to local SQLite when no database URL is set. [VERIFIED: src/database/session.py] | Go refuses to start without `DATABASE_URL` and `REDIS_ADDR`. [VERIFIED: backend-go/internal/config/config.go] | Current mixed-runtime repo state. [VERIFIED: .planning/codebase/STACK.md, backend-go/internal/config/config.go] | Planner must decide whether later cutover supports SQLite users or requires an explicit PostgreSQL migration step. [VERIFIED: src/database/session.py, backend-go/internal/config/config.go] |
| Verification documentation still describes Phase 1 E2E as a minimal healthz check. [VERIFIED: backend-go/docs/phase1-runbook.md] | The repo already contains Go E2E coverage for accounts, jobs, registration compatibility, batch flows, websocket flows, and stats. [VERIFIED: backend-go/tests/e2e/accounts_flow_test.go, backend-go/tests/e2e/jobs_flow_test.go] | Documentation drift exists right now. [VERIFIED: backend-go/docs/phase1-runbook.md, backend-go/tests/e2e/*.go] | Phase 1 should align the safety-check artifact list with the actual executable harness, or planners may under-scope available verification. [VERIFIED: backend-go/docs/phase1-runbook.md, backend-go/tests/e2e/*.go] |

**Deprecated/outdated:**
- Python-only process memory as the long-term source of task truth is outdated for the target Go-owned backend, even though it is still the current compatibility reference. [VERIFIED: src/web/task_manager.py, .planning/ROADMAP.md]
- “SQLite by default” is outdated as a cutover assumption for Go-owned backend execution because the Go runtime requires PostgreSQL and Redis. [VERIFIED: src/database/session.py, backend-go/internal/config/config.go]

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | No additional external admin UI control plane beyond DB/env-backed settings was discoverable from tracked repo files alone. [ASSUMED] | Runtime State Inventory | A later plan could miss manual migration work for dashboards, reverse proxies, or service-side config stored outside git. |
| A2 | Production OS/service registrations were not inspected in this session. [ASSUMED] | Runtime State Inventory | Final cutover planning could miss launchd/systemd/pm2/package-installer steps that still point at Python. |
| A3 | The current environment does not provide easy local PostgreSQL/Redis bring-up because `psql`, `redis-cli`, and Docker are absent. [ASSUMED] | Environment Availability | A planner might over- or under-estimate what can be verified locally without first confirming alternate infrastructure access. |

## Open Questions (RESOLVED)

1. **Will later cutover support current SQLite-first installs, or is PostgreSQL a hard prerequisite once Go owns more domains?**
   What we know: Python defaults to `data/database.db` when no DB URL is set, but Go requires `DATABASE_URL` and `REDIS_ADDR`. [VERIFIED: src/database/session.py, backend-go/internal/config/config.go]
   RESOLVED: Once Go owns additional backend domains beyond the current baseline, PostgreSQL + Redis are hard prerequisites for the Go production path, and SQLite remains a legacy Python reference input only. Phase 1 should document the resulting storage-policy split and require an explicit SQLite-to-PostgreSQL migration step before Phase 5 cutover claims production readiness. [VERIFIED: .planning/PROJECT.md, .planning/ROADMAP.md, src/database/session.py, backend-go/internal/config/config.go]

2. **What is the intended auth boundary for `/api` and `/payment` during the migration window?**
   What we know: Most HTML pages check `_is_authenticated`, `payment_page` does not, and API routers mount without a shared auth dependency. [VERIFIED: src/web/app.py]
   RESOLVED: Phase 1 treats the current page/API exposure as the compatibility reference, not as an endorsement of the long-term security model. Later migration phases must preserve current behavior unless they explicitly scope and verify an auth-boundary change; no implicit hardening is allowed to slip into compatibility work. [VERIFIED: .planning/PROJECT.md, .planning/ROADMAP.md, src/web/app.py]

3. **Where are deployment-specific process registrations and environment injections managed today?**
   What we know: Tracked repo files show Python launch entrypoints and Docker startup helpers, but no tracked system service manifests. [VERIFIED: webui.py, scripts/docker/start-webui.sh, codex_register.spec, rg repo audit]
   RESOLVED: Phase 1 will use git-tracked launch paths as the verified deployment baseline and record any out-of-repo service-manager or env-injection details as a mandatory external inventory item for Phase 5. That means lack of host-level inspection is not a blocker for Phase 1 planning, but it is a hard prerequisite before final cutover. [VERIFIED: .planning/ROADMAP.md, webui.py, scripts/docker/start-webui.sh, codex_register.spec]

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Python runtime | Python route/tests and current compatibility oracle. [VERIFIED: pyproject.toml, tests/*.py] | ✓ [VERIFIED: local env] | `3.11.9` [VERIFIED: local env] | — |
| Node / npm | Frontend helper tests and static client auditing. [VERIFIED: tests/frontend/*.mjs, package.json] | ✓ [VERIFIED: local env] | `node v20.19.4`, `npm 10.8.2` [VERIFIED: local env] | — |
| Go toolchain | `backend-go` tests, migration tests, API/worker binaries. [VERIFIED: backend-go/Makefile, backend-go/go.mod] | ✓ [VERIFIED: local env] | Installed `go1.24.3`; repo requires `go 1.25.0`. [VERIFIED: local env, backend-go/go.mod] | Static code audit still works, but live Go verification should wait for toolchain alignment. [VERIFIED: local env, backend-go/go.mod] |
| PostgreSQL access | Real migration verification and live Go API start. [VERIFIED: backend-go/docs/phase1-runbook.md, backend-go/internal/config/config.go] | ✗ [VERIFIED: local env] | `psql` absent on this host. [VERIFIED: local env] | Limit Phase 1 to static schema/route research until a disposable PostgreSQL URL is provided. [VERIFIED: backend-go/db/migrations/migrations_test.go, backend-go/docs/phase1-runbook.md] |
| Redis access | Live worker loop and queue-backed task semantics. [VERIFIED: backend-go/internal/config/config.go, backend-go/cmd/worker/main.go] | ✗ [VERIFIED: local env] | `redis-cli` absent on this host. [VERIFIED: local env] | Use existing unit/e2e fixtures for planning; defer live worker smoke tests. [VERIFIED: backend-go/tests/e2e/jobs_flow_test.go] |
| Docker | Disposable local infra fallback for PostgreSQL/Redis. [VERIFIED: docker-compose.yml, Dockerfile] | ✗ [VERIFIED: local env] | — | No local container fallback on this host. [VERIFIED: local env] |

**Missing dependencies with no fallback:**
- None for planning-only research in this phase. [VERIFIED: .planning/phases/01-compatibility-baseline/01-CONTEXT.md, .planning/ROADMAP.md]

**Missing dependencies with fallback:**
- Local PostgreSQL/Redis/Docker are absent, so Phase 1 can still produce route/schema/runtime contracts but cannot assume live Go API/worker smoke verification on this host. [VERIFIED: local env, backend-go/docs/phase1-runbook.md, backend-go/internal/config/config.go]

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes [VERIFIED: src/web/app.py] | Freeze whether parity preserves current cookie-based HTML auth and current API exposure, or whether Phase 1 introduces a shared admin auth boundary. [VERIFIED: src/web/app.py] |
| V3 Session Management | yes [VERIFIED: src/web/app.py, src/web/routes/payment.py] | Preserve `webui_auth` cookie semantics and payment session bootstrap/token endpoint behavior until an explicit replacement exists. [VERIFIED: src/web/app.py, src/web/routes/payment.py, static/js/payment.js] |
| V4 Access Control | yes [VERIFIED: src/web/routes/accounts.py, src/web/routes/settings.py, src/web/routes/email.py, src/web/routes/payment.py, src/web/routes/logs.py, src/web/routes/upload/*.py] | Add parity tests around privileged export, import, cleanup, and service-config endpoints before cutover plans move them. [VERIFIED: same] |
| V5 Input Validation | yes [VERIFIED: src/web/routes/*.py, backend-go/internal/{accounts,jobs,registration}/http/handlers.go] | Preserve Python Pydantic validation and current Go typed decode behavior as contract fixtures. [VERIFIED: same] |
| V6 Cryptography | yes [VERIFIED: src/web/app.py] | Keep cookie HMAC and standard library cryptography as-is; do not hand-roll auth token changes inside Phase 1. [VERIFIED: src/web/app.py] |

### Known Threat Patterns for This Migration

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Auth boundary drift between Python HTML routes, `/payment`, and `/api` mounts. [VERIFIED: src/web/app.py] | Elevation of Privilege / Information Disclosure | Write the current exposure contract down in Phase 1 and add tests for whichever boundary is chosen. [VERIFIED: src/web/app.py] |
| Schema drift between SQLite-backed Python installs and PostgreSQL-backed Go runtime. [VERIFIED: src/database/session.py, backend-go/internal/config/config.go] | Tampering / Integrity | Use a shared schema manifest plus migration verification instead of assuming name overlap equals parity. [VERIFIED: src/database/models.py, backend-go/db/migrations/*.sql, backend-go/db/migrations/migrations_test.go] |
| Loss of task state or log semantics when moving from in-process Python state to queue-backed Go state. [VERIFIED: src/web/task_manager.py, src/web/routes/websocket.py, backend-go/internal/registration/batch_service.go] | Denial of Service / Integrity | Promote current task and websocket behaviors into compatibility fixtures before runtime replacement work starts. [VERIFIED: tests/test_registration_routes.py, tests/test_registration_task_binding.py, backend-go/tests/e2e/jobs_flow_test.go] |
| Sensitive service config leakage through `/full` endpoints, exports, or DB import/export paths. [VERIFIED: src/web/routes/email.py, src/web/routes/upload/cpa_services.py, src/web/routes/upload/sub2api_services.py, src/web/routes/settings.py, src/web/routes/accounts.py] | Information Disclosure | Freeze masking, access, and response-shape rules in Phase 1 so later migration work cannot widen exposure accidentally. [VERIFIED: same] |

## Sources

### Primary (HIGH confidence)
- `.planning/phases/01-compatibility-baseline/01-CONTEXT.md` - locked scope and Phase 1 decisions. [VERIFIED: local file read]
- `.planning/REQUIREMENTS.md`, `.planning/ROADMAP.md`, `.planning/STATE.md` - requirement mapping, phase ordering, and current focus. [VERIFIED: local file read]
- `src/web/routes/__init__.py`, `src/web/app.py`, `src/web/routes/{registration,accounts,settings,email,payment,logs,team,team_tasks,websocket}.py` - Python route surface, page auth, websocket semantics, and domain hotspots. [VERIFIED: local file read]
- `templates/*.html`, `static/js/{app,accounts,accounts_overview,settings,email_services,logs,payment,auto_team}.js` - current client dependencies and route usage. [VERIFIED: local file read]
- `src/database/models.py`, `src/database/team_models.py`, `src/database/session.py`, `src/config/settings.py`, `src/config/constants.py` - Python persistence and runtime defaults. [VERIFIED: local file read]
- `backend-go/internal/http/router.go`, `backend-go/internal/{accounts,jobs,registration}/http/handlers.go`, `backend-go/internal/registration/{available_services.go,outlook_service.go,stats.go,batch_service.go}`, `backend-go/internal/accounts/{types.go,service.go,repository_postgres.go}`, `backend-go/internal/config/config.go` - current Go baseline surface and runtime rules. [VERIFIED: local file read]
- `backend-go/db/migrations/{0001_init_jobs.sql,0002_init_accounts_registration.sql,0003_extend_registration_service_configs.sql}`, `backend-go/db/migrations/migrations_test.go` - Go schema and migration verification. [VERIFIED: local file read]
- `tests/*.py`, `tests/frontend/*.test.mjs`, `backend-go/tests/e2e/*.go`, `backend-go/Makefile`, `backend-go/docs/phase1-runbook.md` - existing compatibility harness and operational commands. [VERIFIED: local file read]
- Local environment probes for `python3`, `go`, `node`, `npm`, `uv`, `redis-cli`, `psql`, and `docker`. [VERIFIED: local env]

### Secondary (MEDIUM confidence)
- None.

### Tertiary (LOW confidence)
- None.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - versions and tool roles are pinned in repo files or local environment output. [VERIFIED: pyproject.toml, backend-go/go.mod, package.json, local env]
- Architecture: HIGH - route, client, schema, and runtime boundaries were audited directly from source files. [VERIFIED: src/web/routes/*.py, src/web/app.py, static/js/*.js, src/database/*.py, backend-go/internal/**/*.go, backend-go/db/migrations/*.sql]
- Pitfalls: MEDIUM - each pitfall is grounded in current code, but rollout impact still depends on deployment choices that were not fully inspectable in this session. [VERIFIED: src/web/app.py, src/web/task_manager.py, src/database/session.py][ASSUMED]

**Research date:** 2026-04-05
**Valid until:** 2026-04-19
