---
phase: 03-management-apis
plan: "04"
subsystem: api
tags: [go, postgres, uploader, compatibility, chi]
requires:
  - phase: 02-native-registration-runtime
    provides: uploader builders/senders plus Go-owned account upload writeback semantics
provides:
  - uploader admin CRUD service and Postgres write-side contracts for CPA/Sub2API/TM configs
  - compatibility HTTP handlers for `/api/cpa-services*`, `/api/sub2api-services*`, and `/api/tm-services*`
  - Go-owned Sub2API direct upload/test-connection actions that reuse existing uploader sender/client foundations
affects: [03-06 management router wiring, Phase 5 cutover, uploader compatibility verification]
tech-stack:
  added: []
  patterns: [generic uploader admin repository over existing service rows, compatibility DTO mapping with secret-vs-full splits, sender/client reuse for external test and upload actions]
key-files:
  created:
    - backend-go/internal/uploader/service.go
    - backend-go/internal/uploader/http/handlers.go
  modified:
    - backend-go/internal/uploader/types.go
    - backend-go/internal/uploader/repository_postgres.go
    - backend-go/internal/uploader/repository_postgres_test.go
    - backend-go/internal/uploader/service_test.go
    - backend-go/internal/uploader/http/handlers_test.go
key-decisions:
  - "在 uploader 包内扩展通用 AdminRepository 和兼容 DTO，避免另起 upload-admin 包。"
  - "连接测试和 Sub2API 直传动作放在 uploader service 内，并复用既有 sender/client 工具，不让 handler 直接拼外部 HTTP。"
  - "Sub2API 直传仅对成功账号写回 `sub2api_uploaded`，同时保持 Python 的 success/failed/skipped/detail 结果形状。"
patterns-established:
  - "Compatibility DTO split: list/get 只暴露 has_token/has_key，`/full` 才返回真实密钥。"
  - "Uploader actions: handler 只做解码和 detail 错误映射，service 负责选择配置、探测外部服务和发送上传。"
requirements-completed: [MGMT-02]
duration: 16m
completed: 2026-04-05
---

# Phase 3 Plan 04: Upload-config Management APIs Summary

**Go 侧 `uploader` 现在可以兼容管理 CPA/Sub2API/TM 配置、连接测试和 Sub2API 直传动作，同时继续复用现有 sender/builder/client 基础。**

## Performance

- **Duration:** 16m
- **Started:** 2026-04-05T13:16:14Z
- **Completed:** 2026-04-05T13:31:48Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments
- 为 `uploader` 增加了 Go-owned admin service、兼容 DTO 和 Postgres 写侧 CRUD/full 合同，覆盖 CPA/Sub2API/TM 三类服务配置。
- 增加了 `internal/uploader/http` 兼容 handler，保留 `/full`、`/{id}/test`、`test-connection`、`/upload` 路径与 plain-array/detail 错误体语义。
- 让 Sub2API 直传继续复用现有 sender/builder/client，并在 Go 内保持 Python 当前的 success/failed/skipped/detail 结果形状和成功写回语义。

## Task Commits

Each task was committed atomically:

1. **Task 1: uploader admin config contracts/service/repository** - `b365275` (test), `4c5558e` (feat)
2. **Task 2: upload-config compatibility handlers/actions** - `c5a4b2f` (test), `4bb5611` (feat)

## Files Created/Modified

- `backend-go/internal/uploader/types.go` - uploader admin DTO、连接测试/直传动作请求结果类型与 store/repository 合同
- `backend-go/internal/uploader/repository_postgres.go` - uploader Postgres admin CRUD/full 查询与 kind-specific SQL
- `backend-go/internal/uploader/repository_postgres_test.go` - uploader repo 读写兼容测试，并将测试前缀对齐到计划验证正则
- `backend-go/internal/uploader/service.go` - uploader admin orchestration、连接测试和 Sub2API 直传服务逻辑
- `backend-go/internal/uploader/service_test.go` - uploader service 兼容语义测试、外部探测请求测试、直传写回测试
- `backend-go/internal/uploader/http/handlers.go` - CPA/Sub2API/TM 兼容 handler 与 JSON `detail` 错误映射
- `backend-go/internal/uploader/http/handlers_test.go` - upload-config 路由兼容性与 handler 错误体测试

## Decisions Made

- 在 `uploader` 包内部增加通用 admin repository/service，而不是创建新的 upload-admin slice，保持 payload builder/sender 的单一真值源。
- `test`、`test-connection` 和 `/api/sub2api-services/upload` 全部经 `uploader` service 复用 `client.go`/`sender.go`，避免在 handler 层复制目标协议。
- `/api/sub2api-services/upload` 维持 Python 当前的批量结果结构，并且只对真实上传成功的账号回写 `sub2api_uploaded`。

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] 调整 uploader repo 测试前缀以匹配计划验证正则**
- **Found during:** Task 1
- **Issue:** 原有 repo 测试名称不匹配 `Test(Upload|ServiceConfig|Builder|Sender).*`，会让新的 Postgres 写侧兼容测试绕过计划验证命令。
- **Fix:** 将 uploader repo 测试统一改为 `TestServiceConfig...` 前缀，让计划的官方验证命令实际覆盖 repo 读写合同。
- **Files modified:** `backend-go/internal/uploader/repository_postgres_test.go`
- **Verification:** `cd backend-go && go test ./internal/uploader -run 'Test(Upload|ServiceConfig|Builder|Sender).*' -v`
- **Committed in:** `4c5558e`

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** 仅修正验证覆盖范围，没有引入额外功能或越界改动。

## Issues Encountered

- 计划里的第二条验证命令 `go test ./internal/uploader -run 'Test.*Handler.*' -v` 只会测试父包，`internal/uploader/http` 子包会显示 `no tests to run`。为确认真实 handler 契约，额外执行了 `cd backend-go && go test ./internal/uploader/http -run 'Test.*Handler.*' -v`。

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `internal/uploader/http` handler 已准备好由 03-06 挂到管理 router，不需要再回退到 Python upload-config CRUD。
- 03-06 需要做的只剩最终 router wiring 与页面级整体验证；当前计划没有触碰 accounts router integration，符合边界要求。

---
*Phase: 03-management-apis*
*Completed: 2026-04-05*

## Self-Check: PASSED

- FOUND: `.planning/phases/03-management-apis/03-management-apis-04-SUMMARY.md`
- FOUND: `b365275`
- FOUND: `4c5558e`
- FOUND: `c5a4b2f`
- FOUND: `4bb5611`
