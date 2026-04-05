---
phase: 03-management-apis
plan: "08"
subsystem: api
tags: [go, accounts, uploader, sub2api, postgres, router]
requires:
  - phase: 03-management-apis/03-01
    provides: accounts slice Postgres repository and compatibility account models reused as upload-account store
  - phase: 03-management-apis/03-04
    provides: uploader direct-upload service semantics and success-only sub2api writeback contract
  - phase: 03-management-apis/03-06
    provides: mounted management router paths and cmd/api bootstrap baseline for management slices
provides:
  - accounts-backed `uploader.UploadAccountStore` implementation on the existing Postgres repository
  - cmd/api uploader bootstrap that injects the shared accounts repository into `/api/sub2api-services/upload`
  - bootstrap-level regression coverage for mounted `/api/sub2api-services/upload` compatibility responses
affects: [management-apis, phase-05-cutover, verification, sub2api-upload]
tech-stack:
  added: []
  patterns:
    - shared repository reuse between accounts slice and uploader direct-upload writeback
    - cmd/api-local builder helpers for bootstrap wiring tests without changing router ownership boundaries
key-files:
  created:
    - backend-go/cmd/api/main_test.go
  modified:
    - backend-go/internal/accounts/repository_postgres.go
    - backend-go/internal/accounts/repository_postgres_test.go
    - backend-go/cmd/api/main.go
key-decisions:
  - "复用现有 `accountsRepository` 同时提供 accounts slice 与 uploader `UploadAccountStore`，避免新增第二套 upload-account repository。"
  - "把 bootstrap 可测性收敛到 `cmd/api` 自己的小 helper，而不是改 router 结构或退回 handler fake 测试。"
patterns-established:
  - "Sub2API direct upload 的账号读取/写回继续走 accounts Postgres 真值源，并只对成功账号回写 `sub2api_uploaded`。"
  - "cmd/api 的 mounted route 回归测试优先经过真实 service wiring + router，再用 fake repo/sender 隔离外部依赖。"
requirements-completed: [MGMT-02, CUT-01]
duration: 8min
completed: 2026-04-05
---

# Phase 3 Plan 08: Sub2API upload bootstrap wiring summary

**`/api/sub2api-services/upload` 现在通过 cmd/api 的真实 bootstrap 接上 accounts-backed upload store，并有 router 级回归测试锁定 success/failed/skipped/detail 兼容响应。**

## Performance

- **Duration:** 8 min
- **Started:** 2026-04-05T14:26:33Z
- **Completed:** 2026-04-05T14:34:49Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- 为 `accounts.PostgresRepository` 补上 `uploader.UploadAccountStore` 所需的账号读取与 `sub2api_uploaded` 写回实现。
- 在 `cmd/api` 中通过 `newAPIUploaderService(...)` 显式注入共享 `accountsRepository`，修复真实 API 进程里 `/api/sub2api-services/upload` 的缺依赖问题。
- 新增 `cmd/api` 自身的 mounted route 回归测试，直接验证 `/api/sub2api-services/upload` 的兼容结果结构和成功账号写回。

## Task Commits

Each task was committed atomically:

1. **Task 1: 为 uploader direct-upload 接上真实 accounts-backed store** - `2cc7b83` (test), `d644f6d` (feat)
2. **Task 2: 用真实 bootstrap/router 回归测试锁定 `/api/sub2api-services/upload` 接线** - `31b7e04` (test), `10245d7` (feat)

_Note: Both tasks followed the plan’s TDD flow with paired RED → GREEN commits._

## Files Created/Modified

- `backend-go/internal/accounts/repository_postgres.go` - 实现 `ListUploadAccounts` 与 `MarkSub2APIUploaded`，让 uploader 复用 accounts 表作为 direct-upload 读写源。
- `backend-go/internal/accounts/repository_postgres_test.go` - 增加 uploader store 合同测试，锁定账号读取字段与成功账号写回 SQL 约束。
- `backend-go/cmd/api/main.go` - 增加 `newAPIUploaderService` 与 `newAPIHandler` 小型 bootstrap helper，并在主流程显式注入 accounts-backed upload store。
- `backend-go/cmd/api/main_test.go` - 增加 builder 测试和 mounted `/api/sub2api-services/upload` 回归测试，验证真实 router wiring、兼容计数和成功写回。

## Decisions Made

- 继续把 direct-upload 的账号真值源放在 accounts slice，不引入额外 repository 或新的表访问层。
- 让 `cmd/api` 自己暴露最小 wiring helper 给测试复用，这样能测到真实挂载链路，同时不扩大 Phase 3 的 router ownership。

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] 对齐 `cmd/api` 测试命名以命中计划验证正则**
- **Found during:** Task 1
- **Issue:** 初版 builder 测试名称未命中 `go test ./cmd/api -run 'Test.*Sub2API.*Upload.*' -v`，会导致验证命令出现 `no tests to run`。
- **Fix:** 将测试名调整为 `TestAPISub2APIUploadServiceInjectsUploadAccountStore`，确保计划验证命令真实覆盖 `cmd/api` 接线。
- **Files modified:** `backend-go/cmd/api/main_test.go`
- **Verification:** `cd backend-go && go test ./cmd/api -run 'Test.*Sub2API.*Upload.*' -v`
- **Committed in:** `d644f6d`

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** 仅修正验证覆盖范围，没有扩展 03-08 范围，也没有触碰 payment/team 或 uploader 其他管理能力。

## Issues Encountered

- 提交过程中两次遇到瞬时 `.git/index.lock` 残留；确认没有活动 git 进程且锁文件已自动消失后直接重试提交，未对仓库做额外清理。
- 仓库里存在大量与 03-08 无关的 `backend-go/` 脏改动。本计划只暂存并提交上述 4 个目标文件。

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Phase 3 的 Verification blocker 2 已关闭，真实 Go API 进程中的 `/api/sub2api-services/upload` 不再因缺少 `UploadAccountStore` 直接失败。
- 03-VERIFICATION 里仍剩 accounts overview refresh 语义缺口，属于 03-08 范围外问题，不在本计划内扩展处理。

## Self-Check: PASSED

- FOUND: `.planning/phases/03-management-apis/03-management-apis-08-SUMMARY.md`
- FOUND: `2cc7b83`
- FOUND: `d644f6d`
- FOUND: `31b7e04`
- FOUND: `10245d7`
