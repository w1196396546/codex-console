# Roadmap: Codex Console Go Migration

## Overview

This roadmap finishes the remaining Python-to-Go backend migration in the current brownfield repository. It treats the existing Go jobs, registration, accounts, and uploader foundations as baseline, then focuses only on the domains and runtime semantics that still keep Python on the critical path. Every phase is organized around preserving current API contracts, stored data shapes, and operator-facing business behavior while progressively shifting ownership to Go.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Compatibility Baseline** - Freeze the migration contract, parity matrix, and shared data/runtime rules. (completed 2026-04-05)
- [x] **Phase 2: Native Registration Runtime** - Remove Python from the registration critical path while preserving task behavior. (completed 2026-04-05)
- [x] **Phase 3: Management APIs** - Move current admin and management domains to Go behind the existing UI. (completed 2026-04-05)
- [ ] **Phase 4: Payment and Team Domains** - Migrate the remaining Python-only product workflows with compatible runtime semantics.
- [ ] **Phase 5: Cutover and Decommission** - Switch production backend ownership to Go and retire Python responsibilities safely.

## Phase Details

### Phase 1: Compatibility Baseline
**Goal**: Freeze the remaining Python-to-Go migration contract so later phases can move domains without breaking current clients or stored data.
**Depends on**: Nothing (first phase)
**Requirements**: [COMP-01, COMP-02, DATA-01, OPS-01]
**Canonical refs**: `.planning/codebase/ARCHITECTURE.md`, `.planning/codebase/STRUCTURE.md`, `src/web/routes/__init__.py`, `backend-go/internal/http/router.go`, `src/database/models.py`, `src/database/team_models.py`, `backend-go/db/migrations/0002_init_accounts_registration.sql`, `backend-go/db/migrations/0003_extend_registration_service_configs.sql`
**UI hint**: no
**Success Criteria** (what must be TRUE):
  1. The remaining Python-only route surface and client dependencies are captured in a parity matrix with explicit Go ownership targets.
  2. Shared data contracts are documented for migrated and not-yet-migrated domains, including how PostgreSQL replaces or adapts legacy Python data semantics.
  3. Migration safety rails exist before domain cutover begins, including compatibility fixtures, operational checks, and a clear auth/runtime boundary decision.
**Plans**: 3 plans

Plans:
- [x] 01-01: Build the Python-versus-Go route and client parity matrix
- [x] 01-02: Define the shared schema and runtime compatibility contract
- [x] 01-03: Add migration harness, compatibility fixtures, and cutover safety checks

### Phase 2: Native Registration Runtime
**Goal**: Complete Go ownership of registration execution, task lifecycle, and upload side effects so Python is no longer required on the registration critical path.
**Depends on**: Phase 1
**Requirements**: [RUN-01, RUN-02, RUN-03, COMP-03]
**Canonical refs**: `src/web/routes/registration.py`, `src/web/task_manager.py`, `src/core/register.py`, `backend-go/internal/registration/`, `backend-go/internal/nativerunner/`, `backend-go/cmd/worker/main.go`
**UI hint**: no
**Success Criteria** (what must be TRUE):
  1. Registration start, batch, and Outlook batch flows complete through Go-owned execution without requiring the Python runner bridge on the critical path.
  2. Existing clients can inspect, pause, resume, cancel, and observe task and batch progress through Go with compatible status and log behavior.
  3. Account persistence and CPA/Sub2API/TM upload side effects remain compatible with current workflow semantics.
**Plans**: 4 plans

Plans:
- [x] 02-01: Close native registration preparation and worker critical-path parity
- [x] 02-02: Complete task, batch, Outlook, and polling runtime-semantics compatibility
- [x] 02-03: Finalize Go-owned account persistence and auto-upload side effects
- [x] 02-04: Align task and batch websocket runtime semantics

