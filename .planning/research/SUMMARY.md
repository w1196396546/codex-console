# Project Research Summary

**Project:** Codex Console Go Migration
**Domain:** Brownfield Python-to-Go backend migration
**Researched:** 2026-04-05
**Confidence:** HIGH

## Executive Summary

Codex Console is not a greenfield Go service. It is an existing Python product with a partial Go backend already in place. That means the right migration strategy is not "design the best new system", but "finish the delta safely". The migration should converge on the current Go stack in `backend-go/` while using the existing Python routes, templates, and data behavior as the compatibility oracle.

The recommended approach is a strangler migration with explicit route, data, and workflow parity at every step. The highest-risk mistake would be treating partial Go coverage as proof that the remaining domains can move without first freezing the current contract. Payment, team, and admin/configuration domains still sit on the Python side, and the production value of this migration only appears when the Python backend is no longer on the critical path.

## Key Findings

### Recommended Stack

The current Go stack is already the correct destination: Go 1.25, Chi, PostgreSQL via pgx/sqlc/goose, and Redis/Asynq for long-running workflows. The migration should extend the existing `backend-go/` runtime instead of creating another runtime or a clean-room rewrite.

**Core technologies:**
- Go: final backend runtime - already established in `backend-go/`
- PostgreSQL: durable source of truth - already used by the Go side for accounts and service configs
- Redis/Asynq: task orchestration - already better aligned than Python's process-local task state

### Expected Features

This migration project's table stakes are route/payload parity, native Go ownership of registration/runtime semantics, migration of management/admin APIs, and migration of payment/team domains. The valuable differentiators are the native Go runner, durable job orchestration, and an explicit compatibility regression harness. UI rewrite, schema redesign, and indefinite dual writes are anti-features for this milestone.

**Must have (table stakes):**
- Compatibility contract for the remaining Python-owned backend APIs
- Native Go registration/runtime parity
- Management/admin API parity
- Payment and team domain parity
- Final Go cutover with Python backend retirement plan

**Should have (competitive):**
- Durable task orchestration with explicit parity checks
- Native Go execution for flows currently hidden behind Python runtime behavior

**Defer (v2+):**
- UI/runtime consolidation beyond the current templates/static JS
- Schema cleanup unrelated to migration safety

### Architecture Approach

The target architecture is a compatibility facade over domain-oriented Go packages. Keep current clients unchanged, translate compatibility DTOs at the edge, run long-lived work through Go jobs/workers, and consolidate durable state in PostgreSQL/Redis. Any short-lived Python fallback must be isolated as an explicit adapter with a planned retirement phase.

**Major components:**
1. Compatibility HTTP layer - preserve current `/api/*` and `/api/ws/*` contracts
2. Domain services - own registration, management, payment, and team logic
3. Durable execution layer - replace process-local Python runtime semantics with Go job orchestration

### Critical Pitfalls

1. **Route contract drift** - prevent with a parity matrix and contract fixtures before each cutover
2. **Shared data drift** - freeze the data contract before moving more domains
3. **Hidden frontend coupling** - treat current templates/static JS as first-class migration consumers
4. **Permanent Python bridge** - make bridge removal explicit scope, not implied future cleanup
5. **Domain migration without runtime semantics** - migrate task/session/side-effect behavior, not only CRUD endpoints

## Implications for Roadmap

Based on research, suggested phase structure:

### Phase 1: Compatibility Baseline
**Rationale:** Migration cannot proceed safely until the remaining Python-owned contract is explicit.
**Delivers:** Route/data parity matrix, shared schema/runtime rules, and migration safety rails.
**Addresses:** Compatibility and data-contract requirements.
**Avoids:** Route contract drift and shared data drift.

### Phase 2: Native Registration Runtime
**Rationale:** Registration is already partially in Go and is the highest-value place to remove Python from the critical path.
**Delivers:** Native Go ownership of registration, batch, Outlook batch, and task lifecycle parity.
**Uses:** Existing `registration/`, `jobs/`, and `nativerunner/` foundations.
**Implements:** Durable runtime behavior instead of Python bridge reliance.

### Phase 3: Management APIs
**Rationale:** The current UI cannot move off Python until accounts, settings, upload configs, proxies, and logs are available through Go.
**Delivers:** Go-owned admin/configuration domains with compatible UI-facing APIs.
**Implements:** Contract-preserving management services and UI cutover readiness.

### Phase 4: Payment and Team Domains
**Rationale:** These are still entirely Python-owned and contain hidden runtime/side-effect complexity.
**Delivers:** Go-owned payment/bind-card and team workflows with current behavior preserved.
**Avoids:** The false completion state where CRUD moves but runtime semantics do not.

### Phase 5: Cutover and Decommission
**Rationale:** The migration only creates value once Python is off the production critical path.
**Delivers:** Production cutover, rollback plan, compatibility evidence, and Python backend retirement.

### Phase Ordering Rationale

- Compatibility and shared data rules must come before domain migration.
- Registration/runtime comes before management because Go already has strong foundations there.
- Payment/team come after management because they depend on the compatibility rails and admin/config surfaces.
- Cutover happens last because it depends on verified parity across all in-scope domains.

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 2:** native OpenAI registration runtime details and parity edge cases
- **Phase 4:** payment/bind-card/browser automation behavior and team-task semantics

Phases with standard patterns (skip research-phase):
- **Phase 1:** compatibility contract and migration harness setup
- **Phase 3:** domain-oriented CRUD/admin API migration patterns

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Based on current repo reality rather than speculative future tooling |
| Features | HIGH | Directly derived from route/domain scans across Python and Go |
| Architecture | HIGH | Strongly anchored to the current split-runtime codebase |
| Pitfalls | HIGH | Consistent with the known current concerns and migration gap pattern |

**Overall confidence:** HIGH

### Gaps to Address

- Payment and team runtime details need deeper phase-level planning because they are still Python-only.
- Current frontend-to-endpoint coupling should be made explicit in Phase 1 so later cutover work stays bounded.

## Sources

### Primary (HIGH confidence)
- `.planning/codebase/ARCHITECTURE.md` - current split-runtime structure
- `.planning/codebase/CONCERNS.md` - current migration and parity risks
- `backend-go/README.md` - explicit Go scope and not-yet-migrated areas

### Secondary (MEDIUM confidence)
- Route scans across `src/web/routes/` and `backend-go/internal/**/http/*.go`

### Tertiary (LOW confidence)
- None - this summary is grounded in local codebase analysis rather than external trend research

---
*Research completed: 2026-04-05*
*Ready for roadmap: yes*
