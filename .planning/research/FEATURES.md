# Feature Research

**Domain:** Brownfield Python-to-Go backend migration for Codex Console
**Researched:** 2026-04-05
**Confidence:** HIGH

## Feature Landscape

### Table Stakes (Users Expect These)

Features users assume still work after the backend moves from Python to Go.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Route and payload parity for current `/api/*` flows | Existing templates, static JS, and operator workflows already depend on these routes | HIGH | This is the migration contract, not an optional nicety |
| Registration start, batch, Outlook batch, and task lifecycle parity | Registration is the product's core operational flow | HIGH | Go already covers part of this surface, but the lifecycle is not fully complete |
| Account/admin workflow parity | Operators rely on import/export, refresh, validate, upload, and settings management today | HIGH | Python still owns most of this surface |
| Payment and bind-card workflow parity | Payment/session/bootstrap/bind-card flows are live operator capabilities | HIGH | Still Python-only in the current repo |
| Team workflow parity | Discovery, sync, invite, and membership management are already present in the product | HIGH | Still Python-only in the current repo |

### Differentiators (Competitive Advantage)

Features that make the migrated Go backend materially better than the legacy Python backend.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Native Go registration runner | Removes the Python bridge from the critical path | HIGH | Existing `nativerunner` foundation makes this achievable |
| Durable job orchestration with PostgreSQL/Redis | Replaces Python's process-local runtime maps with recoverable worker execution | MEDIUM | Strong operational win once parity is preserved |
| Explicit compatibility regression suite | Makes route/data drift visible before cutover | MEDIUM | Critical for safe strangler migration |

### Anti-Features (Commonly Requested, Often Problematic)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Rewrite the UI while migrating the backend | Feels like a chance to clean everything up at once | Doubles the number of moving parts and hides backend regressions | Keep current templates/static JS until Go parity is proven |
| Redesign data models during migration | Seems like a good time to "fix" awkward schemas | Breaks stored data compatibility and complicates rollback | Preserve contracts now, schedule cleanup later |
| Keep dual writes indefinitely | Looks like a safe fallback | Creates drift and makes the source of truth ambiguous | Use time-boxed transition adapters with an explicit retirement phase |

## Feature Dependencies

```text
Compatibility contract
    -> shared schema/runtime rules
        -> native registration runtime parity
            -> management API migration
                -> payment/team migration
                    -> production cutover
```

### Dependency Notes

- **Management API migration requires shared schema/runtime rules:** admin endpoints cannot move safely until stored-field compatibility is pinned down.
- **Payment/team migration requires the compatibility contract:** these domains have the most hidden coupling to current route semantics and side effects.
- **Production cutover requires every domain migration:** retiring Python too early would turn missing parity into an outage.

## MVP Definition

### Launch With (v1)

Minimum migration milestone that counts as "Python backend replaced by Go" for this project.

- [ ] Route and payload compatibility for all in-scope backend APIs - without this, the current clients break
- [ ] Native Go ownership of registration/task runtime - this removes the biggest Python critical-path dependency
- [ ] Go ownership of current management APIs - operators need the same admin workflows after cutover
- [ ] Go ownership of payment and team domains - these are still missing in the current partial migration
- [ ] Production cutover and Python backend retirement plan - otherwise the migration remains half-finished

### Add After Validation (v1.x)

- [ ] Storage hardening for secrets - valuable, but not the first migration milestone
- [ ] Deeper retry/compensation policy improvements - add after parity is stable

### Future Consideration (v2+)

- [ ] UI/runtime consolidation beyond the current templates/static JS - defer until backend parity is stable
- [ ] Schema cleanup and data-model simplification - only after compatibility pressure is gone

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Compatibility contract and regression harness | HIGH | MEDIUM | P1 |
| Native registration/runtime parity | HIGH | HIGH | P1 |
| Management API migration | HIGH | HIGH | P1 |
| Payment and team migration | HIGH | HIGH | P1 |
| Post-cutover cleanup | MEDIUM | MEDIUM | P1 |
| Secret hardening and cleanup-only improvements | MEDIUM | MEDIUM | P2 |

**Priority key:**
- P1: Must have for launch
- P2: Should have, add when possible
- P3: Nice to have, future consideration

## Competitor Feature Analysis

| Feature | Legacy Python backend | Partial Go backend | Our Approach |
|---------|-----------------------|--------------------|--------------|
| Registration runtime | Mature but monolithic | Partial and improving | Finish Go ownership without breaking request semantics |
| Admin/management APIs | Broad feature coverage | Minimal coverage | Migrate by domain until the current UI can run on Go |
| Payment/team domains | Fully productized in Python | Not migrated | Move only after compatibility rails are in place |

## Sources

- `.planning/codebase/ARCHITECTURE.md`
- `.planning/codebase/STRUCTURE.md`
- `.planning/codebase/CONCERNS.md`
- `backend-go/README.md`
- Route scans across `src/web/routes/` and `backend-go/internal/**/http/*.go`

---
*Feature research for: Brownfield Python-to-Go backend migration for Codex Console*
*Researched: 2026-04-05*
