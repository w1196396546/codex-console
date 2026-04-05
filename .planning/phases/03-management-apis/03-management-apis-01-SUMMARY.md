---
phase: 03-management-apis
plan: "01"
subsystem: api
tags: [accounts, compatibility, postgres, uploader, oauth, management]
requires:
  - phase: 01-compatibility-baseline
    provides: accounts route/client parity matrix and shared schema contracts for account fields
  - phase: 02-native-registration-runtime/02-03
    provides: Go-owned account persistence and uploader writeback baselines reused by management APIs
provides:
  - Go-owned `/api/accounts*` compatibility handlers for read/write management paths
  - account CRUD/import/export, token refresh/validate, overview card actions, and manual uploader dispatch inside the accounts slice
  - e2e and handler compatibility coverage for current accounts and accounts-overview pages
affects: [management-apis, cutover-and-decommission, accounts-overview, payment-team-mixed-pages]
tech-stack:
  added: []
  patterns:
    - handler-edge typed decode with per-route interface assertions to avoid router bootstrap changes
    - service-owned compatibility adapters over existing accounts persistence and uploader senders
    - accounts-slice local queries for cross-domain read needs when importing another slice would create cycles
key-files:
  created: []
  modified:
    - backend-go/internal/accounts/types.go
    - backend-go/internal/accounts/service.go
    - backend-go/internal/accounts/repository_postgres.go
    - backend-go/internal/accounts/http/handlers.go
    - backend-go/internal/accounts/service_test.go
    - backend-go/internal/accounts/http/handlers_test.go
    - backend-go/tests/e2e/accounts_flow_test.go
key-decisions:
  - "Kept `internal/http/router.go` untouched by making `accounts/http.Handler` accept the existing dependency object and assert write-side capabilities per route."
  - "Reused the existing Postgres-backed uploader config repository and sender implementations for CPA/Sub2API/TM actions instead of re-implementing transport logic in handlers."
  - "Resolved `inbox-code` config lookup inside the accounts slice with direct `email_services` reads to avoid an `accounts -> emailservices -> registration -> accounts` import cycle."
patterns-established:
  - "Accounts compatibility stays inside the accounts slice: handlers decode current JS payloads, services adapt Python semantics, repositories keep SQL isolated."
  - "Write-side actions that touch external targets return compatibility envelopes while persisting CPA/Sub2API writeback flags through the existing account upsert path."
requirements-completed: [MGMT-01]
duration: 37m
completed: 2026-04-05
---

# Phase 3 Plan 01: Accounts Management API Summary

**Go-owned accounts compatibility layer covering CRUD/import/export, token maintenance, overview card actions, inbox code lookup, and CPA/Sub2API/TM manual uploads under the existing `/api/accounts*` surface**

## Performance

- **Duration:** 37m
- **Started:** 2026-04-05T13:09:40Z
- **Completed:** 2026-04-05T13:47:04Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments

- Extended the Go `accounts` slice from a list-only baseline to the current page-compatible read surface, including `/current`, `/stats/*`, `/overview/cards*`, account detail, tokens, and cookies responses.
- Added the write-side management paths the current pages call today: manual create/import, patch/delete, batch state/delete, exports, token refresh/validate, overview card mutations, manual uploads, and inbox-code lookup.
- Locked the compatibility surface with TDD-style handler/e2e tests while leaving router wiring and `cmd/api` bootstrap for `03-06` as required by scope.

## Task Commits

Each task was committed atomically:

1. **Task 1: 扩展 accounts 读侧合同、查询过滤与 overview/current 兼容端点** - `07da62d` (test), `019936e` (feat)
2. **Task 2: 补齐 accounts 写侧、导入导出、刷新校验与手动上传兼容端点** - `0a45028` (test), `47b9404` (feat)

_Note: Both tasks followed the plan’s TDD flow with paired RED → GREEN commits._

## Files Created/Modified

