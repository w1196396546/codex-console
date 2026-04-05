# Phase 1 Runtime Boundary Contract

## Auth Boundary

- Most HTML pages rendered by `src/web/app.py` enforce `_is_authenticated(request)` and redirect to `/login` when the `webui_auth` cookie is absent.
- `/payment` is the notable page-level exception: it renders without the `_is_authenticated` gate.
- `/api` and `/api/ws` are mounted globally through `app.include_router(api_router, prefix="/api")` and `app.include_router(ws_router, prefix="/api")` without a shared authentication dependency or middleware.
- This is the **current compatibility reference, not a hardening endorsement**. Later phases must preserve this behavior unless they explicitly scope, verify, and communicate an auth-boundary change.

## Runtime Ownership Baseline

- Existing Go baseline, not to be reimplemented in Phase 1:
  - jobs control API under `/api/jobs`
  - registration/task compatibility endpoints under `/api/registration/*`
  - websocket streams `/api/ws/task/{task_uuid}` and `/api/ws/batch/{batch_id}`
  - `/api/accounts` listing baseline
  - uploader persistence config reads in `backend-go/internal/uploader/`
  - native registration runner foundations in `backend-go/internal/nativerunner/`
- Python still owns:
  - HTML page rendering and login cookie flow
  - broader account management APIs
  - settings, email-service, uploader-config, proxy, and log admin APIs
  - payment and bind-card orchestration
  - team discovery/sync/invite/task flows
  - process-local task state in `src/web/task_manager.py`

## Task and Batch Semantics

The following fields and channels are part of the compatibility contract:

- `task_uuid`
- `status`
- `logs`
- `log_offset`
- `log_next_offset`
- `/api/ws/task/{task_uuid}`
- `/api/ws/batch/{batch_id}`

Current behavior to preserve while migrating:

- Python task and batch flows support pause, resume, cancel, and heartbeat-style websocket interaction.
- Python runtime keeps live state in `src/web/task_manager.py` and augments durable task records through route-layer writes.
- Go registration compatibility derives current task and batch state from jobs plus batch projections.
- Later phases must preserve observable semantics first, even if the implementation source of truth changes underneath.

## Go Cutover Preconditions

- `DATABASE_URL` is required for the Go API and worker runtime.
- `REDIS_ADDR` is required for the Go API and worker runtime.
- PostgreSQL and Redis are therefore required for any Go-owned production path beyond the current limited baseline.
- SQLite is **legacy reference only** for the final Go cutover path.
- The local repository currently documents Go `1.25.0` in `backend-go/go.mod`; local verification on an older Go toolchain should not be mistaken for production readiness.

## Runtime Split Notes

- Python remains the compatibility oracle for current page-level behavior and Python-only domains.
- Go remains the target runtime for durable jobs, queue-backed execution, and future backend ownership.
- Phase 1 freezes this split; it does not resolve it by implementation.

## Decision Traceability

- **D-01:** Python backend behavior remains the reference for not-yet-migrated domains.
- **D-02:** Path, method, payload/status fields, websocket semantics, and critical side effects are all part of parity.
- **D-03:** Existing Go jobs, registration/task APIs, websocket streams, account-listing baseline, uploader persistence, and native runner foundations remain baseline.
- **D-04:** Phase 1 does not re-plan or re-implement existing Go foundations.
- **D-05:** Current templates and static/js clients are treated as first-class consumers of the runtime contract.
- **D-06:** Shared schema/runtime rules are frozen before later domain cutover work begins.

---
*Phase: 01-compatibility-baseline*
*Runtime contract frozen: 2026-04-05*
