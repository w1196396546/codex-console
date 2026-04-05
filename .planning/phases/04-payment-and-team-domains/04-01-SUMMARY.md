---
phase: 04-payment-and-team-domains
plan: "01"
subsystem: payments
tags: [go, payment, bind-card, postgres, compatibility, handlers]
requires:
  - phase: 01-compatibility-baseline
    provides: payment route parity matrix and bind_card_tasks schema contract
  - phase: 02-native-registration-runtime/02-03
    provides: accounts truth-source persistence semantics and typed side-effect boundaries
  - phase: 03-management-apis/03-08
    provides: mounted API bootstrap pattern and shared accounts repository reuse
provides:
  - bind_card_tasks postgres migration and repository lifecycle for payment runtime state
  - go-owned payment service state machine for session save/bootstrap, bind-card task transitions, and subscription confidence writeback
  - unmounted /api/payment compatibility handlers and consumer-contract regression tests for payment.js and accounts.js
affects: [04-02-team-domain, 04-03-payment-team-integration, phase-05-cutover]
tech-stack:
  added: []
  patterns:
    - explicit payment adapter seams for browser/session/subscription-risk operations
    - accounts repository reuse as the single session/subscription truth source
    - bind-card task persistence as DB-backed state machine instead of jobs/websocket runtime
key-files:
  created:
    - backend-go/db/migrations/0007_init_payment_bind_card_tasks.sql
    - backend-go/internal/payment/types.go
    - backend-go/internal/payment/repository.go
    - backend-go/internal/payment/repository_postgres.go
    - backend-go/internal/payment/service.go
    - backend-go/internal/payment/http/handlers.go
  modified:
    - backend-go/db/migrations/phase4_payment_migration_test.go
    - backend-go/internal/payment/repository_postgres_test.go
    - backend-go/internal/payment/service_test.go
    - backend-go/internal/payment/http/handlers_test.go
key-decisions:
  - "Payment slice 继续以 bind_card_tasks 持久化状态机为核心，不引入 jobs/websocket 模型。"
  - "账号 session_token、cookies、subscription_type/subscription_at 全部继续走共享 accounts 真值源，不新增第二套 payment 账号存储。"
  - "浏览器自动绑卡、会话补全、订阅探测等高风险步骤统一收敛到显式 adapter seam，04-01 不在 handler 层拼 Python fallback。"
patterns-established:
  - "Payment handlers 统一返回现有 static-js 依赖的 JSON 字段，并把失败保持为 detail 语义。"
  - "低置信度 free 不清空已有付费态，高置信度 free 才允许回落到 free。"
requirements-completed: [PAY-01]
duration: 18min
completed: 2026-04-05
---

# Phase 4 Plan 01: Payment slice summary

**Go payment slice now owns bind-card task persistence, account session/subscription writeback rules, and unmounted `/api/payment/*` compatibility handlers for the existing payment and accounts pages.**

## Performance

- **Duration:** 18 min
- **Started:** 2026-04-05T15:32:59Z
- **Completed:** 2026-04-05T15:51:05Z
- **Tasks:** 2
- **Files modified:** 10

## Accomplishments

- 新建 `backend-go/internal/payment`，闭合了 bind-card task repository、payment service 状态机、adapter seams 和 compatibility DTO。
- 把 session save/bootstrap、mark-user-action、sync-subscription、manual mark-subscription、batch-check-subscription 的关键写回规则迁到 Go，并继续复用 accounts 真值源。
- 提供了未挂载的 `/api/payment/*` handlers 和 regression tests，锁定 `static/js/payment.js` 与 `static/js/accounts.js` 当前真实消费的路径、字段和 detail 错误语义。

## Task Commits

Each task was committed atomically:

1. **Task 1: 定义 payment 持久化合同与 Go service 状态机** - `d3cd381` (test), `6d039dc` (feat)
2. **Task 2: 提供未挂载的 payment compatibility handlers 与页面 consumer 回归测试** - `5f2e1f7` (test), `b5f5682` (feat)