- `backend-go/internal/accounts/types.go` - Added compatibility DTOs and typed request/response contracts for read/write accounts routes.
- `backend-go/internal/accounts/service.go` - Implemented read/write orchestration, exports, token maintenance, overview mutations, uploader dispatch, and inbox-code lookup.
- `backend-go/internal/accounts/repository_postgres.go` - Added filtered account queries, selection helpers, current-account lookup, stats aggregation, and delete support.
- `backend-go/internal/accounts/http/handlers.go` - Registered and decoded the full accounts compatibility route family without changing router bootstrap.
- `backend-go/internal/accounts/service_test.go` - Extended service-level coverage for normalized list filters and repository test doubles.
- `backend-go/internal/accounts/http/handlers_test.go` - Added compatibility tests for read and write routes, payload decode, and response envelopes.
- `backend-go/tests/e2e/accounts_flow_test.go` - Extended e2e coverage to include write-side route mounting and export attachment semantics.

## Decisions Made

- Preserved the router/bootstrap boundary by keeping all new capability discovery inside `accounts/http.Handler` rather than widening `internal/http/router.go` before `03-06`.
- Reused `uploader.NewPostgresConfigRepository` + `uploader.NewSender` directly from the accounts service so upload semantics stay aligned with existing CPA/Sub2API/TM protocols.
- Kept `inbox-code` inside 03-01 scope by reading enabled `email_services` rows directly from Postgres and feeding the native mail providers, instead of pulling email-service management wiring forward.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Removed an `accounts -> emailservices -> registration -> accounts` import cycle**
- **Found during:** Task 2 (write-side actions and inbox-code implementation)
- **Issue:** Reusing the email-services repository directly from `accounts` introduced a compile-time cycle through `registration`.
- **Fix:** Replaced that dependency with a local `email_services` SQL reader inside the accounts slice and kept the mail-provider reuse.
- **Files modified:** `backend-go/internal/accounts/service.go`
- **Verification:** `go test ./internal/accounts ./internal/accounts/http ./tests/e2e -run 'Test(Accounts|RecentAccounts).*' -v`
- **Committed in:** `47b9404`

**2. [Rule 1 - Bug] URL-encoded the OAuth refresh form body**
- **Found during:** Task 2 (token refresh implementation)
- **Issue:** The initial refresh request body concatenated `redirect_uri` and token values without form encoding, which could corrupt refresh requests.
- **Fix:** Switched the request body construction to `url.Values.Encode()`.
- **Files modified:** `backend-go/internal/accounts/service.go`
- **Verification:** `go test ./internal/accounts ./internal/accounts/http -run 'Test(Accounts|Service).*' -v`
- **Committed in:** `47b9404`

---

**Total deviations:** 2 auto-fixed (1 blocking, 1 bug)
**Impact on plan:** Both deviations were required for correctness and to keep the work inside 03-01 scope. No router/bootstrap or payment/team scope creep was introduced.

## Known Stubs

- `backend-go/internal/accounts/service.go:683` - `RefreshOverview` currently refreshes cards through the Go fallback/cache path rather than a full native remote quota fetcher. The route shape is compatible and the page continues to function, but quota data may remain `unknown` until a deeper native overview fetch implementation lands.

## Issues Encountered

- The workspace already contained unrelated `backend-go/` changes. Only the 03-01 accounts files and tests listed above were staged into the plan commits.

## User Setup Required

None - no external service configuration required beyond whatever uploader/email service rows already exist in the current database.

## Next Phase Readiness

- `03-06` can wire the expanded accounts slice into the final API/bootstrap closure without additional accounts contract work.
- Payment/team routes remain on their current owner, as required by the phase boundary.
- If stricter `/accounts-overview` parity is needed before cutover, the remaining gap is native remote quota refresh depth rather than route shape or request decoding.

## Self-Check: PASSED

- FOUND: `.planning/phases/03-management-apis/03-management-apis-01-SUMMARY.md`
- FOUND: `07da62d`
- FOUND: `019936e`
- FOUND: `0a45028`
- FOUND: `47b9404`

---
*Phase: 03-management-apis*
*Completed: 2026-04-05*
