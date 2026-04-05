# Phase 1 Shared Schema Contract

## Contract Scope

This document freezes the storage contract that later migration phases must respect. Python ORM models remain the compatibility reference for field semantics. Go migrations and repositories are the target production path where coverage already exists. Shared table names do not imply full parity; column- and behavior-level gaps remain part of the contract.

SQLite remains a Python reference input, not the Go production path.

## Shared Table Overview

| Table | Current owner | Go coverage | Storage policy | Compatibility gaps | Phase handoff |
|---|---|---|---|---|---|
| `accounts` | Shared: Python compatibility oracle + Go baseline persistence | `backend-go/db/migrations/0002_init_accounts_registration.sql`, `0003_extend_registration_service_configs.sql`, `backend-go/internal/accounts/repository_postgres.go` | Shared transition table; preserve current account field names and semantics while Go expands ownership | No Phase 1 blocker beyond preserving Python semantics for `extra_data`, upload flags, subscription metadata, and cookies on the Go path | Phase 2 keeps registration persistence compatible; Phase 3 expands account-management ownership |
| `email_services` | Shared: Python compatibility oracle + Go baseline persistence | `0002_init_accounts_registration.sql`, `backend-go/internal/registration/available_services_postgres.go`, `backend-go/internal/registration/outlook_repository_postgres.go` | Shared transition table; Go may read/write covered fields, Python remains oracle for service behavior | `email_services.last_used` exists in Python ORM and is not present in current Go migrations; Python service config semantics remain the source of truth | Phase 3 must decide whether to add `last_used` to Go schema or treat it as legacy-only metadata |
| `settings` | Shared: Python compatibility oracle + Go baseline persistence | `0002_init_accounts_registration.sql`, `backend-go/internal/registration/available_services_postgres.go` | Shared transition table for key/value configuration readback | `settings.description/category/updated_at` exist in Python ORM and are not present in current Go migrations; Python settings metadata remains richer than Go | Phase 3 must preserve these metadata fields explicitly or formalize their retirement |
| `cpa_services` | Shared: Python compatibility oracle + Go uploader-config persistence | `0003_extend_registration_service_configs.sql`, `backend-go/internal/uploader/repository_postgres.go` | Shared transition table; Go can already read enabled uploader configs | No current Phase 1 field blocker; behavior still remains Python-owned at the admin API layer | Phase 3 migrates config management endpoints while keeping current row shape |
| `sub2api_services` | Shared: Python compatibility oracle + Go uploader-config persistence | `0003_extend_registration_service_configs.sql`, `backend-go/internal/uploader/repository_postgres.go` | Shared transition table; preserve `target_type` behavior and enabled/priority semantics | No current Phase 1 field blocker beyond preserving current config meanings | Phase 3 migrates admin/config endpoints without reshaping current records |
| `tm_services` | Shared: Python compatibility oracle + Go uploader-config persistence | `0003_extend_registration_service_configs.sql`, `backend-go/internal/uploader/repository_postgres.go` | Shared transition table; preserve current API URL/key semantics | No current Phase 1 field blocker beyond preserving current config meanings | Phase 3 migrates admin/config endpoints without reshaping current records |
| `registration_tasks` | Python-owned runtime table | No Go schema coverage | Legacy Python runtime record; do not assume Go ownership yet | Entire table is absent from Go migrations; Python task lifecycle and stored logs/results still define behavior | Phase 2 decides whether to replace with Go job projections or add a compatibility persistence layer |
| `bind_card_tasks` | Python-owned runtime table | No Go schema coverage | Python-only payment/bind-card runtime state until Phase 4 | Entire table is absent from Go migrations and carries payment session/task semantics | Phase 4 must either migrate or explicitly replace this runtime contract |
| `app_logs` | Python-owned runtime table | No Go schema coverage (`job_logs` is not a drop-in replacement) | Python-only admin log store until a compatible replacement exists | Entire table is absent from Go migrations; Go `job_logs` covers queue logs, not current app log browsing semantics | Phase 3 must decide whether to mirror or replace current admin log behavior |
| `proxies` | Python-owned configuration table | No Go schema coverage | Python-only proxy configuration until a Go config domain exists | Entire table absent from Go migrations; includes `is_default`, credentials, and `last_used` behavior | Phase 3 must migrate proxy admin APIs and storage contract together |
| `teams` | Python-owned domain table | No Go schema coverage | Python-only team domain state | Entire table absent from Go migrations | Phase 4 owns migration strategy |
| `team_memberships` | Python-owned domain table | No Go schema coverage | Python-only team domain state | Entire table absent from Go migrations | Phase 4 owns migration strategy |
| `team_tasks` | Python-owned runtime/domain table | No Go schema coverage | Python-only team task state and logs | Entire table absent from Go migrations; carries task UUID, active scope, payload, and logs semantics | Phase 4 owns migration strategy |
| `team_task_items` | Python-owned domain table | No Go schema coverage | Python-only team task item state | Entire table absent from Go migrations | Phase 4 owns migration strategy |

