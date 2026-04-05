---
phase: 03-management-apis
plan: "05"
subsystem: api
tags: [go, postgres, app_logs, chi, management-api]
requires:
  - phase: 01-compatibility-baseline
    provides: route and shared-schema parity contracts for app_logs and /api/logs
  - phase: 03-management-apis
    provides: Phase 3 management API scope, threat model, and UI contract
provides:
  - app_logs PostgreSQL schema and migration coverage for Go management APIs
  - backend-go/internal/logs service and repository for list/stats/cleanup/clear behavior
  - /api/logs compatibility handlers ready for 03-06 route mounting
affects: [03-06, MGMT-02, logs-page]
tech-stack:
  added: []
  patterns: [compatibility-first Go slice, app_logs over job_logs, JSON detail errors for management APIs]
key-files:
  created:
    - backend-go/db/migrations/0006_add_app_logs_management.sql
    - backend-go/db/migrations/phase3_app_logs_migration_test.go
    - backend-go/internal/logs/types.go
    - backend-go/internal/logs/service.go
    - backend-go/internal/logs/repository_postgres.go
    - backend-go/internal/logs/http/handlers.go
    - backend-go/internal/logs/service_test.go
    - backend-go/internal/logs/http/handlers_test.go
  modified: []
key-decisions:
  - "日志管理单独落到 app_logs slice，明确不复用 job_logs。"
  - "列表响应只暴露 logs 页面当前消费的字段，错误响应统一保留 detail 语义。"
patterns-established:
  - "Management log slices should normalize request bounds in service and keep handler-level compatibility errors in JSON detail form."
  - "Destructive admin endpoints should keep confirm gating in service and tests before router wiring."
requirements-completed: [MGMT-02]
duration: 15m
completed: 2026-04-05
---

# Phase 3 Plan 05: Logs Management Summary

**Go-owned app_logs schema plus compatibility list, stats, cleanup, and clear APIs for the existing /logs page**

## Performance

- **Duration:** 15m
- **Started:** 2026-04-05T13:09:38Z
- **Completed:** 2026-04-05T13:24:45Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments

- Added a dedicated `app_logs` migration instead of pointing `/api/logs` at `job_logs`.
- Implemented `backend-go/internal/logs` types, service, and PostgreSQL repository for filtered list and stats behavior with Shanghai-time output.
- Added `/api/logs`, `/api/logs/stats`, `/api/logs/cleanup`, and `DELETE /api/logs?confirm=true` handlers with compatibility tests and destructive-confirm protection.

## Task Commits

1. **Task 1: 增加 app_logs schema，并实现 logs repository/service 查询统计合同** - `28d41f7` (feat)
2. **Task 2: 暴露 `/api/logs*` handlers，保留 cleanup 与 clear confirm 语义** - `ad9e095` (feat)

**Plan metadata:** pending

## Files Created/Modified

- `backend-go/db/migrations/0006_add_app_logs_management.sql` - Creates and backfills the Go-side `app_logs` schema with query-oriented indexes.
- `backend-go/db/migrations/phase3_app_logs_migration_test.go` - Locks the migration contract and optional real-Postgres roundtrip for `app_logs`.
- `backend-go/internal/logs/types.go` - Defines compatibility request/response types and normalization bounds for log management.
- `backend-go/internal/logs/service.go` - Maps repository data to logs-page payloads, Shanghai timestamps, cleanup defaults, and clear confirmation behavior.
- `backend-go/internal/logs/repository_postgres.go` - Implements filtered `app_logs` queries, stats aggregation, cleanup, and clear operations against PostgreSQL.
- `backend-go/internal/logs/service_test.go` - Covers list/stats/cleanup/clear service behavior and query construction.
- `backend-go/internal/logs/http/handlers.go` - Exposes the compatibility `/api/logs*` handlers without touching final router wiring.
- `backend-go/internal/logs/http/handlers_test.go` - Verifies route semantics, JSON payload shape, cleanup validation, and clear confirmation errors.

## Decisions Made

- Kept `app_logs` as an explicit management storage concern instead of reusing `job_logs`, matching the Phase 1 schema contract and 03-05 threat model.
- Trimmed list payloads to the fields consumed by `static/js/logs.js` while preserving JSON `detail` errors for operator-facing failures.
- Kept router mounting out of this plan; the handlers are ready but remain unmounted until 03-06.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- The repository root `.gitignore` rule `logs/` also matched `backend-go/internal/logs`, so task commits had to stage those files with `git add -f`.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `backend-go/internal/logs/http` is ready for 03-06 to mount into the shared router without changing handler behavior.
- The existing `/logs` page contract now has a Go owner for schema, list/stats, cleanup, and clear semantics.

## Self-Check

PASSED

- FOUND: `.planning/phases/03-management-apis/03-management-apis-05-SUMMARY.md`
- FOUND: `28d41f7`
- FOUND: `ad9e095`

---
*Phase: 03-management-apis*
*Completed: 2026-04-05*
