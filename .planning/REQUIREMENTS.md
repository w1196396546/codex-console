# Requirements: Codex Console Go Migration

**Defined:** 2026-04-05
**Core Value:** The Go backend can take over the current Codex Console backend responsibilities without forcing existing clients, persisted data, or critical registration, payment, and team workflows to change behavior.

## v1 Requirements

### Compatibility

- [ ] **COMP-01**: Existing clients can call every remaining Python-only backend capability through a Go-owned route using the same path and HTTP method.
- [ ] **COMP-02**: Existing clients receive compatible JSON field names, status values, and error semantics from migrated Go endpoints.
- [x] **COMP-03**: Existing polling and websocket consumers receive compatible task and batch progress semantics from Go-owned backend flows.

### Data Contracts

- [ ] **DATA-01**: Existing persisted records for accounts, settings, upload services, proxies, bind-card tasks, app logs, and team entities remain readable and writable after Go takeover without manual reshaping.

### Runtime

- [x] **RUN-01**: Registration start, batch, and Outlook batch flows execute on Go-owned runtime logic without requiring the Python runner bridge on the critical path.
- [x] **RUN-02**: Existing clients can list, inspect, pause, resume, cancel, and clean up registration tasks and batches through Go with current behavior.
- [x] **RUN-03**: Registration side effects continue to persist accounts and trigger CPA, Sub2API, and TM uploads with current semantics.

### Management

- [x] **MGMT-01**: Operators can manage accounts through Go-owned APIs with current CRUD, import/export, refresh, validate, and upload workflows.
- [x] **MGMT-02**: Operators can manage settings, email services, upload-service configs, proxies, and logs through Go-owned APIs with current behavior.

### Domain Flows

- [x] **PAY-01**: Operators can run payment, bind-card, and subscription-sync workflows through Go-owned APIs with current task and session semantics.
- [ ] **TEAM-01**: Operators can run team discovery, sync, invite, membership, and team-task workflows through Go-owned APIs with current behavior.

### Cutover

- [x] **CUT-01**: Current templates and static JavaScript can target the Go backend for migrated domains without requiring a UI rewrite.
- [ ] **CUT-02**: Production deployment can disable Python backend responsibilities with rollback instructions and parity verification evidence in place.
- [ ] **OPS-01**: Go backend operational controls are sufficient to replace Python backend duties safely in production for all migrated domains.

## v2 Requirements

### Hardening

- **HARD-01**: Secrets are encrypted at rest and sensitive admin endpoints are fully hardened after the migration milestone.
- **HARD-02**: Retry, compensation, and failure-recovery policies are expanded beyond current migration parity needs.

### Consolidation

- **CONS-01**: Frontend delivery is consolidated away from the Python/Jinja runtime if a later milestone wants a single-runtime stack.
- **CONS-02**: Legacy schema cleanup removes transitional compatibility-only fields and adapters after migration risk is gone.

## Out of Scope

| Feature | Reason |
|---------|--------|
| Frontend redesign or full SPA rewrite | Not required to replace the backend safely and would hide parity regressions |
| Net-new product capabilities unrelated to migration | The scope is remaining migration work only |
| Intentional API redesign | Existing clients and operators already depend on the current contract |
| Schema cleanup before parity is complete | It mixes migration risk with redesign risk |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| COMP-01 | Phase 1 | Pending |
| COMP-02 | Phase 1 | Pending |
| DATA-01 | Phase 1 | Pending |
| OPS-01 | Phase 1 | Pending |
| RUN-01 | Phase 2 | Complete |
| RUN-02 | Phase 2 | Complete |
| RUN-03 | Phase 2 | Complete |
| COMP-03 | Phase 2 | Complete |
| MGMT-01 | Phase 3 | Complete |
| MGMT-02 | Phase 3 | Complete |
| CUT-01 | Phase 3 | Complete |
| PAY-01 | Phase 4 | Complete |
| TEAM-01 | Phase 4 | Pending |
| CUT-02 | Phase 5 | Pending |

**Coverage:**
- v1 requirements: 14 total
- Mapped to phases: 14
- Unmapped: 0 ✓

---
*Requirements defined: 2026-04-05*
*Last updated: 2026-04-05 after initial definition*
