# Phase 1 Compatibility Fixture Manifest

## Coverage Matrix

| Requirement | Surface | Python oracle | Frontend/client check | Go check | Command | Status | Follow-up owner |
|---|---|---|---|---|---|---|---|
| COMP-01 | Registration route family and task controls | `tests/test_registration_routes.py` | `tests/frontend/registration_log_buffer.test.mjs` | `backend-go/tests/e2e/jobs_flow_test.go` (`TestRegistrationCompatibilityFlow`, `TestRegistrationWebSocketCompatibility`, `TestRegistrationBatchCompatibilityFlow`) | `pytest tests/test_registration_routes.py tests/test_accounts_routes.py tests/test_settings_routes.py tests/test_payment_routes.py tests/test_team_routes.py tests/test_team_tasks_routes.py -q` | ready | Phase 2 |
| COMP-01 | Accounts management and overview route family | `tests/test_accounts_routes.py` | `tests/frontend/accounts_state_actions.test.mjs`, `tests/frontend/accounts_team_entry.test.mjs` | `backend-go/tests/e2e/accounts_flow_test.go` (`TestRecentAccountsCompatibilityEndpoint`) | `pytest tests/test_registration_routes.py tests/test_accounts_routes.py tests/test_settings_routes.py tests/test_payment_routes.py tests/test_team_routes.py tests/test_team_tasks_routes.py -q` | partial | Phase 3 |
| COMP-01 | Settings, email-services, upload-configs, and logs route families | `tests/test_settings_routes.py` | No dedicated frontend test covers the full admin surface yet | No Go parity suite yet for these domains | `pytest tests/test_registration_routes.py tests/test_accounts_routes.py tests/test_settings_routes.py tests/test_payment_routes.py tests/test_team_routes.py tests/test_team_tasks_routes.py -q` | partial | Phase 3 |
| COMP-01 | Payment route family | `tests/test_payment_routes.py` | None | No Go parity suite yet for payment domain | `uv run pytest tests/test_payment_routes.py -q` | partial | Phase 4 |
| COMP-01 | Team route family | `tests/test_team_routes.py`, `tests/test_team_tasks_routes.py` | `tests/frontend/auto_team.test.mjs` | No Go parity suite yet for team domain | `uv run pytest tests/test_team_routes.py tests/test_team_tasks_routes.py -q` | partial | Phase 4 |
| COMP-02 | Registration payload, task-log, batch-log, and websocket semantics | `tests/test_registration_routes.py` | `tests/frontend/registration_log_buffer.test.mjs` | `backend-go/tests/e2e/jobs_flow_test.go` (`TestRegistrationCompatibilityFlow`, `TestRegistrationWebSocketCompatibility`, `TestRegistrationBatchCompatibilityFlow`) | `cd backend-go && go test ./tests/e2e -run 'TestRecentAccountsCompatibilityEndpoint|TestRegistrationCompatibilityFlow|TestRegistrationWebSocketCompatibility|TestRegistrationBatchCompatibilityFlow' -v` | ready | Phase 2 |
| COMP-02 | Accounts/status payload semantics | `tests/test_accounts_routes.py` | `tests/frontend/accounts_state_actions.test.mjs`, `tests/frontend/accounts_team_entry.test.mjs` | `backend-go/tests/e2e/accounts_flow_test.go` (`TestRecentAccountsCompatibilityEndpoint`) | `cd backend-go && go test ./tests/e2e -run 'TestRecentAccountsCompatibilityEndpoint|TestRegistrationCompatibilityFlow|TestRegistrationWebSocketCompatibility|TestRegistrationBatchCompatibilityFlow' -v` | partial | Phase 3 |
| DATA-01 | Shared Go schema and migration safety for current PostgreSQL-owned tables | Python ORM models in `src/database/models.py`, `src/database/team_models.py` | None | `backend-go/db/migrations/migrations_test.go` | `cd backend-go && go test ./db/migrations -v` | ready | Phase 2 / Phase 3 |
| DATA-01 | Python-only tables: `registration_tasks`, `bind_card_tasks`, `app_logs`, `proxies`, `team_*` | `src/database/models.py`, `src/database/team_models.py` | None | No Go automation currently validates takeover for these tables | `cd backend-go && go test ./db/migrations -v` | missing | Phase 3 / Phase 4 |
| OPS-01 | Scripted compatibility baseline entrypoint | Phase 1 contract docs | None | `scripts/verify_phase1_compat_baseline.sh` orchestrates Python, frontend, and Go suites | `node --test tests/frontend/registration_log_buffer.test.mjs tests/frontend/accounts_state_actions.test.mjs tests/frontend/accounts_team_entry.test.mjs tests/frontend/auto_team.test.mjs` | ready | Phase 1 |
| OPS-01 | Live infrastructure smoke coverage for Go API/worker against PostgreSQL and Redis | `backend-go/docs/phase1-runbook.md` | None | Optional env-gated checks only | `cd backend-go && go test ./tests/e2e -run 'TestRecentAccountsCompatibilityEndpoint|TestRegistrationCompatibilityFlow|TestRegistrationWebSocketCompatibility|TestRegistrationBatchCompatibilityFlow' -v` | partial | Phase 5 |

## Canonical Commands

- `pytest tests/test_registration_routes.py tests/test_accounts_routes.py tests/test_settings_routes.py tests/test_payment_routes.py tests/test_team_routes.py tests/test_team_tasks_routes.py -q`
- `node --test tests/frontend/registration_log_buffer.test.mjs tests/frontend/accounts_state_actions.test.mjs tests/frontend/accounts_team_entry.test.mjs tests/frontend/auto_team.test.mjs`
- `cd backend-go && go test ./db/migrations -v`
- `cd backend-go && go test ./tests/e2e -run 'TestRecentAccountsCompatibilityEndpoint|TestRegistrationCompatibilityFlow|TestRegistrationWebSocketCompatibility|TestRegistrationBatchCompatibilityFlow' -v`

## Interpretation Notes

- `ready`: current tests already exercise the named compatibility surface closely enough to serve as a baseline gate.
- `partial`: there is some executable coverage, but it does not yet freeze the whole surface needed for cutover.
- `missing`: Phase 1 has documented the contract, but no existing automated suite proves that surface yet.
- Team route family remains advisory, not part of the Phase 1 green gate, because those red tests belong to the later Team migration surface rather than the current contract-freezing deliverable.

## Immediate Follow-up Signals

- `settings`, `email-services`, upload-config, and log admin APIs still need Go-side parity coverage before Phase 3 can claim completeness.
- Payment, bind-card, and team runtime semantics remain documented but under-tested on the Go side, which is expected until Phase 4.
- Team route family remains advisory, not part of the Phase 1 green gate, until Phase 4 owns the domain-level runtime fixes.
- Python-only tables remain a migration obligation even when shared Go tables already have migration tests.

---
*Phase: 01-compatibility-baseline*
*Fixture manifest created: 2026-04-05*