## Table Detail Notes

## accounts

- Python compatibility source: `src/database/models.py`
- Go baseline coverage: `backend-go/db/migrations/0002_init_accounts_registration.sql`, `backend-go/db/migrations/0003_extend_registration_service_configs.sql`
- Contract rule: Preserve existing field names and meanings for tokens, cookies, upload flags, `source`, `subscription_type`, `subscription_at`, and `extra_data`.
- Current handoff: Go already persists registration/account baseline records, but Python still defines richer management semantics and some write paths.

## email_services

- Python compatibility source: `src/database/models.py`
- Go baseline coverage: `backend-go/db/migrations/0002_init_accounts_registration.sql`
- Contract rule: Preserve current `service_type`, `name`, `config`, `enabled`, and `priority` semantics.
- Known gap: `email_services.last_used` is Python-only metadata today.

## settings

- Python compatibility source: `src/database/models.py`, `src/config/settings.py`
- Go baseline coverage: `backend-go/db/migrations/0002_init_accounts_registration.sql`
- Contract rule: Preserve current key/value behavior while recognizing that metadata is richer on the Python side.
- Known gap: `settings.description/category/updated_at` exist only in the Python ORM and current Python management flows.

## Python-only Runtime and Domain Tables

- `registration_tasks` and `bind_card_tasks` are runtime-bearing tables, not just archival records.
- `app_logs` and `proxies` are active admin/configuration data, not optional extras.
- `teams`, `team_memberships`, `team_tasks`, and `team_task_items` hold the current team domain and cannot be hand-waved as future cleanup.

## bind_card_tasks

- Python compatibility source: `src/database/models.py`
- Go coverage: none in current migrations or repositories
- Contract rule: Preserve current bind-card task fields, checkout/session metadata, and status semantics until Phase 4 explicitly replaces them.
- Current handoff: Remains Python-only runtime state for payment and subscription-sync workflows.

## teams

- Python compatibility source: `src/database/team_models.py`
- Go coverage: none in current migrations or repositories
- Contract rule: Preserve current team owner/account linkage, membership/task relationships, and status/error fields until Phase 4 migration work lands.
- Current handoff: Remains Python-only domain state for team discovery, sync, invite, and task flows.

## Shared Storage Policy

- Python ORM definitions remain the compatibility oracle for field semantics and row meaning until a later phase explicitly replaces them.
- Go coverage that already exists is baseline infrastructure, not proof of full parity.
- Any later phase touching shared tables must state whether it is:
  - preserving the current row contract,
  - extending the Go schema to match current Python semantics, or
  - formally retiring a Python-only field with migration evidence.

## Runtime Preconditions

- Go production-path execution requires `DATABASE_URL`.
- Go worker and queue-backed execution require `REDIS_ADDR`.
- Python may still read/write SQLite locally today, but that path is not sufficient for a Go-owned production cutover.
- Any future claim of “Go backend cutover complete” must account for explicit migration from SQLite-backed installs where relevant.

## Phase Handoff Rules

- Phase 2 may build on `accounts`, `email_services`, `settings`, and uploader-config tables, but it must not assume `registration_tasks` is already retired.
- Phase 3 must carry forward the `settings.description/category/updated_at` and `email_services.last_used` gaps as explicit decisions, not hidden omissions.
- Phase 4 must treat `bind_card_tasks` and all `team_*` tables as live compatibility obligations.

---
*Phase: 01-compatibility-baseline*
*Contract frozen: 2026-04-05*
