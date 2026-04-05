---
phase: 04-payment-and-team-domains
plan: "03"
subsystem: api
tags: [go, payment, team, router, websocket, e2e, compatibility]
requires:
  - phase: 04-01
    provides: payment bind-card/session/subscription slice and compatibility handlers
  - phase: 04-02
    provides: team read-model/task slice and compatibility handlers
provides:
  - Go API bootstrap and router ownership for `/api/payment*` and `/api/team*`
  - phase-wide e2e coverage for payment polling semantics and team task websocket parity
  - deferred human-validation checklist for real payment/team runtime verification
affects: [phase-04-payment-team, phase-05-cutover, operator-validation]
tech-stack:
  added: []
  patterns:
    - existing `/api/*` route ownership extended additively without new prefixes
    - Team accepted-task live flow continues to reuse shared `/api/ws/task/{task_uuid}`
    - payment/team phase evidence is split into automated contract coverage and deferred real-environment validation
key-files:
  created:
    - backend-go/tests/e2e/payment_team_flow_test.go
  modified:
    - backend-go/cmd/api/main.go
    - backend-go/internal/http/router.go
    - backend-go/internal/http/router_test.go
key-decisions:
  - "Payment 与 Team 继续沿用现有 `/api/*` 路径和共享 `/api/ws/task/{task_uuid}`，不新增专用前缀或 Team-only websocket。"
  - "04-03 只完成 Go API route ownership 与 phase-wide contract coverage，不提前执行 Phase 5 cutover。"
  - "按用户明确指令，将真实 payment/team 人工验收记为 deferred/non-blocking，并在 summary 中显式保留风险与待验证项。"
patterns-established:
  - "Router tests 直接把 Phase 4 边界从 404 改成 mounted ownership，防止 payment/team 再次漂回未挂载状态。"
  - "E2E 以 static-js 当前 consumer contract 为锚点，用 fake repositories/services 锁定 payment 轮询与 Team websocket 语义。"
requirements-completed: [PAY-01, TEAM-01]
duration: 5min
completed: 2026-04-06
---

# Phase 4 Plan 03: Payment/team route ownership and parity summary

**Go API 现在直接接管 `/api/payment*` 与 `/api/team*` 路由边界，并用 phase-wide e2e 锁定 payment 轮询语义与 Team accepted-task websocket 契约，同时把真实环境人工验收明确延后。**

## Performance

- **Duration:** 5 min
- **Started:** 2026-04-05T16:17:46Z
- **Completed:** 2026-04-05T16:23:02Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- 在 Go API bootstrap 和 router 层正式挂载了 payment/team slices，结束了 Phase 4 之前的 router 404 边界。
- 新增 phase-wide e2e，直接覆盖 payment 页面依赖的 bind-card task 轮询/状态流转，以及 Team accepted-task 复用共享 websocket 的 live flow。
- 明确把真实 `/payment` 与 `/auto-team` 页面的人工作业验证记录为 deferred，而不是把未验证的外部依赖动作伪装成已完成 cutover。

## Task Commits

Each task was committed atomically:

1. **Task 1: 在 Go API bootstrap/router 中挂载 payment 与 team slices，并补 phase-wide e2e compatibility coverage** - `fafbc42` (test), `ab77480` (feat)
2. **Task 2: 人工验收真实 payment/team 运行时与操作员行为** - deferred by explicit user instruction (no code commit)

## Files Created/Modified

- `backend-go/cmd/api/main.go` - 注入 payment/team services 到现有 API bootstrap。
- `backend-go/internal/http/router.go` - 在现有 `/api/*` 路径上挂载 payment/team handlers，同时保留共享 task websocket mount。
- `backend-go/internal/http/router_test.go` - 把 Phase 4 router 边界断言从 “仍然 404” 切换为 “必须已挂载”。
- `backend-go/tests/e2e/payment_team_flow_test.go` - 新增 payment/team phase-wide parity e2e，锁定 payment polling 与 Team websocket live flow。

## Decisions Made

- 继续复用现有 `cmd/api` + `internal/http/router.go` 的依赖注入和路由挂载模式，不引入新的 route namespace。
- Team live task flow 继续复用共享 `/api/ws/task/{task_uuid}`，没有为 Team 单独再造 websocket 协议。
- 用户明确要求跳过当前轮次的人工作业验收，因此本计划只对代码 ownership 和自动化兼容证据负责，不把真实运行时外部动作宣称为已验证。

