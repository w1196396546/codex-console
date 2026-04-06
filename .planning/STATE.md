---
gsd_state_version: 1.0
milestone: v1.1
milestone_name: go-admin-frontend-refactor
status: initialized
stopped_at: Milestone v1.1 initialized
last_updated: "2026-04-06T07:56:09Z"
last_activity: 2026-04-06
progress:
  total_phases: 4
  completed_phases: 0
  total_plans: 14
  completed_plans: 0
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-06)

**Core value:** The Go runtime can own the operator console end to end while preserving the registration, account, payment, team, logs, and settings workflows operators already depend on.
**Current focus:** Phase 06 — Go Frontend Isolation Baseline

## Current Position

Phase: 06 (Go Frontend Isolation Baseline) — NOT STARTED
Plan: 0 of 3 complete
Status: Milestone initialized; ready for `/gsd-discuss-phase 6` or `/gsd-plan-phase 6`
Last activity: 2026-04-06

Progress: [░░░░░░░░░░] 0%

## Performance Metrics

**Velocity:**

- Total plans completed: 0
- Average duration: -
- Total execution time: 0.0 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 6 | 0 | - | - |
| 7 | 0 | - | - |
| 8 | 0 | - | - |
| 9 | 0 | - | - |

**Recent Trend:**

- Last 5 plans: -
- Trend: Milestone not started

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- [Milestone v1.1]: Build the new admin frontend from a Go-owned copy of the current frontend assets instead of editing the legacy Python frontend in place.
- [Milestone v1.1]: Remove shared project statement, GitHub/Telegram/sponsorship links, and unrelated public-facing copy from the new frontend shell.
- [Milestone v1.1]: Favor a management-system information architecture and denser operator workflow layout rather than preserving the current public/open-source framing.
- [Milestone v1.1]: Preserve existing workflow and contract semantics; this milestone changes shell, layout, and content rather than backend APIs or stored data.
- Phase 0: Plan only the remaining migration delta; existing Go foundations are baseline.
- Phase 0: API, data, and workflow compatibility are the governing release constraint.
- Phase 0: Frontend rewrite is out of scope for this migration milestone.
- [Phase 02]: Worker preparation now injects explicit Postgres-backed proxy selection and Outlook reservation adapters instead of implicit no-op wiring.
- [Phase 02]: Outlook reservation state stays in registration job payloads so concurrent child jobs do not require a second runtime store before 02-02.
- [Phase 02]: Password login, workspace continuation, and add-phone recovery remain inside native auth helpers to keep Python off the normal registration path.
- [Phase 02]: Reuse jobs.Service as the durable registration task list/delete source instead of introducing a second runtime store.
- [Phase 02]: Project batch and outlook cancelling as a two-step HTTP/polling transition while leaving websocket-specific files for 02-04.
- [Phase 02]: Runner account persistence now crosses the executor boundary via RunnerOutput and RunnerError instead of leaking through result payload fields.
- [Phase 02]: Typed runner failures still persist compatible partial account state through Go when account persistence data is present.
- [Phase 02]: Token-completion runtime metadata is updated with Postgres compare-and-swap semantics so later writes do not clobber stronger state.
- [Phase 02]: Task websocket 在控制回包上投影 `cancelling` 中间态和中文 message，但 jobs 仍保持持久真值源。
- [Phase 02]: Batch websocket 状态帧补齐 `skipped/current_index/log_*` 和 `timestamp`，让重连与 polling 回退共享同一游标语义。
- [Phase 02]: 当 HTTP 先消费掉 batch service 的一次性 `cancelling` 窗口时，由 websocket 投影层补发 `cancelling`，避免 Outlook 批量直接跳到 `cancelled`。
- [Phase 03-management-apis]: 日志管理独立落到 app_logs slice，明确不复用 job_logs。
- [Phase 03-management-apis]: 列表响应只暴露 logs 页面当前消费字段，错误保持 JSON detail 语义。
- [Phase 03]: 在 uploader 包内扩展通用 AdminRepository 和兼容 DTO，避免另起 upload-admin 包。
- [Phase 03]: 连接测试和 Sub2API 直传动作放在 uploader service 内，并复用既有 sender/client 工具，不让 handler 直接拼外部 HTTP。
- [Phase 03]: Sub2API 直传仅对成功账号写回 sub2api_uploaded，同时保持 Python 的 success/failed/skipped/detail 结果形状。
- [Phase 03]: Email-services list/detail responses keep filtered config while /full remains the only secret-bearing endpoint.
- [Phase 03]: 03-03 consumes tempmail/yyds settings from the shared settings table and leaves /api/settings/tempmail ownership to 03-02.
- [Phase 03]: Email-services connectivity tests stay Go-owned by probing native mail providers instead of falling back to Python.
- [Phase 03-management-apis]: Settings admin now reuses Python db_key names and metadata instead of introducing an env-only model.
- [Phase 03-management-apis]: Tempmail remains owned by 03-02 because /email-services depends directly on /api/settings/tempmail.
- [Phase 03-management-apis]: Database admin keeps /api/settings/database* paths on Go via PostgreSQL logical backup/import/cleanup behavior.
- [Phase 03]: Kept accounts handler capability discovery inside accounts/http so router.go and cmd/api remain untouched until 03-06.
- [Phase 03]: Reused existing Postgres-backed uploader configs and sender implementations for CPA/Sub2API/TM account actions instead of re-implementing transports.
- [Phase 03-management-apis]: Phase 03: Go router keeps Phase 3 management slices additive on the existing /api/* paths while payment/team remain Phase 4 owners.
- [Phase 03-management-apis]: Phase 03: Management e2e uses real Go services with fake repositories to lock current static-js plain-array/object/full response contracts without a new UI harness.
- [Phase 03]: Reused the shared accountsRepository as uploader UploadAccountStore so /api/sub2api-services/upload reads and writes against the existing accounts truth source.
- [Phase 03]: cmd/api exposes small uploader/router wiring helpers so bootstrap-level tests can hit the mounted Sub2API upload path without widening router ownership.
- [Phase 03-management-apis]: Overview refresh now fetches remote me/wham/codex usage inside the accounts slice before persisting codex_overview.
- [Phase 03-management-apis]: Refresh results count success only when hourly and weekly quota both resolve away from unknown; otherwise they return failed details with operator-readable errors.
- [Phase 04]: Payment slice stays a bind_card_tasks-backed state machine and does not move onto jobs/websocket semantics in 04-01.
- [Phase 04]: Payment session_token, cookies, and subscription writeback continue to use the shared accounts truth source.
- [Phase 04]: High-risk payment operations are isolated behind explicit adapter seams rather than Python route fallbacks in handlers.
- [Phase 04]: 将 shared jobs 的 JobID 直接作为 team task_uuid，避免为 Team 引入第二套 websocket 通道或任务标识。
- [Phase 04]: 在不覆盖当前 workspace 已占用 migration 序号的前提下，将计划中的 team migration 实际落为 0007_init_team_domains.sql。
- [Phase 04]: Phase 04: payment/team route ownership now mounts on the existing /api/* paths and Team live flow continues to reuse /api/ws/task/{task_uuid}.
- [Phase 04]: Phase 04: real payment/team operator validation is deferred by explicit user instruction; no Phase 5 cutover, rollout, or rollback work was executed in 04-03.

### Pending Todos

None yet.

### Blockers/Concerns

- Go currently owns the backend critical path, but it does not yet own a dedicated frontend asset/template delivery pipeline for the operator console.
- Current templates and static JavaScript are tightly coupled to existing routes, shared chrome, and `style.css`, so copy-vs-drift management will be a recurring risk.
- The legacy frontend must remain untouched and available, which means rollout/fallback wiring is part of the milestone scope rather than an afterthought.
- The new frontend must avoid route/API/websocket drift while still changing navigation, layout, copy, and shared page structure.
- Chi static-file mounts and admin route grouping should avoid middleware combinations that conflict with `http.FileServer` behavior during rollout.

## Session Continuity

Last session: 2026-04-06T07:56:09Z
Stopped at: Milestone v1.1 initialized
Resume file: None
