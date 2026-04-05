# Pitfalls Research

**Domain:** Brownfield Python-to-Go backend migration for Codex Console
**Researched:** 2026-04-05
**Confidence:** HIGH

## Critical Pitfalls

### Pitfall 1: Route Contract Drift

**What goes wrong:**
Go handlers look "functionally similar" but change path coverage, payload field names, status values, or error shapes that the current UI and scripts already expect.

**Why it happens:**
Developers compare implementations instead of comparing observable client contracts.

**How to avoid:**
Create and maintain a parity matrix for every remaining Python-owned endpoint before migrating the domain.

**Warning signs:**
Frontend code needs special cases for Go, or migration notes use phrases like "slightly different response".

**Phase to address:**
Phase 1

---

### Pitfall 2: Shared Data Drift

**What goes wrong:**
Go and Python read or write different interpretations of the same data, especially for task state, team data, proxies, bind-card tasks, and logs.

**Why it happens:**
The repo currently spans SQLite-era Python models and PostgreSQL-era Go migrations with only partial overlap.

**How to avoid:**
Freeze the shared schema contract first, then migrate domains with explicit read/write compatibility checks.

**Warning signs:**
New migration work requires ad hoc data backfills, "temporary" reshaping code, or one-off repair scripts.

**Phase to address:**
Phase 1

---

### Pitfall 3: Hidden Frontend Coupling

**What goes wrong:**
A backend domain is marked "migrated" even though current templates/static JS still call Python-only endpoints or depend on Python-specific timing/state semantics.

**Why it happens:**
The migration focuses on backend packages but forgets to audit the current clients as compatibility consumers.

**How to avoid:**
Use current templates/static JS as first-class migration inputs and keep `CUT-01` visible before final cutover.

**Warning signs:**
There is no route-to-client map, or migrated domains are not exercised by current UI flows.

**Phase to address:**
Phases 1 and 3

---

### Pitfall 4: Permanent "Temporary" Python Bridge

**What goes wrong:**
The Python bridge remains on the critical path because it is convenient, so the migration never truly completes.

**Why it happens:**
The bridge reduces short-term pain and hides native parity gaps.

**How to avoid:**
Time-box any Python fallback, track it as explicit scope, and make bridge removal a success criterion.

**Warning signs:**
New Go features quietly depend on the bridge, or no phase owns removing it.

**Phase to address:**
Phases 2 and 5

---

### Pitfall 5: Domain Migration Without Runtime Semantics

**What goes wrong:**
Payment or team APIs move to Go, but task behavior, side effects, and resumability no longer match operator expectations.

**Why it happens:**
Teams migrate CRUD endpoints first and leave long-running orchestration semantics undefined.

**How to avoid:**
Treat runtime semantics as part of compatibility, not as a later cleanup.

**Warning signs:**
Migration checklists mention endpoints but not pause/resume/cancel/log/session behavior.

**Phase to address:**
Phase 4

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Keep Python and Go both writing the same domain indefinitely | Reduces short-term migration pressure | Source-of-truth ambiguity and hard-to-debug drift | Only in short, explicitly bounded transition windows |
| Move multiple unrelated domains in one phase | Fewer roadmap items | Hidden regressions and unclear rollback scope | Never for this migration |
| Skip regression fixtures because "the routes are obvious" | Faster coding today | Breakage only appears during cutover | Never |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| OpenAI auth/registration | Port request plumbing but miss status/state semantics | Verify observable workflow behavior, not just request success |
| Email services | Assume config persistence is enough | Preserve config keys, ordering, enable/disable behavior, and fallback semantics |
| CPA/Sub2API/TM uploads | Move payload senders without preserving trigger timing | Keep upload side effects behind the same post-persistence semantics |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Leaving process-local runtime state in migrated domains | Restart loses task visibility or control semantics | Move domain runtime state into durable Go services | As soon as operators need reliable long-running jobs |
| Running compatibility checks only manually | Regressions keep coming back | Automate parity checks for critical routes and workflows | Immediately once multiple domains are moving in parallel |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Accidentally widening access while moving endpoints | Sensitive admin actions become easier to call incorrectly | Preserve current access assumptions and make any auth changes explicit, not incidental |
| Logging or exporting more secret material during migration | Secret exposure increases while parity is being tested | Keep payload and export behavior intentionally compatible and reviewed |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Backend parity requires UI changes everywhere | Operators see migration churn as product churn | Keep the current UI unchanged until Go parity is complete |
| Async progress semantics change silently | Operators lose confidence in task state | Preserve current pause/resume/cancel/log expectations as success criteria |

## "Looks Done But Isn't" Checklist

- [ ] **Registration migration:** native runner exists, but the Python bridge is no longer required for critical flows
- [ ] **Management migration:** current UI paths no longer depend on Python-only endpoints
- [ ] **Payment/team migration:** task/session semantics match existing operator expectations
- [ ] **Cutover:** rollback instructions and compatibility evidence exist, not just code changes

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Route contract drift | MEDIUM | Re-enable compatibility facade, compare payload fixtures, and restore the last known good contract |
| Shared data drift | HIGH | Stop cutover, restore schema compatibility, and replay migration checks against stored records |
| Permanent Python bridge | MEDIUM | Move the bridge into an explicit adapter and add removal work to the active phase |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| Route contract drift | Phase 1 | Parity matrix and contract fixtures exist for remaining domains |
| Shared data drift | Phase 1 | Shared schema/runtime contract is documented and tested |
| Hidden frontend coupling | Phase 3 | Current UI can exercise migrated management APIs without Python-only routes |
| Permanent Python bridge | Phase 5 | Production critical path no longer depends on Python runtime |
| Domain migration without runtime semantics | Phase 4 | Payment/team flows preserve task and side-effect behavior |

## Sources

- `.planning/codebase/CONCERNS.md`
- `.planning/codebase/ARCHITECTURE.md`
- `backend-go/README.md`
- Route scans across `src/web/routes/` and `backend-go/internal/**/http/*.go`

---
*Pitfalls research for: Brownfield Python-to-Go backend migration for Codex Console*
*Researched: 2026-04-05*
