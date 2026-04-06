# Project Research Summary

**Project:** Codex Console Go Migration  
**Milestone:** v1.1 Go Admin Frontend Refactor  
**Domain:** Brownfield Go-owned admin frontend rollout  
**Researched:** 2026-04-06  
**Confidence:** HIGH

## Executive Summary

The right move is not to redesign the product from scratch. It is to take the current operator frontend, copy it into a Go-owned asset and routing surface, remove the unrelated public/project-promo framing, and then refactor the copied pages into a management-system shell while keeping the legacy frontend untouched as fallback.

Because the backend critical path is already Go-owned, this milestone should avoid backend-scope creep. The primary risks are asset ownership drift, broken page workflows during DOM/layout changes, and losing the safety net of the untouched legacy frontend.

## Key Findings

### Stack Additions

- Keep server-rendered pages plus page-specific vanilla JS for parity.
- Introduce a Go-owned frontend asset tree and admin UI router.
- Rebuild only thin template helpers in Go for shared shell concerns.
- Keep static asset mounts explicit; chi docs via Context7 warn that `RedirectSlashes` is not compatible with `http.FileServer`.

### Feature Table Stakes

- Copy, not modify, the existing frontend
- Management-oriented shared shell and navigation
- Removal of project notice/GitHub/Telegram/support content from the new frontend
- Functional parity for registration, accounts, overview, email services, settings, logs, payment, card pool, and team pages
- Page-by-page rollout with legacy fallback

### Watch Out For

- In-place edits to the legacy frontend break the user’s core constraint
- Shell-less page copying creates immediate drift
- Frontend-only aesthetic changes can accidentally break API/websocket usage
- Missing fallback/rollback rules turns the milestone into an unsafe cutover

## Suggested Phase Structure

### Phase 6: Go Frontend Isolation Baseline

Create the copied Go-owned frontend workspace, route mounts, and baseline verification without touching the legacy frontend.

### Phase 7: Admin Shell and Brand Cleanup

Build shared management navigation, page shell, and content cleanup rules for the new frontend.

### Phase 8: Core Management Pages

Migrate registration, accounts, overview, email services, settings, and logs into the new shell with parity checks.

### Phase 9: Workflow Pages and Rollout Readiness

Finish payment/card-pool/team/login polish plus rollout, fallback, and parity verification.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Strongly anchored to the current repo’s frontend delivery model and Go router shape |
| Features | HIGH | Directly derived from the user’s request and current page inventory |
| Architecture | HIGH | Clear separation between Go-owned frontend, Go APIs, and untouched legacy frontend |
| Pitfalls | HIGH | Risks are concrete and visible from the current brownfield setup |

**Overall confidence:** HIGH

---
*Research completed: 2026-04-06*
*Ready for roadmap: yes*
