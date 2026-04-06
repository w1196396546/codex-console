# Roadmap: Codex Console Go Migration (v1.1 Go Admin Frontend Refactor)

## Overview

This roadmap turns the completed Go backend migration into a Go-owned operator console. It does that by copying the current frontend into a Go-exclusive asset and routing surface, removing unrelated public/project-promo content, and refactoring the copied pages into a management-oriented shell while preserving the workflows and contracts operators already use.

## Phases

**Phase Numbering:**
- Integer phases (6, 7, 8, 9): Planned milestone work
- Decimal phases (6.1, 6.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 6: Go Frontend Isolation Baseline** - Create the copied Go-owned frontend workspace, route mounts, and compatibility guardrails without touching the legacy frontend.
- [ ] **Phase 7: Admin Shell and Brand Cleanup** - Build the management-system shell, navigation, shared page chrome, and content cleanup for the new frontend.
- [ ] **Phase 8: Core Management Pages** - Refactor the highest-traffic operator pages into the new shell while preserving their current actions and contracts.
- [ ] **Phase 9: Workflow Pages and Rollout Readiness** - Finish the remaining workflow pages, fallback wiring, and parity verification for the new frontend rollout.

## Phase Details

### Phase 6: Go Frontend Isolation Baseline
**Goal**: Establish a Go-owned copy of the frontend assets and entrypoints so the new admin console can evolve independently while the existing Python frontend stays untouched.
**Depends on**: Phase 5
**Requirements**: [ISO-01, ISO-02]
**Canonical refs**: `templates/`, `static/css/style.css`, `static/js/`, `src/web/app.py`, `backend-go/internal/http/router.go`, `backend-go/cmd/api/main.go`
**UI hint**: yes
**Success Criteria** (what must be TRUE):
  1. A dedicated Go-owned frontend directory structure exists for copied templates, static assets, and shared layout helpers without mutating the legacy frontend files in place.
  2. The Go runtime has clear route and asset mount points for the new admin console that stay separated from the existing `/api/*` surface and legacy frontend entrypoints.
  3. Baseline verification proves the legacy frontend still runs unchanged and the new Go frontend can boot independently.
**Plans**: 3 plans

Plans:
- [ ] 06-01: Establish the Go frontend directory structure, asset-copy strategy, and admin route mount points
- [ ] 06-02: Port shared login, layout, asset-version, and shell helpers into Go-owned frontend infrastructure
- [ ] 06-03: Add baseline verification that the legacy frontend remains untouched and the new Go frontend boots independently

### Phase 7: Admin Shell and Brand Cleanup
**Goal**: Replace the current public/open-source page framing with a management-oriented shell, shared navigation, and cleaner operator-facing content.
**Depends on**: Phase 6
**Requirements**: [SHELL-01, SHELL-02, SHELL-03]
**Canonical refs**: `templates/index.html`, `templates/accounts.html`, `templates/settings.html`, `templates/logs.html`, `templates/partials/site_notice.html`, `static/css/style.css`
**UI hint**: yes
**Success Criteria** (what must be TRUE):
  1. The new frontend exposes a consistent admin shell with grouped navigation and shared page chrome across modules.
  2. Project statement text, GitHub/Telegram/sponsorship links, and unrelated public-facing copy are removed from the new shared shell and headers.
  3. Shared design tokens, layout primitives, and responsive behavior support desktop-first operator workflows without breaking mobile usability.
**Plans**: 3 plans

Plans:
- [ ] 07-01: Design the management-oriented navigation, module grouping, and shared admin shell structure
- [ ] 07-02: Replace legacy project notice and public-facing copy in the new shared templates and page headers
- [ ] 07-03: Build shared styling, components, and responsive layout behavior for the new admin shell

### Phase 8: Core Management Pages
**Goal**: Move the highest-value operator pages into the new shell while preserving their current forms, actions, and API/websocket contracts.
**Depends on**: Phase 7
**Requirements**: [PAGE-01, PAGE-02]
**Canonical refs**: `templates/index.html`, `templates/accounts.html`, `templates/accounts_overview.html`, `templates/email_services.html`, `templates/settings.html`, `templates/logs.html`, `static/js/app.js`, `static/js/accounts.js`, `static/js/accounts_overview.js`, `static/js/email_services.js`, `static/js/settings.js`, `static/js/logs.js`
**UI hint**: yes
**Success Criteria** (what must be TRUE):
  1. Registration, account management, account overview, and email-service pages work inside the new shell with their current core actions intact.
  2. Settings and logs pages work inside the new shell with current filters, forms, and management actions intact.
  3. Shared page migration does not introduce route drift, broken asset loading, or incompatible frontend-to-API interactions.
**Plans**: 4 plans

Plans:
- [ ] 08-01: Refactor the registration/home and account pages into the new admin shell
- [ ] 08-02: Refactor account overview and email-service pages with a management-first information hierarchy
- [ ] 08-03: Refactor settings and logs pages with clearer operator controls and denser admin layouts
- [ ] 08-04: Verify the migrated core pages preserve their current JavaScript actions, form flows, and API contracts

### Phase 9: Workflow Pages and Rollout Readiness
**Goal**: Complete the remaining workflow pages and make the new frontend safe to roll out with parity checks and fallback rules.
**Depends on**: Phase 8
**Requirements**: [PAGE-03, ROLL-01, ROLL-02]
**Canonical refs**: `templates/payment.html`, `templates/card_pool.html`, `templates/auto_team.html`, `templates/login.html`, `static/js/payment.js`, `static/js/auto_team.js`, `backend-go/internal/http/router.go`, `src/web/app.py`
**UI hint**: yes
**Success Criteria** (what must be TRUE):
  1. Payment, card-pool, and team workflows operate inside the new shell without losing their current operational behavior.
  2. Operators can access the new frontend while still retaining the untouched legacy frontend as an explicit fallback during rollout.
  3. Page parity, API/websocket compatibility, and rollout-readiness evidence exist before the new frontend is positioned as the preferred operator UI.
**Plans**: 4 plans

Plans:
- [ ] 09-01: Refactor payment, card-pool, and team pages into the new admin shell
- [ ] 09-02: Finalize login, shared auth flow, and cross-page polish for the Go admin frontend
- [ ] 09-03: Add rollout and fallback wiring so the untouched legacy frontend remains available during adoption
- [ ] 09-04: Verify page parity, API/websocket compatibility, and production-ready handoff for the new frontend

## Progress

**Execution Order:**
Phases execute in numeric order: 6 -> 7 -> 8 -> 9

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 6. Go Frontend Isolation Baseline | 0/3 | Not started | - |
| 7. Admin Shell and Brand Cleanup | 0/3 | Not started | - |
| 8. Core Management Pages | 0/4 | Not started | - |
| 9. Workflow Pages and Rollout Readiness | 0/4 | Not started | - |
