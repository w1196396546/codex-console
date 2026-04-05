---
phase: 03-management-apis
verified: 2026-04-05T14:44:27Z
status: human_needed
score: 7/7 must-haves verified
re_verification:
  previous_status: gaps_found
  previous_score: 4/7
  gaps_closed:
    - "现有 `/accounts-overview` 页面可通过 Go `/api/accounts/overview/refresh` 获取与 Python 兼容的真实配额刷新结果"
    - "现有 `/api/sub2api-services/upload` 管理动作可在真实 Go API 进程中工作"
  gaps_remaining: []
  regressions: []
human_verification:
  - test: "Accounts Overview real refresh"
    expected: "在 `/accounts-overview` 触发单卡和批量刷新时，真实 hourly/weekly/code review 配额会更新；若仍拿不到新配额，则页面进入 warning/failed 分支而不是 success。"
    why_human: "依赖真实 OpenAI 账号、令牌、网络环境和现有页面交互链路；自动化仅覆盖兼容语义与 handler/e2e 合同。"
  - test: "Settings database tools in real PostgreSQL"
    expected: "`/settings` 页面上的数据库备份、导入、清理按钮在真实 PostgreSQL 环境下可执行，导入前备份和结果提示符合当前页面语义。"
    why_human: "当前自动化验证了逻辑与 handler 合同，但没有在真实数据库环境执行文件导入/导出链路。"
  - test: "External provider management actions"
    expected: "tempmail/outlook 测试、CPA/Sub2API/TM test-connection、账号 upload 动作在真实第三方配置下成功或失败并返回当前页面预期的消息与写回结果。"
    why_human: "这些路径依赖真实外部服务和网络副作用，当前自动化只覆盖本地兼容 contract。"
---

# Phase 3: Management APIs Verification Report

