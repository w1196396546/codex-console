# Phase 1: Compatibility Baseline - Context

**Gathered:** 2026-04-05
**Status:** Ready for planning
**Mode:** Auto-generated (discuss skipped via workflow.skip_discuss)

<domain>
## Phase Boundary

Freeze the remaining Python-to-Go migration contract so later phases can move domains without breaking current clients or stored data. This phase delivers the compatibility baseline only: route/client parity inventory, shared data/runtime contract, and migration safety rails. It does not migrate payment, team, or management domains yet.

</domain>

<decisions>
## Implementation Decisions

### Compatibility contract
- **D-01:** Treat the current Python backend behavior as the compatibility reference for every not-yet-migrated domain.
- **D-02:** Route path, HTTP method, JSON field names, status values, websocket semantics, and critical side effects are all part of parity; none may drift silently.

### Scope boundary
- **D-03:** Count existing Go jobs, registration/task APIs, websocket streams, account-listing baseline, uploader persistence, and native runner foundations as already-migrated baseline.
- **D-04:** Phase 1 must only document and instrument the remaining migration delta; it must not re-plan or re-implement already existing Go foundations.

### Migration safety rails
- **D-05:** Use current templates and `static/js` clients as first-class compatibility consumers when building the parity matrix.
- **D-06:** Capture shared schema/runtime rules before moving more domains so later phases can migrate against an explicit contract instead of ad hoc behavior.

### the agent's Discretion
The agent may choose the exact artifact format for parity matrices, compatibility checklists, and regression fixtures as long as they are reviewable, traceable, and directly usable by later planning/execution phases.

</decisions>

<specifics>
## Specific Ideas

No specific UX or implementation preferences beyond the migration rules already captured in `.planning/PROJECT.md`: preserve current API, data structures, and critical business behavior; plan only the remaining migration work.

</specifics>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project scope and constraints
- `.planning/PROJECT.md` — Authoritative project framing for the migration, including validated baseline, active scope, and compatibility constraints.
- `.planning/REQUIREMENTS.md` — Phase-level requirement IDs and v1 scope boundaries for migration work.
- `.planning/ROADMAP.md` — Phase ordering, success criteria, and canonical refs for this phase.

### Current codebase map
- `.planning/codebase/ARCHITECTURE.md` — Current Python/Go split-runtime architecture and data-flow boundaries.
- `.planning/codebase/STRUCTURE.md` — Physical file layout and where Python and Go responsibilities currently live.
- `.planning/codebase/CONCERNS.md` — Known migration risks, parity pitfalls, and fragile areas already identified.
- `.planning/codebase/STACK.md` — Runtime and storage split between Python, Go, PostgreSQL, Redis, and current tooling.

### Current backend contract sources
- `src/web/routes/__init__.py` — Python API router composition and current domain prefixes.
- `src/web/app.py` — Python HTML + `/api` mounting and current page/runtime entry points.
- `backend-go/internal/http/router.go` — Current Go route surface and mounted websocket endpoints.
- `backend-go/README.md` — Explicit statement of what the current Go backend does and does not yet migrate.

### Current client and persistence anchors
- `static/js/` — Existing browser clients that encode route and payload expectations.
- `src/database/models.py` — Current Python-side persisted models for accounts, settings, logs, bind-card tasks, proxies, and service configs.
- `src/database/team_models.py` — Current Python-side persisted team entities and task models.
- `backend-go/db/migrations/0002_init_accounts_registration.sql` — Current Go-side baseline schema for accounts, email services, and settings.
- `backend-go/db/migrations/0003_extend_registration_service_configs.sql` — Current Go-side schema for uploader configs and account/service compatibility fields.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `.planning/codebase/*.md`: ready-made architectural and risk summaries that can seed parity and migration-contract artifacts.
- `backend-go/internal/http/router.go`, `backend-go/internal/registration/http/handlers.go`, `backend-go/internal/accounts/http/handlers.go`: current Go route surface and handler style to extend.
- `src/web/routes/*.py`: Python route modules grouped by domain, useful as the source inventory for the remaining migration delta.

### Established Patterns
- Python groups operator-facing behavior by route module and mixes runtime/state behavior into domain endpoints; later phases need an explicit contract before decomposing that logic.
- Go already uses domain packages plus chi handlers, PostgreSQL, and Redis/Asynq; later phases should extend this structure instead of copying Python monolith patterns.
- Existing frontend code in `static/js/` talks directly to `/api/*` and `/api/ws/*`, so contract stability matters more than internal implementation purity.

### Integration Points
- Phase 1 should connect roadmap requirements to real code boundaries in `src/web/routes/`, `static/js/`, `src/database/`, and `backend-go/internal/`.
- Later phases will depend on the parity matrix and shared contract artifacts created here to decide what to migrate and how to verify it.

</code_context>

<deferred>
## Deferred Ideas

- Actual migration of management, payment, and team domains belongs to later phases.
- Security hardening beyond what is necessary to define migration-safe contracts remains outside this phase.

</deferred>

---
*Phase: 01-compatibility-baseline*
*Context gathered: 2026-04-05*
