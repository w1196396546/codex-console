---
phase: 02-native-registration-runtime
plan: "02"
subsystem: api
tags: [go, registration, polling, jobs, compatibility]
requires:
  - phase: 01-compatibility-baseline
    provides: registration route/client parity matrix and runtime contract
provides:
  - Go registration task list and delete compatibility endpoints
  - Python-compatible 200 start responses for single, batch, and outlook batch registration
  - Batch and outlook polling fields for cancelling, current_index, skipped, and log window offsets
affects: [phase-02-runtime, phase-02-04-websocket, registration-ui]
tech-stack:
  added: []
  patterns:
    - jobs-backed registration task listing and cleanup
    - two-step cancelling projection for batch and outlook polling compatibility
key-files:
  created: []
  modified:
    - backend-go/internal/registration/http/handlers.go
    - backend-go/internal/registration/batch_service.go
    - backend-go/internal/registration/outlook_service.go
    - backend-go/internal/jobs/service.go
    - backend-go/internal/jobs/repository_runtime.go
    - backend-go/internal/jobs/repository_memory.go
    - backend-go/tests/e2e/jobs_flow_test.go
key-decisions:
  - "复用 jobs 作为注册任务列表/删除的数据源，不引入第二套 Go task_manager。"
  - "用 batch service 的两段式 cancelling 投影补齐 HTTP/polling 语义，不触碰 02-04 websocket 文件。"
patterns-established:
  - "Registration HTTP compatibility should enrich DTOs at the handler layer while leaving execution/runtime ownership in jobs + registration services."
  - "Polling parity fields (`current_index`, `skipped`, `log_base_index`, `log_next_offset`) are regression-tested in handler, integration, and e2e coverage."
requirements-completed: [RUN-02, COMP-03]
duration: 15min
completed: 2026-04-05
---

# Phase 2 Plan 02: Task, Batch, and Outlook polling compatibility summary

**Go registration handlers now expose Python-compatible task list/delete and batch/outlook polling semantics, including 200 start responses and cancelling-aware progress windows.**

## Performance

- **Duration:** 15 min
- **Started:** 2026-04-05T10:24:00Z
- **Completed:** 2026-04-05T10:39:09Z
- **Tasks:** 1
- **Files modified:** 13

## Accomplishments
- Added `/api/registration/tasks` and `DELETE /api/registration/tasks/{task_uuid}` on top of the Go jobs-backed registration compatibility layer.
- Changed `/api/registration/start`, `/batch`, and `/outlook-batch` to return HTTP 200 with richer compatibility payloads instead of accepted-only responses.
- Extended batch and outlook polling responses with `skipped`, `current_index`, `log_base_index`, and a visible `cancelling -> cancelled` transition backed by tests.

## Task Commits

1. **Task 1: 对齐 HTTP 轮询面与任务/批量清理语义** - `8578d64` (`fix`)

## Files Created/Modified
- `backend-go/internal/registration/http/handlers.go` - Added task listing/deletion routes and aligned start responses to Python-compatible 200 semantics.
- `backend-go/internal/registration/batch_service.go` - Added batch polling compatibility fields and cancelling transition projection.
- `backend-go/internal/registration/outlook_service.go` - Mirrored batch compatibility fields for outlook batch polling.
- `backend-go/internal/jobs/service.go` - Exposed list/delete job capabilities to support registration task cleanup flows.
- `backend-go/internal/jobs/repository_runtime.go` - Implemented Postgres-backed job listing and deletion for registration tasks.
- `backend-go/internal/jobs/repository_memory.go` - Implemented in-memory job listing and deletion for compatibility tests and local runtime coverage.
- `backend-go/internal/registration/http/handlers_test.go` - Locked handler-level 200/list/delete/batch-outlook polling compatibility expectations.
- `backend-go/internal/registration/http/integration_test.go` - Verified start/readback flow now includes task listing and cleanup.
- `backend-go/internal/registration/batch_service_test.go` - Covered batch compatibility fields and cancelling transition behavior.
- `backend-go/internal/registration/outlook_service_test.go` - Covered outlook batch compatibility fields and cancelling transition behavior.
- `backend-go/tests/e2e/jobs_flow_test.go` - Proved current clients can list, inspect, cancel, and clean up tasks/batches through Go without a contract rewrite.

## Decisions Made
- Used the existing `jobs.Service` as the single durable source for task list/delete operations instead of introducing a registration-specific store.
- Kept websocket-specific files unchanged; HTTP/polling compatibility was completed by enriching handler/service projections only.

## Deviations from Plan

None - plan executed within the intended registration/jobs compatibility scope.

## Issues Encountered

- `backend-go/internal/registration/http/integration_test.go` already had overlapping uncommitted runtime work in the workspace. The plan continued on that targeted file in place because the 02-02 test coverage explicitly required it.
- Outlook batch e2e initially consumed websocket terminal state before HTTP polling observed the `cancelling` window. Reordered the verification to assert HTTP polling compatibility first without changing websocket implementation files.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Registration HTTP/polling compatibility is now in place for existing clients; Phase 02-03 can finalize account persistence and upload side effects on top of the same jobs-backed flow.
- Websocket frame parity remains isolated to 02-04; no websocket-specific files were modified in this plan.

## Self-Check: PASSED

- Found `.planning/phases/02-native-registration-runtime/02-02-SUMMARY.md`
- Found task commit `8578d64`
