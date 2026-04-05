---
phase: 04-payment-and-team-domains
plan: "02"
subsystem: api
tags: [go, team, jobs, websocket, postgres]
requires:
  - phase: 01-compatibility-baseline
    provides: team route/client parity matrix and shared team_* schema contract
  - phase: 02-native-registration-runtime
    provides: shared jobs service and /api/ws/task/{task_uuid} websocket projection semantics
  - phase: 03-management-apis
    provides: existing phase boundary discipline and shared accounts-side Go foundations
provides:
  - Go-owned team read models, membership action guards, and team_* PostgreSQL migration
  - accepted-task orchestration for discovery/sync/invite flows on top of shared jobs task UUIDs
  - unmounted /api/team compatibility handlers with task readback and membership action coverage
affects: [phase-04-team, phase-04-router-mount, auto-team-ui]
tech-stack:
  added: []
  patterns:
    - team accepted tasks reuse jobs.JobID as task_uuid so ws_channel remains /api/ws/task/{task_uuid}
    - read-model service and accepted-task TaskService split compatibility shaping from async execution/persistence ownership
key-files:
  created:
    - backend-go/db/migrations/0007_init_team_domains.sql
    - backend-go/internal/team/repository.go
    - backend-go/internal/team/repository_postgres.go
    - backend-go/internal/team/service.go
    - backend-go/internal/team/tasks.go
    - backend-go/internal/team/http/handlers.go
  modified:
    - backend-go/internal/team/repository_postgres_test.go
    - backend-go/internal/team/service_test.go
    - backend-go/internal/team/tasks_test.go
    - backend-go/internal/team/http/handlers_test.go
    - backend-go/internal/team/types.go
key-decisions:
  - "将 shared jobs 的 JobID 直接作为 team task_uuid，避免为 Team 引入第二套 websocket 通道或任务标识。"
  - "把 Team slice 切成 read/membership service 与 accepted-task TaskService，两者都由 team package 拥有，handler 只做 decode 和 detail 错误映射。"
  - "在不覆盖当前 workspace 已占用 migration 序号的前提下，将计划中的 team migration 实际落为 0007_init_team_domains.sql。"
patterns-established:
  - "Team task readback 先读 team_tasks/team_task_items 持久化结果，再叠加 shared jobs 的当前 status/logs。"
  - "Invite/discovery/sync execution is exposed through an injected team TaskExecutor seam while Go admission, scope dedupe, result persistence, and handler contracts stay team-owned."
requirements-completed: [TEAM-01]
duration: 13min
completed: 2026-04-05
---

# Phase 4 Plan 02: Team domain slice summary

**Go team slice with team_* PostgreSQL schema, compatibility read models, membership actions, and jobs-backed accepted-task orchestration for discovery/sync/invite flows**

## Performance

- **Duration:** 13 min
- **Started:** 2026-04-05T15:49:26Z
- **Completed:** 2026-04-05T16:02:24Z
- **Tasks:** 2
- **Files modified:** 11

## Accomplishments
- Added the Go `internal/team` slice with typed read models, membership action guards, and a PostgreSQL-backed repository surface for `teams`, `team_memberships`, `team_tasks`, and `team_task_items`.
- Reused the Phase 2 shared jobs runtime so Team accepted payloads keep `task_uuid`, `ws_channel`, and current task websocket semantics without reintroducing Python `task_manager`.
- Added unmounted `/api/team/*` handlers and tests for discovery, sync-batch, invite, membership actions, and task list/detail compatibility while leaving router/bootstrap ownership to 04-03.

## Task Commits

Each task was committed atomically:

1. **Task 1: 定义 Team 持久化、read model 与 membership action 语义** - `f64ce09` (`feat`)
2. **Task 2: 实现 Team accepted-task 编排与未挂载 compatibility handlers** - `7d2f31e` (`feat`)

