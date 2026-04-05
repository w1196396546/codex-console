---
phase: 02-native-registration-runtime
plan: "03"
subsystem: api
tags: [registration, accounts, postgres, auto-upload, runner]
requires:
  - phase: 01-compatibility-baseline
    provides: shared schema and runtime parity contracts for account persistence and upload side effects
  - phase: 02-native-registration-runtime/02-01
    provides: native registration execution seams and worker-owned runner wiring
  - phase: 02-native-registration-runtime/02-02
    provides: jobs-backed registration task lifecycle used by the executor path
provides:
  - typed runner-to-executor account persistence payloads that stay out of user-facing results
  - partial-failure account persistence for token-pending and login-incomplete registration paths
  - verified post-persist CPA/Sub2API/TM upload ordering with existing writeback asymmetry preserved
affects: [management-apis, cutover-and-decommission, websocket-runtime]
tech-stack:
  added: []
  patterns:
    - typed RunnerOutput/RunnerError persistence handoff
    - executor-owned persist-then-upload side-effect chain
    - compare-and-swap token-completion runtime account updates
key-files:
  created: []
  modified:
    - backend-go/internal/registration/executor.go
    - backend-go/internal/registration/executor_persistence_test.go
    - backend-go/internal/registration/native_runner.go
    - backend-go/internal/registration/native_runner_boundary_test.go
    - backend-go/internal/registration/python_runner.go
    - backend-go/internal/registration/python_runner_persistence_test.go
    - backend-go/internal/accounts/repository_postgres.go
    - backend-go/internal/accounts/repository_postgres_test.go
key-decisions:
  - "Runner account persistence now crosses the executor boundary via RunnerOutput and RunnerError instead of leaking through result payload fields."
  - "Typed runner failures still persist compatible partial account state through Go when account persistence data is present."
  - "Token-completion runtime metadata is updated with Postgres compare-and-swap semantics so later writes do not clobber stronger state."
patterns-established:
  - "Runner adapters keep persistence payloads internal while preserving the user-visible result shape."
  - "Executor ordering remains runner output -> account upsert -> auto-upload dispatch -> CPA/Sub2API writeback only."
requirements-completed: [RUN-03, RUN-01]
duration: 16m
completed: 2026-04-05
---

# Phase 2 Plan 03: Native Registration Persistence Summary

**Typed runner persistence handoff, partial-error account upserts, and verified CPA/Sub2API/TM post-persist upload semantics**

## Performance

- **Duration:** 16m
- **Started:** 2026-04-05T10:33:00Z
- **Completed:** 2026-04-05T10:49:17Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments

- Moved runner account persistence data off the user result payload and into a typed Go executor boundary that both native and Python-backed runners can share.
- Preserved Python-compatible account field semantics for successful and partial-failure registration outcomes, including token-pending/login-incomplete persistence.
- Verified that Go still owns the ordered persist-then-auto-upload chain and keeps CPA/Sub2API writeback asymmetric with TM log-only behavior.

## Task Commits

1. **Task 1: 对齐 native runner 输出与 Go 账号持久化合同** - `bd8fd97` (feat)
2. **Task 2: 固化 Go auto-upload 顺序与写回非对称语义** - `bd8fd97` (feat, same executor seam; no additional file delta beyond the shared commit)

## Files Created/Modified

- `backend-go/internal/registration/executor.go` - Switched executor persistence to typed `RunnerOutput`/`RunnerError` handling and kept persist-before-upload ordering intact.
- `backend-go/internal/registration/executor_persistence_test.go` - Locked persistence, typed-error, and upload-order semantics at the executor boundary.
- `backend-go/internal/registration/native_runner.go` - Kept native runner persistence payloads internal while preserving the external result shape.
- `backend-go/internal/registration/native_runner_boundary_test.go` - Verified native runner/executor boundary behavior with Go persistence enabled.
- `backend-go/internal/registration/python_runner.go` - Decoded bridge persistence payloads into typed Go requests and propagated fatal errors with persistence context.
- `backend-go/internal/registration/python_runner_persistence_test.go` - Covered typed bridge persistence payloads, optional fields, and fatal-error persistence output.
- `backend-go/internal/accounts/repository_postgres.go` - Added compare-and-swap writes for token-completion runtime metadata in `accounts.extra_data`.
- `backend-go/internal/accounts/repository_postgres_test.go` - Verified compare-and-swap SQL conditions and fence-conflict behavior.

## Decisions Made

- Used typed `RunnerOutput` and `RunnerError` objects instead of embedding internal persistence payloads in user-visible result maps.
- Kept partial registration failures writing compatible account rows through Go so `token_pending` and `login_incomplete` flows still survive executor errors.
- Reused the existing account merge path and upload dispatcher semantics rather than introducing a parallel persistence or upload writeback path.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- The workspace already contained unrelated backend-go changes. Only the 02-03 files listed above were staged into the plan commit.
- `roadmap update-plan-progress` returned success but left the Phase 2 progress table stale at `1/4`; the roadmap row was corrected manually to `3/4`.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- The websocket/runtime-alignment plan can now rely on a typed executor persistence contract and stable post-persist upload ordering.
- No 02-03 blockers remain; unrelated workspace changes outside this plan were left untouched.

## Self-Check: PASSED

- FOUND: `.planning/phases/02-native-registration-runtime/02-03-SUMMARY.md`
- FOUND: `bd8fd97`

---
*Phase: 02-native-registration-runtime*
*Completed: 2026-04-05*
