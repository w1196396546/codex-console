# Pitfall Research: v1.1 Go Admin Frontend Refactor

**Project:** Codex Console Go Migration  
**Milestone:** v1.1 Go Admin Frontend Refactor  
**Researched:** 2026-04-06  
**Confidence:** HIGH

## Critical Pitfalls

### 1. Editing the legacy frontend in place

**Why it hurts:** Violates the user’s explicit constraint and removes the fallback/reference copy.  
**Prevention:** Create a Go-owned copied asset tree first and treat legacy frontend files as read-only for this milestone.

### 2. Rebuilding the UI as a brand-new SPA

**Why it hurts:** Explodes scope and hides whether regressions come from workflow differences or shell/layout changes.  
**Prevention:** Keep server-rendered pages plus page-specific vanilla JS for this milestone.

### 3. Copying pages without a shared shell contract

**Why it hurts:** Duplicates nav/header/content-cleanup changes across every page and creates drift immediately.  
**Prevention:** Build the admin shell before bulk page migration.

### 4. Leaving project notice/promotional content in page fragments

**Why it hurts:** The user explicitly wants those removed from the new frontend, and leftover fragments will make the milestone look unfinished.  
**Prevention:** Identify all shared content fragments and repeated headers early, then centralize cleanup in the shared shell phase.

### 5. Breaking API/websocket expectations while changing DOM/layout

**Why it hurts:** The new frontend may look better but become operationally unusable.  
**Prevention:** Keep API/websocket contracts unchanged and verify copied JS actions/page flows after each page migration phase.

### 6. Static asset route/middleware conflicts

**Why it hurts:** Broken CSS/JS delivery can make the new frontend appear randomly incomplete.  
**Prevention:** Use explicit static mounts and avoid chi middleware combinations that conflict with `http.FileServer`; chi docs note `RedirectSlashes` is incompatible there.

### 7. No explicit rollback/fallback path

**Why it hurts:** Operators get stuck if a migrated page is not production-ready.  
**Prevention:** Make legacy frontend availability and page-by-page rollout an explicit requirement, not an afterthought.

## Phase Mapping

- Phase 6 should address pitfalls 1 and 6
- Phase 7 should address pitfalls 3 and 4
- Phase 8 should address pitfall 5 for core pages
- Phase 9 should address pitfall 7 and final parity risk

---
*Research completed: 2026-04-06*