## Files Created/Modified

- `backend-go/db/migrations/0007_init_payment_bind_card_tasks.sql` - 新增 payment 运行时 `bind_card_tasks` PostgreSQL 表和索引。
- `backend-go/db/migrations/phase4_payment_migration_test.go` - 锁定 payment migration 的核心 schema 合同。
- `backend-go/internal/payment/types.go` - 定义 bind-card task、session/subscription、handler contract 和 adapter seam 类型。
- `backend-go/internal/payment/repository.go` - 定义 payment repository 合同和基础归一化规则。
- `backend-go/internal/payment/repository_postgres.go` - 实现 bind-card task 的 create/get/list/update/delete PostgreSQL repository。
- `backend-go/internal/payment/repository_postgres_test.go` - 覆盖 repository 的字段 round-trip、过滤分页和生命周期更新。
- `backend-go/internal/payment/service.go` - 实现 payment 状态机、accounts 写回、subscription confidence gate 和高风险 adapter seams。
- `backend-go/internal/payment/service_test.go` - 覆盖 session save/bootstrap、mark-user-action、sync-subscription、auto-bind 状态流转。
- `backend-go/internal/payment/http/handlers.go` - 提供未挂载的 `/api/payment/*` 兼容 handlers。
- `backend-go/internal/payment/http/handlers_test.go` - 锁定 payment/session/bind-card/subscription 路由的请求解码和响应字段契约。

## Decisions Made

- 沿用 Python payment 领域的状态词汇和 bind-card task 表结构，而不是把 payment 强行改造成 Phase 2 的 queue/websocket 模式。
- payment service 直接复用 accounts repository 做 session/cookies/subscription 写回，以便继续遵守已存在的 account 数据合同。
- 对 generate-link、random-billing、session bootstrap/probe、subscription check、auto-bind local/third-party 统一引入显式接口，避免 handler 直接依赖 Python route 或浏览器细节。

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Payment migration renumbered from `0004` to `0007`**
- **Found during:** Task 1 (定义 payment 持久化合同与 Go service 状态机)
- **Issue:** 计划里写的是 `0004_init_payment_bind_card_tasks.sql`，但当前仓库已经存在 Phase 3 引入的 `0004`、`0005`、`0006` migration；继续使用 `0004` 会覆盖现有迁移历史并破坏工作区现状。
- **Fix:** 改为新增 `backend-go/db/migrations/0007_init_payment_bind_card_tasks.sql`，并用独立 migration test 锁定 schema 合同。
- **Files modified:** `backend-go/db/migrations/0007_init_payment_bind_card_tasks.sql`, `backend-go/db/migrations/phase4_payment_migration_test.go`
- **Verification:** `cd backend-go && go test ./db/migrations -run 'TestPaymentMigrationDefinesBindCardTasksCompatibilitySchema' -v`
- **Committed in:** `6d039dc`

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** 仅修正 migration 编号以适配当前仓库真实历史，没有扩展到 Team 或 router/bootstrap 集成范围。

## Issues Encountered

- 仓库内存在大量与 04-01 无关的 `backend-go` 脏改动和未跟踪文件。本计划只暂存 payment slice、migration 与对应测试文件，没有回滚或覆盖用户工作。

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 04-03 可以直接在 router/cmd/api 层挂载 payment handlers；核心 payment 领域逻辑和 contract tests 已经准备好。
- session/bootstrap、generate-link、subscription-check、auto-bind 等高风险路径已经被收敛到显式 adapter seams；后续接线时只需注入具体实现，不需要回到 handler 层改协议。
- 04-02 Team slice 仍是独立范围，本计划没有触碰 Team domain。

## Self-Check: PASSED

- FOUND: `.planning/phases/04-payment-and-team-domains/04-01-SUMMARY.md`
- FOUND: `d3cd381`
- FOUND: `6d039dc`
- FOUND: `5f2e1f7`
- FOUND: `b5f5682`

---
*Phase: 04-payment-and-team-domains*
*Completed: 2026-04-05*