## Deviations from Plan

None for code scope. Plan 的代码目标按原范围完成；`checkpoint:human-verify` 在 continuation 中按用户明确指令改为 deferred/non-blocking，且没有引入 Phase 5 cutover 或 rollout 工作。

## Issues Encountered

- 当前 workspace 的 payment/team slices 已可被 `cmd/api` 和 router 挂载，但真实 payment 外部动作与 Team 上游动作仍依赖未在本计划中落地的 adapter seam/gateway wiring，因此不能把“路由已接管”直接等同于“真实外部依赖动作已验证”。
- 仓库中存在大量与 04-03 无关的 `backend-go` 脏改动和未跟踪文件。本计划只暂存了 04-03 列出的四个目标文件，没有回滚或覆盖用户工作。

## Deferred Human Validation

以下人工验收项被用户明确指示延后，当前视为 non-blocking，但在任何 Phase 5 cutover 之前都仍需补做：

1. `/payment` 页面上的 `生成支付链接`、`生成并加入绑卡任务（半自动）`、`打开`、`我已完成支付`、`同步订阅`、`删除`。
2. `/accounts` 或 `/payment` 中的 `session-bootstrap`、`mark-subscription`、`batch-check-subscription`。
3. `/auto-team` 页面上的 `发现母号`、`批量同步`、Team 列表/详情/memberships、`撤销邀请`、`移除成员`、`绑定本地账号`、`批量邀请`，以及 accepted 后的 `/api/ws/task/{task_uuid}` 实时链路。
4. “本阶段未做生产 cutover / rollback 动作” 的操作员确认。

延后原因：

- 这些步骤需要真实或 staging 环境、有效外部账号/令牌/支付上下文，以及可接受的真实副作用。
- 04-03 已完成 route ownership 与自动化 contract 证明，但当前 workspace 中的 live external adapter wiring 仍未闭合，继续强行宣称真实运行时通过不诚实。
- 用户明确要求此轮先完成 Phase 4 代码收尾和状态同步，把人工验收作为后续补验证项处理。

## User Setup Required

需要真实或 staging 环境、有效的 ChatGPT/payment/team 上游凭据，以及能接受真实副作用的操作窗口后，才能执行 deferred 人工验收。

## Known Stubs

- `backend-go/cmd/api/main.go:67`：`payment.NewService(...)` 只接入了 Postgres repository 和 accounts truth source，没有在 API bootstrap 中注入 `CheckoutLinkGenerator`、`BillingProfileGenerator`、`BrowserOpener`、`SessionAdapter`、`SubscriptionChecker`、`AutoBinder`。这意味着真实 `generate-link`、`random-billing`、`open-incognito`、`session-bootstrap`、`sync-subscription`、auto-bind 动作仍依赖后续 runtime wiring 或人工验证确认。
- `backend-go/cmd/api/main.go:69`：`team.NewService(teamRepository, nil)` 以 `nil` `MembershipGateway` 挂载，真实 `revoke/remove` membership actions 在 live wiring 下仍会命中 gateway-not-configured 分支。
- `backend-go/cmd/api/main.go:70`：`team.NewTaskService(teamRepository, teamService, jobService, nil)` 以 `nil` `TaskExecutor` 挂载，真实 discovery/sync/invite accepted 后的执行链路仍需后续 runtime wiring；本计划自动化只锁定了 contract、job/task UUID 和 websocket 可观测语义。

## Next Phase Readiness

- Phase 4 的 code ownership 已经闭环：payment/team routes 现在由 Go API 正式挂载，Phase 4 不再停留在 router 404 边界之外。
- 真实 payment/team 运行时与操作员行为验证仍然是 deferred gap，必须在任何 Phase 5 cutover 前补完。
- Phase 5 cutover、rollout、rollback、Python decommission 仍然完全 out of scope，本计划没有提前执行。

## Self-Check: PASSED

- FOUND: `.planning/phases/04-payment-and-team-domains/04-03-SUMMARY.md`
- FOUND: `fafbc42`
- FOUND: `ab77480`
