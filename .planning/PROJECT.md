# Codex Console Go Migration

## What This Is

Codex Console has completed the Go-backend cutover for the backend critical path. The next milestone extends that brownfield migration into the operator-facing frontend by creating a Go-exclusive admin console from a copied frontend asset set, keeping the current Python/Jinja frontend untouched, and refactoring the experience toward a management-system workflow without changing the core capabilities operators already use.

## Core Value

The Go runtime can own the operator console end to end while preserving the registration, account, payment, team, logs, and settings workflows operators already depend on.

## Current Milestone: v1.1 Go Admin Frontend Refactor

**Goal:** Introduce a Go-exclusive management frontend that keeps core workflows intact, leaves the legacy frontend untouched, and removes unrelated public/project-promo content from the new shell.

**Target features:**
- Go-owned copy of templates/static assets and route entrypoints for the admin UI
- Management-oriented navigation, layout, page chrome, and content cleanup
- Page-by-page migration of current operational workflows into the new shell
- Rollout and fallback rules that keep the legacy frontend available during transition

## Requirements

### Validated

- ✓ Operators can already use the Python Web UI for registration, account management, settings, logs, payment/bind-card, and team workflows through `webui.py`, `src/web/`, `src/core/`, and `src/database/`.
- ✓ The Go backend already owns PostgreSQL/Redis-backed jobs, registration start/batch APIs, task and batch websocket streaming, and worker orchestration through `backend-go/cmd/api/main.go`, `backend-go/cmd/worker/main.go`, `backend-go/internal/jobs/`, and `backend-go/internal/registration/`.
- ✓ The Go backend already persists core `accounts`, `email_services`, `settings`, `cpa_services`, `sub2api_services`, and `tm_services` state in PostgreSQL through `backend-go/db/migrations/0002_init_accounts_registration.sql`, `backend-go/db/migrations/0003_extend_registration_service_configs.sql`, `backend-go/internal/accounts/`, and `backend-go/internal/uploader/`.
- ✓ A native Go registration runner already exists and is wired into the Go worker through `backend-go/internal/nativerunner/` and `backend-go/cmd/worker/main.go`, even though migration coverage is still incomplete.
- ✓ By the end of `v1.0`, the default backend ownership path is Go-first (`go-api` + `go-worker`), while the Python frontend remains only as an explicit compatibility/presentation shell where retained.

### Active

- [ ] Create a Go-owned copy of the current frontend assets and page entrypoints without editing the existing Python frontend in place.
- [ ] Refactor the new Go frontend into a management-system shell with clearer navigation, denser operator workflows, and shared page chrome.
- [ ] Remove legacy project statements, GitHub/Telegram/sponsorship links, and unrelated public-facing copy from the new Go frontend.
- [ ] Preserve current route, action, and data behavior so registration, account, payment, team, logs, email-service, and settings workflows continue to work in the new shell.

### Out of Scope

- Editing or replacing the existing Python frontend in place - it must remain available untouched as a fallback/reference during this milestone.
- Backend API or stored-data redesign - this milestone is a presentation-layer refactor on top of the completed Go backend.
- Net-new business capabilities unrelated to current operator workflows - keep scope on UX, shell, and information-architecture refactor rather than product expansion.
- Full SPA/framework rewrite if it delays page parity - prefer a brownfield-friendly Go delivery model that preserves current behaviors first.

## Context

- The repository remains brownfield: backend ownership is now Go-first, while the operator-facing HTML pages still live in Python `templates/` and `static/` and are mounted by `src/web/app.py`.
- The current frontend is server-rendered HTML plus shared CSS (`static/css/style.css`) and page-specific vanilla JS (`static/js/*.js`); page copy, navigation, and shared notice chrome are repeated across templates.
- The existing frontend contains project declaration text, GitHub/Telegram/support links, and "OpenAI 注册系统" positioning that the user explicitly does not want carried into the Go-specific frontend.
- `backend-go/` currently owns the APIs and worker runtime, but does not yet own a dedicated admin UI asset tree, template rendering layer, or static asset mount for the console pages.
- The current page set includes login, registration, accounts, accounts overview, email services, payment, card pool, auto team, logs, and settings; these workflows must stay functionally intact while the UI shell changes.
- The new milestone is explicitly about copying the current frontend into a Go-exclusive workspace, then refactoring the new copy toward a management-system experience rather than touching the legacy frontend directly.

## Constraints

- **Legacy Preservation**: The existing Python frontend under `templates/` and `static/` must remain untouched as part of this refactor - the user explicitly wants a copied Go-specific frontend instead of in-place edits.
- **Workflow Parity**: Registration, account, email-service, payment, team, logs, and settings behaviors must stay functionally intact - the UI shell can change, but the operator playbook cannot be broken.
- **Content Cleanup**: Project notices, GitHub/Telegram/sponsorship links, and unrelated public-facing introduction copy must be removed from the new Go frontend.
- **Go Alignment**: The new frontend should be delivered through Go-owned routes/assets so the operator console continues moving toward single-runtime ownership.
- **Brownfield Safety**: Refactor page by page with clear rollback/fallback options instead of replacing the entire UI in one unsafe step.
- **Admin UX Direction**: Favor a management-system information architecture, denser controls, and clearer operational hierarchy over the current open-source/public project framing.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Use a domain-by-domain strangler migration instead of a big-bang rewrite | The repo already has partial Go coverage; controlled cutover reduces parity drift and rollback risk | ✓ Confirmed |
| Treat API, data, and workflow compatibility as release blockers | The current templates, scripts, and operators are tightly coupled to existing behavior | ✓ Confirmed |
| Count current Go jobs, registration, accounts, and uploader foundations as validated baseline | The user explicitly asked to plan only the remaining migration work | ✓ Confirmed |
| Keep the current frontend/UI in place during backend migration | Rewriting UI at the same time would hide backend parity gaps and expand scope | ✓ Confirmed |
| Remove Python from the production critical path only after parity evidence exists | Cutting over early would turn migration unknowns into user-facing regressions | ✓ Confirmed |
| Start frontend work as a new milestone after the Go backend cutover is complete | The backend migration goal is already achieved; frontend refactor should build on that stable baseline rather than reopen backend scope | — Pending |
| Build the new admin frontend from a Go-owned copy of the current frontend assets | The user explicitly does not want the existing frontend modified in place | — Pending |
| Treat shell/navigation/content cleanup as first-class requirements, not cosmetic polish | The request is about making the Go frontend feel like a management system and removing unrelated project branding/content | — Pending |
| Preserve current workflows and contracts while allowing page chrome and layout to change | The milestone is a frontend/operator-experience refactor, not a business-flow redesign | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? -> Move to Out of Scope with reason
2. Requirements validated? -> Move to Validated with phase reference
3. New requirements emerged? -> Add to Active
4. Decisions to log? -> Add to Key Decisions
5. "What This Is" still accurate? -> Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check - still the right priority?
3. Audit Out of Scope - reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-04-06 after v1.1 milestone initialization*
