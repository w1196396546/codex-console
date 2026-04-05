---
phase: 02-native-registration-runtime
plan: "04"
subsystem: api
tags: [go, websocket, registration, compatibility, e2e]
requires:
  - phase: 01-compatibility-baseline
    provides: registration websocket contract and runtime boundary rules
  - phase: 02-native-registration-runtime
    provides: task and batch HTTP/polling compatibility projections from plan 02
provides:
  - Task websocket status/log frames with timestamp, log cursors, and cancelling-aware control replies
  - Batch websocket status/log frames with progress metadata, timestamp, and cancelling projection
  - Outlook batch websocket reuse of `/api/ws/batch/{batch_id}` without contract drift
affects: [phase-02-runtime, phase-03-management-apis, registration-ui]
tech-stack:
  added: []
  patterns:
    - websocket status frames carry the same cursor/progress metadata clients use for polling fallback
    - websocket projection layers can synthesize compatibility-only intermediate states while jobs remain the durable source of truth
key-files:
  created: []
  modified:
    - backend-go/internal/registration/ws/task_socket.go
    - backend-go/internal/registration/ws/task_socket_test.go
    - backend-go/internal/registration/ws/batch_socket.go
    - backend-go/internal/registration/ws/batch_socket_test.go
    - backend-go/tests/e2e/jobs_flow_test.go
key-decisions:
  - "Task websocket 在控制回包上投影 `cancelling` 中间态和中文 message，但 jobs 仍保持持久真值源。"
  - "Batch websocket 状态帧补齐 `skipped/current_index/log_*` 和 `timestamp`，让重连与 polling 回退共享同一游标语义。"
  - "当 HTTP 先消费掉 batch service 的一次性 `cancelling` 窗口时，由 websocket 投影层补发 `cancelling`，避免 Outlook 批量直接跳到 `cancelled`。"
patterns-established:
  - "Task websocket snapshots send current log window bounds before history frames, while live log frames advance offsets monotonically."
  - "Batch websocket keeps `/api/ws/batch/{batch_id}` as the single live channel for standard and Outlook batch flows."
requirements-completed: [RUN-02, COMP-03]
duration: 6min
completed: 2026-04-05
---

# Phase 2 Plan 04: Websocket runtime semantics summary

**Go registration websockets now preserve task, batch, and Outlook batch progress semantics with timestamped frames, monotonic log cursors, and visible `cancelling` transitions on the existing `/api/ws/*` channels.**

## Performance

- **Duration:** 6 min
- **Started:** 2026-04-05T10:48:39Z
- **Completed:** 2026-04-05T10:54:24Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- Aligned task websocket snapshots, live log frames, and control replies with the Python-facing contract by adding `timestamp`, `log_offset`, `log_next_offset`, and `cancelling` visibility.
- Aligned batch websocket status frames with polling-compatible progress metadata, including `skipped`, `current_index`, `log_base_index`, and monotonic batch log cursors.
- Preserved Outlook batch reuse of `/api/ws/batch/{batch_id}` by projecting a websocket-side `cancelling -> cancelled` transition even when HTTP polling settles the batch state first.

## Task Commits

Each task was committed atomically:

1. **Task 1: 对齐 task websocket 帧形状与控制语义** - `e96d75c` (`test`), `d07f74b` (`feat`)
2. **Task 2: 对齐 batch 与 Outlook batch websocket 语义** - `e664a16` (`test`), `eb8a5bd` (`feat`)

## Files Created/Modified
- `backend-go/internal/registration/ws/task_socket.go` - Added timestamped task status/log frame builders, cursor fields, and websocket-only cancelling projection.
- `backend-go/internal/registration/ws/task_socket_test.go` - Locked task websocket snapshot, control reply, and final status semantics.
- `backend-go/internal/registration/ws/batch_socket.go` - Added batch status/log frame metadata and websocket-side cancelling projection for batch and Outlook flows.
- `backend-go/internal/registration/ws/batch_socket_test.go` - Locked batch websocket progress, cursor, and control reply semantics.
- `backend-go/tests/e2e/jobs_flow_test.go` - Proved existing `/api/ws/task/{task_uuid}` and `/api/ws/batch/{batch_id}` consumers still work for single, batch, and Outlook batch flows.

## Decisions Made

- Reused the existing websocket handlers and route mounts instead of adding new channels; parity was achieved entirely in the projection layer.
- Kept jobs and batch services as the durable source of truth, and limited compatibility-only intermediate-state synthesis to websocket output.
- Used websocket-specific Chinese control messages from the Python oracle only on direct websocket control replies; HTTP-triggered status observations remain message-free projections.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- Outlook batch HTTP polling can consume the batch service’s one-shot `cancelling` window before websocket pollers observe it. Resolved by projecting a websocket-only `cancelling` status once before the final `cancelled` frame, without widening scope into HTTP or service-layer files.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Phase 2 websocket parity is now complete on the existing `/api/ws/task/{task_uuid}` and `/api/ws/batch/{batch_id}` channels.
- Phase 3 can assume registration UI websocket consumers do not need a channel or contract rewrite while management APIs migrate behind the same frontend.

## Self-Check: PASSED

- Found `.planning/phases/02-native-registration-runtime/02-04-SUMMARY.md`
- Found task commits `e96d75c`, `d07f74b`, `e664a16`, and `eb8a5bd`