**Phase Goal:** Migrate the current account, settings, email-service, upload-config, proxy, and log management surfaces to Go while preserving the current UI contract.
**Verified:** 2026-04-05T14:44:27Z
**Status:** human_needed
**Re-verification:** Yes — after gap closure

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | 现有 `/accounts` 与 `/accounts-overview` 页面可继续通过 Go 完成账户 CRUD、import/export、refresh/validate、manual upload 与 overview refresh 工作流。 | ✓ VERIFIED | [service.go](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/accounts/service.go#L695) 已在 refresh 中执行真实 overview fetch、unknown quota 判失败并写回；`TestServiceOverviewRefresh*` 与 `TestManagementAccountsOverviewRefreshCompatibility` 通过。 |
| 2 | 账户详情、tokens、cookies、current-account 与 team relation 只读字段继续保持当前字段名和响应 envelope。 | ✓ VERIFIED | `backend-go/internal/accounts/types.go`、`backend-go/internal/accounts/http/handlers.go` 与 `TestManagementAccountsCompatibilityEndpoints` 保持现有读侧 contract。 |
| 3 | `/settings` 页面依赖的 settings/proxies/database/tempmail 端点可由 Go 兼容提供。 | ✓ VERIFIED | `backend-go/internal/settings/http/handlers.go` 暴露 `/api/settings*` 路由族；用户提供的 settings/proxy/database 测试命令在当前 workspace 已通过。 |
| 4 | `/email-services` 页面与 settings 页内邮箱服务模块可继续调用 Go `/api/email-services*`。 | ✓ VERIFIED | `backend-go/internal/emailservices/http/handlers.go` 暴露 stats/types/list/full/CRUD/test/outlook batch；`TestManagementEmailServicesCompatibilityEndpoints` 通过。 |
| 5 | `/api/cpa-services*`、`/api/sub2api-services*`、`/api/tm-services*` 的配置管理与 direct-upload 动作保持兼容。 | ✓ VERIFIED | [repository_postgres.go](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/accounts/repository_postgres.go#L191) 提供 `UploadAccountStore` 所需读写；[main.go](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/cmd/api/main.go#L63) 注入 `accountsRepository`；`TestAPISub2APIUpload*` 和 `TestManagementUploaderCompatibilityEndpoints` 通过。 |
| 6 | `/logs` 页面可继续通过 Go `/api/logs*` 浏览、统计、清理和清空 app logs。 | ✓ VERIFIED | `backend-go/internal/logs/http/handlers.go` 暴露 `/api/logs*`；`TestManagementLogsCompatibilityEndpoints` 和 logs package 测试通过。 |
| 7 | 当前 templates/static JS 可以直接切到 Go 管理域而不需要前端重写，且 payment/team 仍保持 Phase 4 owner。 | ✓ VERIFIED | [router.go](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/http/router.go#L148) 挂载 accounts/settings/email/uploader/logs；`TestRouterLeavesPhaseFourRoutesUnmounted` 与 `TestManagementPhaseBoundaryExcludesPaymentAndTeamRoutes` 保持 payment/team 404 边界；静态 JS 仍指向既有 `/api/*` 路径。 |

**Score:** 7/7 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `backend-go/internal/accounts/http/handlers.go` | 暴露当前 `/api/accounts*` 兼容路由族 | ✓ VERIFIED | 含 CRUD、overview、tokens、cookies、manual upload、overview refresh 路由。 |
| `backend-go/internal/accounts/service.go` | 提供账户工作流核心语义 | ✓ VERIFIED | `RefreshOverview` 已做真实 fetch、写回和 unknown quota failure mapping。 |
| `backend-go/internal/settings/http/handlers.go` | 暴露 `/api/settings*` 兼容路由族 | ✓ VERIFIED | settings/proxies/database/tempmail 路由完整。 |
| `backend-go/internal/emailservices/http/handlers.go` | 暴露 `/api/email-services*` 兼容路由族 | ✓ VERIFIED | stats/types/list/full/CRUD/test/outlook batch 完整。 |
| `backend-go/internal/uploader/service.go` | 承担 upload-config 管理与 Sub2API direct upload 编排 | ✓ VERIFIED | direct-upload 已接上 `UploadAccountStore`，成功账号回写仍保留。 |
| `backend-go/internal/logs/http/handlers.go` | 暴露 `/api/logs*` 兼容路由族 | ✓ VERIFIED | list/stats/cleanup/clear 全部存在。 |
| `backend-go/internal/http/router.go` | 在现有 `/api/*` 路径挂载所有 Phase 3 管理域 | ✓ VERIFIED | accounts/settings/emailservices/uploader/logs 全部注册。 |
| `backend-go/cmd/api/main.go` | 构建并注入所有 Phase 3 management services | ✓ VERIFIED | settings/emailservices/uploader/logs/accounts 全部注入，uploader 已接入 account store。 |

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| `static/js/accounts_overview.js` | `/api/accounts/overview/refresh` | `api.post('/accounts/overview/refresh', ...)` | ✓ WIRED | `RefreshOverview` 现在返回 success/failed detail 语义，e2e 明确锁定 `plan_type` 与 `error` 分支。 |
| `backend-go/cmd/api/main.go` | `uploader.Service.UploadSub2API` | `newAPIUploaderService(..., accountsRepository)` | ✓ WIRED | bootstrap 已注入 `WithUploadAccountStore`，`cmd/api` route wiring 测试通过。 |
| `static/js/settings.js` | `/api/settings*` | `api.get/post/patch/delete(...)` | ✓ WIRED | settings/proxy/database/tempmail contract 有 handler 和测试覆盖。 |
| `static/js/email_services.js` | `/api/email-services*` 与 `/api/settings/tempmail` | `api.get/post/patch/delete(...)` | ✓ WIRED | emailservices slice 与 settings tempmail slice 协同存在，e2e 覆盖通过。 |
| `static/js/logs.js` | `/api/logs*` | `api.get/post/delete(...)` | ✓ WIRED | list/stats/cleanup/clear 路由与 payload 形状匹配。 |
| `backend-go/internal/http/router.go` | Phase 4 payment/team routes | unmounted 404 boundary | ✓ WIRED | router/e2e 测试都明确 payment/team 未被提前接管。 |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| --- | --- | --- | --- | --- |
| `backend-go/internal/accounts/service.go` | `extra_data["codex_overview"]` -> `hourly_quota/weekly_quota/code_review_quota` | `refreshAccountOverview()` -> `fetchCodexOverview()` | Yes | ✓ FLOWING |
| `backend-go/internal/uploader/service.go` | `UploadSub2API` account list + writeback IDs | `accountStore.ListUploadAccounts/MarkSub2APIUploaded` | Yes | ✓ FLOWING |
| `backend-go/internal/settings/service.go` | aggregate settings/proxy/database payloads | `settings` / `proxies` tables | Yes | ✓ FLOWING |
| `backend-go/internal/emailservices/service.go` | service list/full/stats/outlook projection | `email_services` + `settings` + registered-account lookup | Yes | ✓ FLOWING |
| `backend-go/internal/uploader/service.go` | CPA/Sub2API/TM config payloads | `cpa_services` / `sub2api_services` / `tm_services` tables | Yes | ✓ FLOWING |
| `backend-go/internal/logs/service.go` | logs/stats responses | `app_logs` table | Yes | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| Accounts overview refresh compatibility | `cd backend-go && go test ./internal/accounts -run 'Test.*Overview.*Refresh.*' -v` | PASS | ✓ PASS |
| Sub2API bootstrap wiring | `cd backend-go && go test ./cmd/api -run 'Test.*Sub2API.*Upload.*|TestAPISub2APIUploadWiring' -v` | PASS | ✓ PASS |
| Management router mounting and Phase 4 boundary | `cd backend-go && go test ./internal/http -run 'Test(Router|Management).*' -v` | PASS | ✓ PASS |
| Accounts + management consumer-contract e2e | `cd backend-go && go test ./tests/e2e -run 'Test(RecentAccountsCompatibilityEndpoint|Management).*' -v` | PASS | ✓ PASS |
| Settings/proxies/database compatibility | `cd backend-go && go test ./db/migrations ./internal/settings -run 'Test(Settings|Proxy).*' -v && go test ./internal/settings/http -run 'Test(SettingsHandler|Database|Proxy).*' -v` | PASS (user-provided current-workspace evidence) | ✓ PASS |
| Email-services compatibility | `cd backend-go && go test ./db/migrations ./internal/emailservices -run 'Test(EmailServices|Outlook).*' -v && go test ./internal/emailservices/http -run 'Test(EmailServices|Tempmail|Outlook).*' -v` | PASS (user-provided current-workspace evidence) | ✓ PASS |
| Upload-config and logs compatibility | `cd backend-go && go test ./internal/uploader -run 'Test(Upload|ServiceConfig|Builder|Sender).*' -v && go test ./internal/uploader/http -run 'Test.*Handler.*' -v && go test ./db/migrations ./internal/logs -run 'Test(Logs|Stats).*' -v && go test ./internal/logs/http -run 'Test(Logs|Cleanup|Clear).*' -v` | PASS (user-provided current-workspace evidence) | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| --- | --- | --- | --- | --- |
| `MGMT-01` | `03-01`, `03-06`, `03-07` | Operators can manage accounts through Go-owned APIs with current CRUD, import/export, refresh, validate, and upload workflows. | ✓ SATISFIED | accounts slice + e2e 覆盖到 CRUD/import/export/refresh/validate/upload，overview refresh blocker 已关闭。 |
| `MGMT-02` | `03-02`, `03-03`, `03-04`, `03-05`, `03-06`, `03-08` | Operators can manage settings, email services, upload-service configs, proxies, and logs through Go-owned APIs with current behavior. | ✓ SATISFIED | settings/emailservices/uploader/logs 都有 mounted routes、package tests 与 management e2e； Sub2API bootstrap 接线已闭环。 |
| `CUT-01` | `03-06`, `03-07`, `03-08` | Current templates and static JavaScript can target the Go backend for migrated domains without requiring a UI rewrite. | ? NEEDS HUMAN | 代码与 e2e contract 已满足，但真实页面和外部依赖仍需人工验收。 |

### Anti-Patterns Found

No blocker anti-patterns detected in the re-verified Phase 3 slices. A sweep over accounts/settings/emailservices/uploader/logs/router/cmd/api/e2e files did not surface TODO/placeholder/stub indicators that affect Phase 3 goal delivery.

### Human Verification Required

### 1. Accounts Overview Real Refresh

**Test:** 在现有 `/accounts-overview` 页面点击单卡刷新和批量刷新。
**Expected:** 有真实配额时显示刷新成功并更新卡片；没有拿到新配额时显示 warning/failed，而不是 success。
**Why human:** 需要真实 OpenAI 账号、令牌和浏览器交互链路。

### 2. Settings Database Tools

**Test:** 在真实 PostgreSQL 环境执行 `/settings` 页面的数据库备份、导入和清理。
**Expected:** 备份成功生成文件；导入前自动备份；清理结果和页面提示一致。
**Why human:** 自动化未执行真实文件导入/导出链路。

### 3. External Provider Actions

**Test:** 用真实配置验证 tempmail/outlook 测试、CPA/Sub2API/TM test-connection 和账号 upload。
**Expected:** 返回当前页面预期的成功/失败消息，并且上传写回状态正确。
**Why human:** 依赖真实第三方服务和网络副作用。

### Gaps Summary

上一版两个代码级 blocker 已关闭，当前没有发现新的 Phase 3 代码缺口，也没有回归。剩余工作是人工验收真实 UI 与外部依赖环境，因此本次状态为 `human_needed` 而不是 `passed`。

---

_Verified: 2026-04-05T14:44:27Z_
_Verifier: Claude (gsd-verifier)_
