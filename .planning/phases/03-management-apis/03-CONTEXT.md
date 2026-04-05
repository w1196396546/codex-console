# Phase 3: Management APIs - Context

**Gathered:** 2026-04-05
**Status:** Ready for planning
**Mode:** Auto-generated (discuss skipped via workflow.skip_discuss)

<domain>
## Phase Boundary

Migrate the current account, settings, email-service, upload-config, proxy, and log management surfaces to Go while preserving the current UI contract. This phase owns admin/configuration APIs and the existing management pages that consume them. It does not migrate payment/bind-card flows or team workflows.

</domain>

<decisions>
## Implementation Decisions

### Compatibility boundary
- **D-01:** Current Python management APIs remain the compatibility oracle until their Go replacements prove parity.
- **D-02:** Existing templates and `static/js` pages must keep working without a frontend rewrite; backend contract preservation comes first.

### Scope boundary
- **D-03:** Phase 3 covers accounts management, settings, email services, upload-service configs, proxies, and logs.
- **D-04:** Payment/bind-card flows remain Phase 4 scope even when currently called from accounts or management pages.
- **D-05:** Team discovery/sync/invite/task flows remain Phase 4 scope even when current pages link to them.

### Implementation approach
- **D-06:** Extend existing Go `accounts`, registration-adjacent, and config/uploader persistence foundations instead of reintroducing Python-side orchestration into normal management paths.
- **D-07:** Preserve current field names, filtering semantics, export/import behavior, and operator-visible status strings unless a later phase explicitly changes them.

### the agent's Discretion
The agent may choose the exact decomposition across Go handlers/services/repositories and UI contract fixtures as long as the existing management pages and scripts remain backend-compatible and scope does not bleed into payment or team domains.

</decisions>

<specifics>
## Specific Ideas

No extra product changes beyond parity. Follow the route/client matrix and runtime/schema contracts established in Phase 1, and treat Phase 2 registration work as baseline that management pages must integrate with rather than rework.

</specifics>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project and phase scope
- `.planning/PROJECT.md` — Migration framing, validated baseline, and compatibility constraints.
- `.planning/REQUIREMENTS.md` — Phase 3 requirement IDs `MGMT-01`, `MGMT-02`, `CUT-01`.
- `.planning/ROADMAP.md` — Phase 3 goal, success criteria, and later-phase boundaries.
- `.planning/STATE.md` — Current project state and deferred validation context from Phase 2.

### Phase 1 and Phase 2 contracts
- `.planning/phases/01-compatibility-baseline/01-route-client-parity-matrix.md` — Current management route/client consumers and ownership targets.
- `.planning/phases/01-compatibility-baseline/01-shared-schema-contract.md` — Shared and Python-only storage contract relevant to management/config domains.
- `.planning/phases/01-compatibility-baseline/01-runtime-boundary-contract.md` — Current auth/runtime expectations that management APIs must preserve.
- `.planning/phases/02-native-registration-runtime/02-VERIFICATION.md` — Registration runtime is complete in code/automation but still has deferred staging validation.

### Current management implementation
- `src/web/routes/accounts.py` — Python account-management API behavior.
- `src/web/routes/settings.py` — Python settings, database, proxy, and configuration API behavior.
- `src/web/routes/email.py` — Python email-service management API behavior.
- `src/web/routes/logs.py` — Python log admin API behavior.
- `src/web/routes/upload/cpa_services.py` — Python CPA service-config API behavior.
- `src/web/routes/upload/sub2api_services.py` — Python Sub2API service-config API behavior.
- `src/web/routes/upload/tm_services.py` — Python Team Manager service-config API behavior.
- `backend-go/internal/accounts/` — Existing Go account baseline.
- `backend-go/internal/uploader/` — Existing Go uploader config/payload baseline.

### Current UI consumers
- `templates/accounts.html`, `static/js/accounts.js`
- `templates/accounts_overview.html`, `static/js/accounts_overview.js`, `static/js/accounts_state_actions.js`
- `templates/settings.html`, `static/js/settings.js`
- `templates/email_services.html`, `static/js/email_services.js`
- `templates/logs.html`, `static/js/logs.js`

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `backend-go/internal/accounts/` already provides list/upsert foundations and merge semantics for account data.
- `backend-go/internal/uploader/` already provides persisted uploader config reads plus sender/payload logic.
- Phase 1 route/client matrix already identifies which admin/config surfaces remain Python-owned.

### Established Patterns
- Go registration/runtime work now serves as the baseline style: explicit services, handlers, repositories, and e2e compatibility tests.
- Python management routes are still the oracle for filtering, export/import, and operator-visible field semantics.
- Existing frontend code is page-scoped JavaScript, not a component framework; backend contract drift shows up immediately in those scripts.

### Integration Points
- Management phase changes will connect current templates/static JS to Go-owned handlers under the same `/api/*` paths.
- Account list/detail/filter/upload actions sit close to `backend-go/internal/accounts/` and `backend-go/internal/uploader/`.
- Settings, email services, proxies, and logs likely need new Go domain slices or expansions around existing persistence/config readers.

</code_context>

<deferred>
## Deferred Ideas

- Payment/bind-card API migration remains Phase 4 scope.
- Team domain API migration remains Phase 4 scope.
- Final production cutover and external-environment verification remain Phase 5 scope.

</deferred>

---
*Phase: 03-management-apis*
*Context gathered: 2026-04-05*
