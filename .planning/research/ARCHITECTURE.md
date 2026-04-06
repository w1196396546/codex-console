# Architecture Research: v1.1 Go Admin Frontend Refactor

**Project:** Codex Console Go Migration  
**Milestone:** v1.1 Go Admin Frontend Refactor  
**Researched:** 2026-04-06  
**Confidence:** HIGH

## Recommended Architecture Shape

### 1. Go-Owned Admin UI Surface

Introduce a dedicated Go-owned admin UI surface that is separate from:

- the legacy Python frontend (`templates/`, `static/`)
- the Go API surface (`/api/*`)

This lets the project keep the old frontend intact while the new frontend evolves toward full Go ownership.

### 2. Shared Admin Shell Layer

Create one shared shell for:

- navigation
- page headers
- content cleanup rules
- shared layout primitives
- asset versioning
- login/auth page framing

Every migrated page should plug into that shell instead of re-copying brand/nav/header fragments independently.

### 3. Page Modules

Treat each current page as a bounded migration unit:

- registration
- accounts
- accounts overview
- email services
- settings
- logs
- payment
- card pool
- auto team
- login

This makes page-by-page rollout and verification possible.

### 4. Compatibility Boundary

The new frontend should keep consuming the existing Go-owned API/websocket contracts rather than inventing frontend-only compatibility shims. If DOM/layout changes force JS refactors, those refactors should stay inside the new Go-owned frontend asset tree.

## Suggested Build Order

1. Establish copied asset tree and Go route mounts
2. Build shared admin shell and content-cleanup layer
3. Migrate highest-frequency management pages first
4. Migrate workflow-heavy pages last
5. Add rollout/fallback verification after page parity is in place

## Data Flow Guidance

- Browser -> Go admin page route -> shared shell/template -> copied page asset bundle
- Page JS -> existing Go-owned `/api/*` and `/api/ws/*` contracts
- Fallback path remains available through the untouched legacy Python frontend

## New vs Modified Components

### New

- Go-owned frontend asset directory
- Go admin UI route group
- Shared admin shell/layout helpers
- Rollout/fallback verification docs/tests

### Modified

- Go router/bootstrap to mount admin UI routes and static assets
- New copied JS/CSS/template assets inside the Go-owned tree
- Possibly auth/login page wiring for the new frontend entry

### Not Modified In Place

- Existing Python `templates/`
- Existing Python `static/`
- Existing backend API contracts unless a parity bug is discovered

---
*Research completed: 2026-04-06*
