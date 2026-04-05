---
phase: 03-management-apis
plan: "03"
subsystem: api
tags: [go, postgres, chi, email-services, outlook, tempmail]
requires:
  - phase: 01-compatibility-baseline
    provides: shared email_services contract and last_used gap definition
  - phase: 03-management-apis/03-02
    provides: settings/tempmail owner slice consumed by emailservices tests and service settings reads
provides:
  - Go-owned emailservices service/repository/handler slice for `/api/email-services*`
  - `email_services.last_used` migration coverage for management parity
  - Outlook registration projection and Go-native tempmail/yyds test flow consumption
affects: [03-06, email-services-page, settings-page]
tech-stack:
  added: []
  patterns: [service-repository-handler slice, filtered-vs-full config split, settings-table dependency consumption]
key-files:
  created:
    - backend-go/db/migrations/0005_extend_email_services_management.sql
    - backend-go/db/migrations/phase3_email_services_migration_test.go
    - backend-go/internal/emailservices/http/handlers.go
  modified:
    - backend-go/internal/emailservices/types.go
    - backend-go/internal/emailservices/service.go
    - backend-go/internal/emailservices/repository_postgres.go
    - backend-go/internal/emailservices/service_test.go
    - backend-go/internal/emailservices/http/handlers_test.go
key-decisions:
  - "列表/详情继续过滤敏感 config，仅 `/full` 返回完整配置，避免把 secrets 扩散到页面常规加载链路。"
  - "03-03 只消费 tempmail/yyds settings，不重建 `/api/settings/tempmail` owner；该依赖保持在 03-02。"
  - "服务测试端点保持 Go owner，通过 native mail provider 的 `Create` 探针完成最小连接校验，不回退 Python。"
patterns-established:
  - "Emailservices read/write parity uses one service with explicit action/request structs and JSON-compatible responses."
  - "Outlook registration projection is derived from Go account lookup plus filtered config hints, not Python ORM calls."
requirements-completed: [MGMT-02]
duration: 15m
completed: 2026-04-05
---

# Phase 03 Plan 03: Email Services Summary

**Go email-services management slice with last_used schema parity, filtered/full config contracts, Outlook batch admin, and tempmail test endpoints**

## Performance

- **Duration:** 15m
- **Started:** 2026-04-05T13:17:43Z
- **Completed:** 2026-04-05T13:33:03Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- Added `0005` migration and migration test so Go now explicitly covers `email_services.last_used`.
- Implemented `backend-go/internal/emailservices` service/repository layer for stats, list/detail/full, filtered config hints, and Outlook registration projection.
- Exposed `/api/email-services*` compatibility handlers for CRUD, enable/disable, reorder, Outlook batch import/delete, and tempmail/yyds connectivity tests without touching final router integration.

## Task Commits

Each task was committed atomically:

1. **Task 1: 补齐 email_services schema 与 read-side/stat/full 合同** - `6a3d9de` (test), `b454849` (feat)
2. **Task 2: 实现 email-services 写侧、测试、重排与 Outlook 批量管理 handlers** - `ca884bd` (test), `a9b66a7` (feat)

**Plan metadata:** recorded in the final docs commit after state/roadmap updates

_Note: Task 1 and Task 2 both followed TDD red → green commits._

## Files Created/Modified
- `backend-go/db/migrations/0005_extend_email_services_management.sql` - adds `last_used` and management sort index for `email_services`.
- `backend-go/db/migrations/phase3_email_services_migration_test.go` - locks the migration contract for `last_used`.
- `backend-go/internal/emailservices/types.go` - typed request/response/action contracts for the compatibility API.
- `backend-go/internal/emailservices/service.go` - read/write orchestration, filtered/full projection, Outlook batch import/delete, and tempmail/yyds testing.
- `backend-go/internal/emailservices/repository_postgres.go` - Postgres persistence plus shared settings/account lookup consumption.
- `backend-go/internal/emailservices/service_test.go` - service-level parity tests, including explicit 03-02 tempmail dependency note.
- `backend-go/internal/emailservices/http/handlers.go` - compatibility handlers for the full `/api/email-services*` family.
- `backend-go/internal/emailservices/http/handlers_test.go` - route/method/JSON error contract coverage for handler compatibility.

## Decisions Made

- Kept list/detail responses on filtered config and reserved full secrets for `/full` to match Python and satisfy the threat model.
- Reused the shared settings table and Go account lookup for tempmail/yyds and Outlook registration hints instead of introducing a second owner path.
- Left router wiring and `cmd/api` bootstrap untouched so 03-06 can integrate this slice with the rest of Phase 3 in one pass.

## Deviations from Plan

None - plan executed within 03-03 scope.

## Issues Encountered

- The workspace already had unrelated staged changes in the Git index, so the first RED commit also captured pre-staged app-logs files (`0006_add_app_logs_management.sql` and `phase3_app_logs_migration_test.go`). Subsequent 03-03 commits staged only emailservices files, and the slice implementation itself does not depend on those app-logs artifacts.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `backend-go/internal/emailservices` is ready for 03-06 router wiring and end-to-end cutover verification.
- Final page-level cutover still depends on the separate 03-02 settings slice continuing to own `/api/settings/tempmail`.

## Self-Check

PASSED

- Verified summary and all 03-03 emailservices files exist on disk.
- Verified task commits `6a3d9de`, `b454849`, `ca884bd`, and `a9b66a7` are present in git history.

---
*Phase: 03-management-apis*
*Completed: 2026-04-05*