### Phase 3: Management APIs
**Goal**: Migrate the current account, settings, email-service, upload-config, proxy, and log management surfaces to Go while preserving the current UI contract.
**Depends on**: Phase 2
**Requirements**: [MGMT-01, MGMT-02, CUT-01]
**Canonical refs**: `src/web/routes/accounts.py`, `src/web/routes/settings.py`, `src/web/routes/email.py`, `src/web/routes/logs.py`, `src/web/routes/upload/cpa_services.py`, `src/web/routes/upload/sub2api_services.py`, `src/web/routes/upload/tm_services.py`, `static/js/accounts.js`, `static/js/settings.js`, `static/js/email_services.js`, `static/js/logs.js`, `backend-go/internal/accounts/`
**UI hint**: yes
**Success Criteria** (what must be TRUE):
  1. Existing account-management workflows operate against Go-owned APIs with current CRUD, import/export, refresh, validate, and upload behavior.
  2. Existing configuration and admin APIs behave compatibly for settings, email services, upload services, proxies, and logs.
  3. Current templates and static JavaScript can target these migrated Go domains without a frontend rewrite.
**Plans**: 8 plans

Plans:
- [x] 03-01-PLAN.md — Migrate accounts workflows, compatibility DTOs, and account action endpoints
- [x] 03-02-PLAN.md — Migrate settings, proxies, and database-admin compatibility APIs
- [x] 03-03-PLAN.md — Migrate email-services management APIs and Outlook admin actions
- [x] 03-04-PLAN.md — Migrate CPA/Sub2API/TM upload-config management APIs
- [x] 03-05-PLAN.md — Migrate app log browsing, cleanup, and clear APIs
- [x] 03-06-PLAN.md — Wire management domains into Go API and verify current UI contract parity
- [x] 03-07-PLAN.md — Restore Python-compatible `/accounts-overview` refresh semantics and regression coverage
- [x] 03-08-PLAN.md — Wire `UploadAccountStore` into live Sub2API upload bootstrap and add bootstrap-level tests

### Phase 4: Payment and Team Domains
**Goal**: Migrate the remaining Python-only payment/bind-card and team workflows to Go with compatible runtime and persistence behavior.
**Depends on**: Phase 3
**Requirements**: [PAY-01, TEAM-01]
**Canonical refs**: `src/web/routes/payment.py`, `src/web/routes/team.py`, `src/web/routes/team_tasks.py`, `src/database/team_models.py`, `static/js/payment.js`, `static/js/auto_team.js`, `backend-go/internal/uploader/`
**UI hint**: yes
**Success Criteria** (what must be TRUE):
  1. Payment, bind-card, and subscription-sync flows run through Go-owned APIs with current task, session, and side-effect semantics.
  2. Team discovery, sync, invite, membership, and team-task workflows run through Go-owned APIs with compatible persisted data and operator behavior.
  3. Remaining Python-only runtime semantics in these domains are either migrated or isolated behind explicit transition adapters.
**Plans**: 3 plans

Plans:
- [x] 04-01-PLAN.md — Build the Go payment slice for bind-card tasks, session/bootstrap, and subscription-sync compatibility
- [x] 04-02-PLAN.md — Build the Go team slice for discovery, sync, invite, membership, and accepted-task compatibility
- [x] 04-03-PLAN.md — Mount payment/team into the Go API and verify phase-wide runtime and operator parity

### Phase 5: Cutover and Decommission
**Goal**: Cut production backend ownership over to Go, verify compatibility end to end, and retire Python backend responsibilities safely.
**Depends on**: Phase 4
**Requirements**: [CUT-02]
**Canonical refs**: `webui.py`, `src/web/app.py`, `scripts/docker/start-webui.sh`, `backend-go/cmd/api/main.go`, `backend-go/cmd/worker/main.go`, `docker-compose.yml`
**UI hint**: no
**Success Criteria** (what must be TRUE):
  1. Production can route backend traffic to Go for all in-scope flows with rollback instructions documented and tested.
  2. Python backend responsibilities are removed from the production critical path, or any temporary leftovers are explicitly isolated with a follow-up owner.
  3. Compatibility evidence exists for API, data, and key operator workflows before final migration sign-off.
**Plans**: 2 plans

Plans:
- [ ] 05-01: Execute staged cutover, rollback, and parity verification
- [ ] 05-02: Retire Python backend responsibilities from the production path

## Progress

**Execution Order:**
Phases execute in numeric order: 1 -> 2 -> 3 -> 4 -> 5

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Compatibility Baseline | 3/3 | Complete    | 2026-04-05 |
| 2. Native Registration Runtime | 4/4 | Complete    | 2026-04-05 |
| 3. Management APIs | 8/8 | Complete    | 2026-04-05 |
| 4. Payment and Team Domains | 0/3 | Not started | - |
| 5. Cutover and Decommission | 0/2 | Not started | - |
