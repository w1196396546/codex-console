# Feature Research: v1.1 Go Admin Frontend Refactor

**Project:** Codex Console Go Migration  
**Milestone:** v1.1 Go Admin Frontend Refactor  
**Researched:** 2026-04-06  
**Confidence:** HIGH

## Table Stakes

### Frontend Isolation

- Copied Go-owned frontend asset set
- Legacy frontend left untouched
- Clear route-entry separation between old and new frontend

### Admin Shell

- Shared management-oriented navigation
- Consistent page shell and layout
- Removal of unrelated project/promo/public content

### Page Parity

- Registration page still usable
- Accounts and overview still usable
- Email services, settings, and logs still usable
- Payment, card-pool, and team workflows still usable

### Rollout Safety

- Fallback to untouched legacy frontend
- API/websocket contract stability during page migration
- Page-by-page rollout instead of all-at-once replacement

## Differentiators Worth Including

- Better information hierarchy for high-frequency admin tasks
- Denser operator layouts that reduce page hopping
- Clearer module grouping for registration, accounts, services, payments, teams, logs, and settings
- Shared visual language that feels like a management console rather than a public project landing shell

## Anti-Features

- Full SPA rewrite
- Brand-new business modules
- Marketing/community/support banners in shared page chrome
- One-shot replacement of the entire frontend without fallback
- Reorganizing backend endpoints purely for aesthetic frontend reasons

## Category Suggestions For Requirements

1. Isolation
2. Shell
3. Pages
4. Rollout

## Complexity Notes

- Isolation is medium complexity because it touches routing, asset ownership, and deployment shape.
- Shell is medium complexity because it affects every page through shared layout and content rules.
- Page migration is high complexity because each page has different JS and operator workflows.
- Rollout is medium-high complexity because it requires parity checks plus legacy fallback handling.

---
*Research completed: 2026-04-06*
