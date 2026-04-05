---
phase: 03-management-apis
plan: "06"
subsystem: api
tags: [go, chi, management, compatibility, e2e]
requires:
  - phase: 03-management-apis
    provides: accounts/settings/email-services/uploader/logs compatibility slices from plans 01-05
  - phase: 02-native-registration-runtime
    provides: existing Go API bootstrap and websocket/runtime compatibility baseline
provides:
  - Go API bootstrap wiring for Phase 3 management slices on existing `/api/*` paths
  - router scope guards that keep payment and team routes out of Phase 3 ownership
  - management e2e compatibility coverage for accounts, settings, email services, uploader configs, and logs
affects: [phase-04-payment-team, phase-05-cutover, management-ui]
tech-stack:
  added: []
  patterns:
    - slice-scoped router dependency injection for management handlers
    - page-consumer compatibility e2e over mixed array/object/full envelopes
key-files:
  created:
    - backend-go/tests/e2e/management_flow_test.go
  modified:
    - backend-go/cmd/api/main.go
    - backend-go/internal/http/router.go
    - backend-go/internal/http/router_test.go
    - backend-go/tests/e2e/accounts_flow_test.go
key-decisions:
  - "Phase 3 management handlers continue to mount only on the current `/api/*` paths; payment/team ownership stays outside the router until Phase 4."
  - "Management e2e coverage uses Go services plus fake repositories to validate current static-js contracts without introducing any new UI harness."
patterns-established:
  - "Router wiring stays additive and service-driven: a Phase 3 slice only appears when its service is explicitly injected into `internalhttp.NewRouter`."
  - "Compatibility e2e asserts the concrete page contract shape, including plain-array config lists, object envelopes, `/full` secret views, and 404 phase-boundary guards."
requirements-completed: [CUT-01, MGMT-01, MGMT-02]
duration: 11min
completed: 2026-04-05
---

# Phase 3 Plan 06: Management router wiring and UI compatibility summary

**Go API 进程现在统一接线 Phase 3 management slices，并通过 router/e2e 证据锁定当前 Jinja + static-js 页面可直接切到 Go backend 的 `/api/*` 契约。**

## Performance

- **Duration:** 11 min
- **Started:** 2026-04-05T13:53:36Z
- **Completed:** 2026-04-05T14:04:30Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- 在 `cmd/api` 中构建并注入 settings、email-services、uploader admin、logs 与 accounts management services，让 Go API 进程完整拥有 Phase 3 管理域接线。
- 在 `internal/http/router.go` 中挂载所有 Phase 3 管理域 handlers，并用 router tests 明确锁定 payment/team 路由仍未提前接管。
- 增加 accounts + management e2e compatibility tests，直接按当前页面 consumer 断言 plain-array、object-envelope、`/full` 与 Phase 4 边界 404。

## Task Commits

Each task was committed atomically:

1. **Task 1: 在 API bootstrap/router 中挂载 Phase 3 管理域 handlers** - `3cc6f0f` (`feat`)
2. **Task 2: 增加 management e2e compatibility tests，锁定当前 UI consumer 契约** - `ee9c67c` (`test`)

## Files Created/Modified

- `backend-go/cmd/api/main.go` - 构建 Phase 3 settings/email-services/uploader/logs services，并把它们注入统一的 Go router。
- `backend-go/internal/http/router.go` - 挂载 Phase 3 management handlers，同时保留 payment/team 未接管边界。
- `backend-go/internal/http/router_test.go` - 锁定 management 路由已挂载、Phase 4 路由仍是 404。
- `backend-go/tests/e2e/accounts_flow_test.go` - 补齐 accounts 当前页面依赖的 stats/current/overview/detail/tokens 兼容断言，并把测试纳入 03-06 验证正则。
- `backend-go/tests/e2e/management_flow_test.go` - 新增 settings/email-services/uploader/logs mixed envelope compatibility tests 和 payment/team scope guard。

## Decisions Made

- Router 继续使用现有 `/api/*` 路径而不是新增管理前缀，保证当前模板和静态 JS 无需重写即可切换到 Go。
- Phase 3 e2e 明确把 payment/team 作为未挂载边界来断言，而不是默认依赖“不去测它们”。
- 管理域 e2e 选择“真实 Go service + 伪 repo”模式，这样断言的是实际 handler/service 投影结果，而不是独立造一套 JSON fixture。

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- 仓库中存在大量与 03-06 无关的 `backend-go/internal/nativerunner/*` 脏改动。执行过程中只暂存计划内文件，避免污染本计划提交。
- `AccountsStatsSummary` 的 e2e 测试初稿误用了旧字段名；修正为当前 Go 兼容结构后，计划验证命令全部转绿。

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Phase 3 management `/api/*` 接线已经闭环，Phase 5 cutover 可以直接复用这些 router/e2e 证据做整体验证。
- Payment 和 team 路由 owner 仍然清晰留在 Phase 4，没有在本计划里被提前迁入 Go router。

---
*Phase: 03-management-apis*
*Completed: 2026-04-05*

## Self-Check: PASSED

- FOUND: `.planning/phases/03-management-apis/03-management-apis-06-SUMMARY.md`
- FOUND: `3cc6f0f`
- FOUND: `ee9c67c`
