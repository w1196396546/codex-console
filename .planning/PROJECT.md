# Codex Console Go Migration

## What This Is

Codex Console is an existing brownfield account-management and registration console whose current product behavior mostly lives in the Python FastAPI/Jinja stack in `webui.py` and `src/`. `backend-go/` already provides a partial Go control plane with PostgreSQL/Redis, jobs, accounts, and native registration components. This project initializes only the remaining migration work needed to replace the Python backend responsibilities with Go while preserving today's API surface, stored data contracts, and critical business behavior.

## Core Value

The Go backend can take over the current Codex Console backend responsibilities without forcing existing clients, persisted data, or critical registration, payment, and team workflows to change behavior.

## Requirements

### Validated

- ✓ Operators can already use the Python Web UI for registration, account management, settings, logs, payment/bind-card, and team workflows through `webui.py`, `src/web/`, `src/core/`, and `src/database/`.
- ✓ The Go backend already owns PostgreSQL/Redis-backed jobs, registration start/batch APIs, task and batch websocket streaming, and worker orchestration through `backend-go/cmd/api/main.go`, `backend-go/cmd/worker/main.go`, `backend-go/internal/jobs/`, and `backend-go/internal/registration/`.
- ✓ The Go backend already persists core `accounts`, `email_services`, `settings`, `cpa_services`, `sub2api_services`, and `tm_services` state in PostgreSQL through `backend-go/db/migrations/0002_init_accounts_registration.sql`, `backend-go/db/migrations/0003_extend_registration_service_configs.sql`, `backend-go/internal/accounts/`, and `backend-go/internal/uploader/`.
- ✓ A native Go registration runner already exists and is wired into the Go worker through `backend-go/internal/nativerunner/` and `backend-go/cmd/worker/main.go`, even though migration coverage is still incomplete.

### Active

- [ ] Finish the remaining Go API and domain migration for Python-only backend surfaces that current templates and scripts still call directly.
- [ ] Remove critical-path dependence on the Python runtime while preserving route semantics, payload shapes, status values, and side effects.
- [ ] Keep stored data contracts compatible across accounts, settings, upload configs, proxies, bind-card data, logs, and team data during migration.
- [ ] Cut the current frontend clients over to the Go backend, then retire Python backend responsibilities from production usage.

### Out of Scope

- Frontend redesign or template/JavaScript rewrite - migration should preserve the current operator-facing UI first.
- Product feature expansion unrelated to migration - the goal is parity, not net-new capabilities.
- Intentional API redesign or schema simplification - compatibility is a hard constraint for this initiative.
- Security-hardening-only work that does not unblock migration - capture later unless it is required for Go cutover.

## Context

- The repository is brownfield and currently split across Python and Go runtimes.
- Python still exposes most `/api/*` routes plus all HTML pages through `src/web/app.py` and `src/web/routes/`.
- Go currently covers registration, jobs, websocket task/batch streaming, Outlook batch support, and paginated account listing, but not the full management, payment, logs, settings, upload-service, proxy, or team surfaces.
- Existing frontend assets in `templates/` and `static/js/` already encode route expectations, so migration must preserve those contracts or introduce a transparent compatibility layer.
- Python and Go currently use different persistence/runtime patterns (`SQLite + SQLAlchemy + in-memory task manager` versus `PostgreSQL + Redis + sqlc + Asynq`), so the remaining work is semantic migration, not just code translation.

## Constraints

- **Compatibility**: Keep current API paths, HTTP methods, JSON fields, status values, and websocket semantics compatible - the existing frontend and automation surface already depends on them.
- **Data Contract**: Preserve current persisted record shapes and migration paths - breaking existing fields would invalidate live data and current operational workflows.
- **Brownfield Scope**: Only plan remaining migration work - already migrated Go foundations are baseline, not a fresh greenfield roadmap.
- **Execution Safety**: Python may remain as a bounded transition aid temporarily, but the final state cannot depend on Python on the critical backend path.
- **Operational Parity**: Registration, payment/bind-card, team, logs, and admin workflows must keep current business behavior so operators do not need a new playbook just because the implementation moved to Go.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Use a domain-by-domain strangler migration instead of a big-bang rewrite | The repo already has partial Go coverage; controlled cutover reduces parity drift and rollback risk | - Pending |
| Treat API, data, and workflow compatibility as release blockers | The current templates, scripts, and operators are tightly coupled to existing behavior | - Pending |
| Count current Go jobs, registration, accounts, and uploader foundations as validated baseline | The user explicitly asked to plan only the remaining migration work | - Pending |
| Keep the current frontend/UI in place during backend migration | Rewriting UI at the same time would hide backend parity gaps and expand scope | - Pending |
| Remove Python from the production critical path only after parity evidence exists | Cutting over early would turn migration unknowns into user-facing regressions | - Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? -> Move to Out of Scope with reason
2. Requirements validated? -> Move to Validated with phase reference
3. New requirements emerged? -> Add to Active
4. Decisions to log? -> Add to Key Decisions
5. "What This Is" still accurate? -> Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check - still the right priority?
3. Audit Out of Scope - reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-04-05 after initialization*
