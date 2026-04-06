# Requirements: Codex Console Go Migration (v1.1 Go Admin Frontend Refactor)

**Defined:** 2026-04-06
**Core Value:** The Go runtime can own the operator console end to end while preserving the registration, account, payment, team, logs, and settings workflows operators already depend on.

## v1 Requirements

### Isolation

- [ ] **ISO-01**: The new Go admin frontend is built from a dedicated copy of templates and static assets so the existing Python frontend remains untouched and still runnable.
- [ ] **ISO-02**: The Go runtime can serve the new admin frontend through Go-owned routes/assets without reintroducing Python onto the operator critical path.

### Shell

- [ ] **SHELL-01**: Operators land in a management-oriented shell with grouped navigation, clearer module hierarchy, and consistent shared layout across pages.
- [ ] **SHELL-02**: The new shared shell removes project declaration text, GitHub/Telegram/sponsorship links, and unrelated public-facing copy from page chrome and headers.
- [ ] **SHELL-03**: Shared design tokens, layout primitives, and responsive behavior support desktop-first admin workflows while remaining usable on common mobile widths.

### Pages

- [ ] **PAGE-01**: Operators can complete registration, account management, account overview, and email-service workflows from the new frontend with their current core actions intact.
- [ ] **PAGE-02**: Operators can complete settings and logs workflows from the new frontend with their current forms, filters, and management actions intact.
- [ ] **PAGE-03**: Operators can complete payment, card-pool, and team workflows from the new frontend with their current task, session, and operational behaviors intact.

### Rollout

- [ ] **ROLL-01**: The new frontend keeps current API paths, request/response shapes, and websocket behaviors compatible across migrated pages during rollout.
- [ ] **ROLL-02**: Operators can adopt the new frontend page by page or module by module without losing access to the untouched legacy frontend as fallback.

## v2 Requirements

### Consolidation

- **CONS-03**: The legacy Python frontend can be formally retired only after the Go admin frontend has full parity evidence and operator sign-off.
- **CONS-04**: Shared frontend primitives are extracted into a durable design system only after the page migration is stable.

### Expansion

- **EXP-01**: New analytics, dashboards, or role-based admin capabilities are added only after the shell/page parity milestone is complete.
- **EXP-02**: Any future SPA or framework migration is evaluated separately after the copied Go frontend proves its value.

## Out of Scope

| Feature | Reason |
|---------|--------|
| Editing the existing Python frontend in place | The user explicitly wants a copied Go-specific frontend and a preserved legacy fallback |
| Backend API or schema redesign | This milestone is about the operator-facing frontend shell, not changing backend contracts |
| Net-new business workflows | Scope is current page parity plus shell/layout/content refactor |
| Full SPA/framework rewrite | Higher risk and unnecessary before the copied Go frontend reaches parity |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| ISO-01 | Phase 6 | Pending |
| ISO-02 | Phase 6 | Pending |
| SHELL-01 | Phase 7 | Pending |
| SHELL-02 | Phase 7 | Pending |
| SHELL-03 | Phase 7 | Pending |
| PAGE-01 | Phase 8 | Pending |
| PAGE-02 | Phase 8 | Pending |
| PAGE-03 | Phase 9 | Pending |
| ROLL-01 | Phase 9 | Pending |
| ROLL-02 | Phase 9 | Pending |

**Coverage:**
- v1 requirements: 10 total
- Mapped to phases: 10
- Unmapped: 0 ✓

---
*Requirements defined: 2026-04-06*
*Last updated: 2026-04-06 after v1.1 milestone definition*
