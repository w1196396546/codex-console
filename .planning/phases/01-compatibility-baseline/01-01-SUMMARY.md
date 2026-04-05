---
phase: 01-compatibility-baseline
plan: "01"
subsystem: migration
tags: [compatibility, api, websocket, frontend-contract, inventory]
requires: []
provides:
  - Route parity inventory for current Python and Go backend surfaces
  - Domain-grouped route/client parity matrix with ownership targets
  - Explicit baseline-vs-remaining migration boundary for current clients
affects: [phase-02, phase-03, phase-04, cutover]
tech-stack:
  added: []
  patterns: [contract-first inventory, template-and-script consumer mapping]
key-files:
  created:
    - .planning/phases/01-compatibility-baseline/01-route-parity-inventory.json
    - .planning/phases/01-compatibility-baseline/01-route-client-parity-matrix.md
  modified: []
key-decisions:
  - "Treat current Python routes as the compatibility oracle while flagging current Go registration/accounts/jobs surfaces as baseline."
  - "List templates and static/js files as first-class compatibility consumers instead of inferring clients later from memory."
patterns-established:
  - "Parity artifacts distinguish go-baseline, shared-compat, and remaining-python ownership explicitly."
  - "Route inventory and review matrix are generated from tracked sources, then spot-checked for contract holes."
requirements-completed: [COMP-01, COMP-02]
duration: 35min
completed: 2026-04-05
---

# Phase 1: Compatibility Baseline Summary

**Route inventory and parity matrix now freeze the current Python-vs-Go backend surface, including first-class template/static-js consumers and baseline exclusions.**

## Performance

- **Duration:** 35 min
- **Started:** 2026-04-05T08:36:00Z
- **Completed:** 2026-04-05T09:10:57Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments

- Generated a machine-readable inventory covering `170` current `/api/*` and `/api/ws/*` compatibility records.
- Marked each surface as `remaining-python`, `shared-compat`, or `go-baseline` so later phases can see the true migration delta.
- Produced a domain-grouped parity matrix that names the exact template/static-js consumers for registration, accounts, settings, upload configs, payment, logs, and team domains.

## Task Commits

Each task was committed atomically:

1. **Task 1: 生成剩余 Python 路由与 Go 基线的机器可读清单** - `d07badc` (docs)
2. **Task 2: 输出面向后续阶段的路由与客户端 parity matrix** - `d07badc` (docs)

**Plan metadata:** `d07badc` (docs: freeze route parity baseline)

## Files Created/Modified

- `.planning/phases/01-compatibility-baseline/01-route-parity-inventory.json` - Machine-readable route ownership and parity-scope inventory
- `.planning/phases/01-compatibility-baseline/01-route-client-parity-matrix.md` - Domain-grouped matrix tying routes to current template/static-js consumers

## Decisions Made

- Treated Go registration/task/websocket surfaces and `/api/accounts` listing as baseline compatibility slices rather than Phase 1 reimplementation scope.
- Expanded the client-consumer scope to include root-path admin pages and helper scripts such as `email_services.js`, `logs.js`, `accounts_overview.js`, `accounts_state_actions.js`, `outlook_account_selector.js`, and `registration_log_buffer.js`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Contract Coverage] Root-path route decorators were initially omitted from the generated inventory**
- **Found during:** Task 1 (route inventory generation)
- **Issue:** The first generation pass missed `@router.get("")` and `@router.post("")` handlers, which would have dropped `/api/logs`, `/api/email-services`, `/api/accounts`, and several upload-config root endpoints from the parity baseline.
- **Fix:** Updated the generator pass to include empty-subpath decorators and regenerated both parity artifacts before committing them.
- **Files modified:** `.planning/phases/01-compatibility-baseline/01-route-parity-inventory.json`, `.planning/phases/01-compatibility-baseline/01-route-client-parity-matrix.md`
- **Verification:** Spot-check confirmed `/api/logs`, `/api/email-services`, and both `GET`/`POST` variants of `/api/accounts` are present with the expected ownership states.
- **Committed in:** `d07badc` (docs)

---

**Total deviations:** 1 auto-fixed (1 contract coverage)
**Impact on plan:** The fix tightened the parity baseline without changing scope. No business implementation work was pulled into Phase 1.

## Issues Encountered

- The repository's current client usage is distributed across page-local scripts plus helper modules, so route ownership could not be inferred safely from router files alone. This is now handled explicitly by the matrix.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Route/client parity facts are now explicit inputs for the shared schema/runtime contract and safety-rail work in `01-02` and `01-03`.
- Remaining high-risk domains are clearly isolated: management/config flows target Phase 3, and payment/team flows target Phase 4.

---
*Phase: 01-compatibility-baseline*
*Completed: 2026-04-05*