## Files Created/Modified
- `backend-go/db/migrations/0007_init_team_domains.sql` - Adds PostgreSQL tables and indexes for `teams`, `team_memberships`, `team_tasks`, and `team_task_items`.
- `backend-go/internal/team/types.go` - Defines team records, compatibility response shapes, and accepted-task execution contracts.
- `backend-go/internal/team/repository.go` - Declares repository interfaces and not-found semantics for read models and task persistence.
- `backend-go/internal/team/repository_postgres.go` - Implements PostgreSQL reads/writes for team domain entities, active-scope lookup, and task/item persistence.
- `backend-go/internal/team/service.go` - Implements team list/detail/membership/task read models and membership action semantics.
- `backend-go/internal/team/tasks.go` - Implements jobs-backed accepted-task admission, active-scope reuse/conflict rules, execution, and task readback overlay.
- `backend-go/internal/team/http/handlers.go` - Exposes unmounted `/api/team/*` compatibility handlers for later router/bootstrap wiring.
- `backend-go/internal/team/repository_postgres_test.go` - Locks migration contract snippets and repository interface coverage.
- `backend-go/internal/team/service_test.go` - Locks read-model and membership action compatibility semantics.
- `backend-go/internal/team/tasks_test.go` - Locks discovery reuse, active-scope conflict, invite execution, and result persistence semantics.
- `backend-go/internal/team/http/handlers_test.go` - Locks accepted payload, membership action, and task readback HTTP contracts.

## Decisions Made

- Reused `jobs.Service` as the live status/log source for Team tasks so `/api/ws/task/{task_uuid}` remains the only websocket channel Team needs.
- Stored Team task truth in dedicated `team_tasks` / `team_task_items` tables while projecting live status/logs from shared jobs on readback.
- Kept handlers unmounted and bootstrap-free in this plan, but made the Team slice internally complete enough for 04-03 to wire without reopening domain logic.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking issue] Adjusted the Team migration filename to avoid an occupied migration sequence**
- **Found during:** Task 1 (定义 Team 持久化、read model 与 membership action 语义)
- **Issue:** The plan targeted `0005_init_team_domains.sql`, but the current workspace already had `0005` and `0006` migration numbers occupied by unrelated backend-go work, so reusing `0005` would create a conflicting goose version.
- **Fix:** Created `backend-go/db/migrations/0007_init_team_domains.sql` and pointed the migration coverage at that file while keeping the Team schema content in 04-02 scope.
- **Files modified:** `backend-go/db/migrations/0007_init_team_domains.sql`, `backend-go/internal/team/repository_postgres_test.go`
- **Verification:** `cd backend-go && go test ./internal/team -run 'Test(Service|Repository|Membership|TeamTask|Accepted|Invite|Discovery|Sync).*' -v`
- **Committed in:** `f64ce09`, `7d2f31e`

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** The deviation was required to keep goose migration numbering valid in the dirty workspace. No payment or router/bootstrap scope leaked into 04-02.

## Issues Encountered

- The current workspace already contains unrelated backend-go migration and nativerunner changes, so staging had to stay file-exact across both task commits.
- The plan asked for a `0005` migration file name that is not viable in the current workspace; the implementation stayed within Team scope and documented the numbering shift explicitly.

## Known Stubs

- `backend-go/internal/team/tasks.go`: `TaskService` requires an injected `TaskExecutor`. 04-02 defines the execution seam, persistence behavior, and handler contract, but does not wire a concrete upstream Team API adapter into bootstrap because 04-03 owns integration.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 04-03 can mount the new Team handlers and inject a concrete `TaskExecutor` without reopening Team read models, membership actions, or accepted-task contracts.
- Payment domain work remains untouched by this plan, preserving the Phase 4 slice boundary.

## Self-Check: PASSED

- Found `.planning/phases/04-payment-and-team-domains/04-02-SUMMARY.md`
- Found `backend-go/db/migrations/0007_init_team_domains.sql`
- Found `backend-go/internal/team/service.go`
- Found `backend-go/internal/team/tasks.go`
- Found `backend-go/internal/team/http/handlers.go`
- Found task commit `f64ce09`
- Found task commit `7d2f31e`
