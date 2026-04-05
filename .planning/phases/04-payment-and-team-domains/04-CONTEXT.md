# Phase 4: Payment and Team Domains - Context

**Gathered:** 2026-04-05
**Status:** Ready for planning
**Mode:** Auto-generated (discuss skipped via workflow.skip_discuss)

<domain>
## Phase Boundary

Migrate the remaining Python-only payment/bind-card and team workflows to Go with compatible runtime and persistence behavior. This phase owns payment/session/bootstrap/subscription-sync flows plus team discovery, sync, invite, membership, and team-task APIs. It does not redesign the frontend and does not perform the final production cutover.

</domain>

<decisions>
## Implementation Decisions

### Compatibility boundary
- **D-01:** Current Python payment and team route behavior remains the compatibility oracle until Go parity is proven.
- **D-02:** Existing templates and `static/js` consumers must keep working without a frontend rewrite; preserve current `/api/payment*` and `/api/team*` contracts first.

### Scope boundary
- **D-03:** Phase 4 covers payment, bind-card tasks, subscription sync, and all team discovery/sync/invite/membership/task flows.
- **D-04:** Phase 4 must build on the completed Phase 2 runtime semantics and completed Phase 3 management/API wiring, not rework them.
- **D-05:** Final production cutover, rollback, and operator runbooks remain Phase 5 scope even if Phase 4 closes the remaining backend domain gaps.

### Implementation approach
- **D-06:** Reuse existing Go registration/accounts/uploader foundations and add the missing payment/team domain slices rather than copying Python monolith structure.
- **D-07:** Preserve current operator-facing task/session/status semantics, especially around bind-card task lifecycle and team task accepted-response flows.

### the agent's Discretion
The agent may choose the exact decomposition across payment and team slices, persistence adapters, and compatibility fixtures as long as the existing UI and API contract remain stable and no Phase 5 cutover work is pulled forward.

</decisions>

<specifics>
## Specific Ideas

No extra product changes beyond parity. Use the Phase 1 route/client matrix plus the completed Phase 2 and Phase 3 summaries as the baseline for what already moved to Go, then focus only on the remaining payment and team domain deltas.

</specifics>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project and phase scope
- `.planning/PROJECT.md`
- `.planning/REQUIREMENTS.md`
- `.planning/ROADMAP.md`
- `.planning/STATE.md`

### Prior phase outputs
- `.planning/phases/01-compatibility-baseline/01-route-client-parity-matrix.md`
- `.planning/phases/01-compatibility-baseline/01-shared-schema-contract.md`
- `.planning/phases/01-compatibility-baseline/01-runtime-boundary-contract.md`
- `.planning/phases/02-native-registration-runtime/02-VERIFICATION.md`
- `.planning/phases/03-management-apis/03-VERIFICATION.md`

### Current payment and team implementation
- `src/web/routes/payment.py`
- `src/web/routes/team.py`
- `src/web/routes/team_tasks.py`
- `src/database/models.py`
- `src/database/team_models.py`
- `src/database/team_crud.py`
- `src/services/team/`
- `static/js/payment.js`
- `static/js/auto_team.js`
- `templates/payment.html`
- `templates/auto_team.html`

### Existing Go foundations to extend
- `backend-go/internal/accounts/`
- `backend-go/internal/uploader/`
- `backend-go/internal/registration/`
- `backend-go/internal/http/`
- `backend-go/cmd/api/main.go`

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Phase 2 already established Go runtime semantics for tasks, batches, websockets, persistence, and uploader side effects.
- Phase 3 already moved management/config/admin APIs to Go and preserved current UI contracts.
- `backend-go/internal/uploader/` and `backend-go/internal/accounts/` already provide useful primitives for payment/team side effects.

### Established Patterns
- Current Go work favors service + repository + handler layering with compatibility tests at unit and e2e levels.
- Python payment/team behavior is still the oracle for operator-visible task/session/status behavior.
- Existing frontend is page-scoped JavaScript, so route/field/status drift shows up immediately.

### Integration Points
- Payment routes are still consumed from `static/js/payment.js` and some account-page actions in `static/js/accounts.js`.
- Team routes are still consumed from `static/js/auto_team.js` and link-outs from accounts pages.
- Final API mount ownership will still be completed through the existing Go router/bootstrap pattern already used in Phase 3.

</code_context>

<deferred>
## Deferred Ideas

- Final production cutover and rollback choreography remain Phase 5.
- Any schema cleanup or contract simplification after parity remains Phase 5+.

</deferred>

---
*Phase: 04-payment-and-team-domains*
*Context gathered: 2026-04-05*
