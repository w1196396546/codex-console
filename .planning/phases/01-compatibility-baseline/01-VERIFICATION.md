---
phase: 01-compatibility-baseline
verified: 2026-04-05T09:40:59Z
status: passed
score: 3/3 must-haves verified
re_verification:
  previous_status: gaps_found
  previous_score: 2/3
  gaps_closed:
    - "Migration safety rails exist before domain cutover begins, including compatibility fixtures, operational checks, and a clear auth/runtime boundary decision."
  gaps_remaining: []
  regressions: []
---

# Phase 1: Compatibility Baseline Verification Report

**Phase Goal:** Freeze the remaining Python-to-Go migration contract so later phases can move domains without breaking current clients or stored data.
**Verified:** 2026-04-05T09:40:59Z
**Status:** passed
**Re-verification:** Yes — after gap closure

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | The remaining Python-only route surface and client dependencies are captured in a parity matrix with explicit Go ownership targets. | ✓ VERIFIED | `01-route-parity-inventory.json` parses cleanly and contains 170 records: 140 `remaining-python`, 24 `shared-compat`, 6 `go-baseline`, plus 2 websocket channels. `src/web/routes/__init__.py` still mounts the documented Python domains, and `01-route-client-parity-matrix.md` names concrete template/static-js consumers across registration, accounts, settings, upload configs, payment, logs, and team. |
| 2 | Shared data contracts are documented for migrated and not-yet-migrated domains, including how PostgreSQL replaces or adapts legacy Python data semantics. | ✓ VERIFIED | `01-shared-schema-contract.md` covers shared and Python-only tables and records the known schema gaps (`settings.description/category/updated_at`, `email_services.last_used`, `bind_card_tasks`, `app_logs`, `proxies`, `team_*`). Source checks confirm those Python fields/tables still exist and that Go migrations still stop short of those metadata/runtime tables. `01-runtime-boundary-contract.md` also matches current auth/runtime facts in `src/web/app.py` and current websocket/task semantics. |
| 3 | Migration safety rails exist before domain cutover begins, including compatibility fixtures, operational checks, and a clear auth/runtime boundary decision. | ✓ VERIFIED | `01-compat-fixture-manifest.md`, `01-ops-compat-runbook.md`, `scripts/verify_phase1_compat_baseline.sh`, and `tests/test_phase1_compat_baseline_script.py` all exist and are wired together. `uv run pytest tests/test_phase1_compat_baseline_script.py -q` passes (`3 passed`), and `uv run bash scripts/verify_phase1_compat_baseline.sh` exits `0` after running the mandatory Python, frontend, Go migration, and Go e2e compatibility suites. |

**Score:** 3/3 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `.planning/phases/01-compatibility-baseline/01-route-parity-inventory.json` | Machine-readable parity inventory | ✓ VERIFIED | `gsd-tools verify artifacts` passes for Plan `01-01`; JSON parses and includes all ownership states plus websocket transport. |
| `.planning/phases/01-compatibility-baseline/01-route-client-parity-matrix.md` | Reviewable route/client parity matrix | ✓ VERIFIED | `gsd-tools verify artifacts` passes for Plan `01-01`; matrix contains domain sections and concrete client files including `static/js/payment.js`, `static/js/logs.js`, `static/js/accounts_overview.js`, `static/js/accounts_state_actions.js`, `static/js/outlook_account_selector.js`, and `static/js/registration_log_buffer.js`. |
| `.planning/phases/01-compatibility-baseline/01-shared-schema-contract.md` | Shared/Python-only storage contract | ✓ VERIFIED | `gsd-tools verify artifacts` passes for Plan `01-02`; required tables and gap strings are present and source-backed. |
| `.planning/phases/01-compatibility-baseline/01-runtime-boundary-contract.md` | Auth/runtime/task semantics contract | ✓ VERIFIED | `gsd-tools verify artifacts` passes for Plan `01-02`; current `webui_auth` behavior, `/payment` exception, websocket channels, and Go preconditions are documented accurately. |
| `.planning/phases/01-compatibility-baseline/01-compat-fixture-manifest.md` | Requirement-to-fixture map | ✓ VERIFIED | `gsd-tools verify artifacts` passes for Plan `01-03`; all four requirement IDs appear with explicit `ready` / `partial` / `missing` states. |
| `scripts/verify_phase1_compat_baseline.sh` | Single fail-fast Phase 1 verification entrypoint | ✓ VERIFIED | Script is syntax-valid, `uv`-friendly, and currently green. Advisory Team pytest is documented but not forced into the Phase 1 blocking gate. |
| `.planning/phases/01-compatibility-baseline/01-ops-compat-runbook.md` | Stop-ship and handoff runbook | ✓ VERIFIED | `gsd-tools verify artifacts` passes for Plan `01-03`; runbook points to the script, lists stop-ship conditions, and preserves the Phase 1 handoff boundary. |

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| `src/web/routes/__init__.py`, `src/web/app.py`, templates, and `static/js/*` | `01-route-parity-inventory.json` | Mounted route prefixes, websocket paths, and concrete client consumers | ✓ WIRED | Inventory domains and paths align with the current Python route mounts and `/api` / `/api/ws` mounting in `src/web/app.py`. |
| `01-route-parity-inventory.json` | `01-route-client-parity-matrix.md` | Shared route/path/method/client metadata | ✓ WIRED | `gsd-tools verify key-links` passes for Plan `01-01`. |
| Python ORM models and Go migrations | `01-shared-schema-contract.md` | Table ownership, column gaps, and storage-policy notes | ✓ WIRED | Contract claims match actual model/migration evidence for shared tables and Python-only tables. |
| `src/web/app.py`, `src/web/routes/websocket.py`, `src/web/routes/registration.py`, `backend-go/internal/http/router.go` | `01-runtime-boundary-contract.md` | Auth boundary and task/websocket semantics | ✓ WIRED | Current `webui_auth` cookie flow, `/payment` exception, `/api/ws/task/{task_uuid}`, `/api/ws/batch/{batch_id}`, `log_offset`, and `log_next_offset` are all source-backed. |
| `01-compat-fixture-manifest.md` | `scripts/verify_phase1_compat_baseline.sh` | Shared `pytest`, `node --test`, and `go test` command set | ✓ WIRED | `gsd-tools verify key-links` passes for Plan `01-03`. |
| `scripts/verify_phase1_compat_baseline.sh` | `01-ops-compat-runbook.md` | Script named as the required automation entrypoint | ✓ WIRED | `gsd-tools verify key-links` passes for Plan `01-03`, and the runbook still describes the script as the Phase 1 gate. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| --- | --- | --- | --- | --- |
| `scripts/verify_phase1_compat_baseline.sh` | Python contract result | `uv run pytest tests/test_registration_routes.py tests/test_accounts_routes.py tests/test_settings_routes.py tests/test_payment_routes.py -q` | Yes; `53 passed` | ✓ FLOWING |
| `scripts/verify_phase1_compat_baseline.sh` | Frontend contract result | `node --test tests/frontend/registration_log_buffer.test.mjs tests/frontend/accounts_state_actions.test.mjs tests/frontend/accounts_team_entry.test.mjs tests/frontend/auto_team.test.mjs` | Yes; `21 passed` | ✓ FLOWING |
| `scripts/verify_phase1_compat_baseline.sh` | Go migration contract result | `cd backend-go && go test ./db/migrations -v` | Yes; suite passes and real-Postgres check skips explicitly without env vars | ✓ FLOWING |
| `scripts/verify_phase1_compat_baseline.sh` | Go compatibility result | `cd backend-go && go test ./tests/e2e -run 'TestRecentAccountsCompatibilityEndpoint|TestRegistrationCompatibilityFlow|TestRegistrationWebSocketCompatibility|TestRegistrationBatchCompatibilityFlow' -v` | Yes; selected compatibility tests pass | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| Phase 1 guard test holds the stabilized gate contract | `uv run pytest tests/test_phase1_compat_baseline_script.py -q` | `3 passed in 0.01s` | ✓ PASS |
| Full Phase 1 verification entrypoint runs green in current workspace | `uv run bash scripts/verify_phase1_compat_baseline.sh` | Exit `0`; Python `53 passed`, frontend `21 passed`, Go migration suite passed, Go compatibility suite passed | ✓ PASS |
| Optional live checks degrade explicitly instead of silently | `uv run bash scripts/verify_phase1_compat_baseline.sh` | `SKIP live checks: BACKEND_GO_BASE_URL is not set` and `SKIP live checks: MIGRATION_TEST_DATABASE_URL or DATABASE_URL is not set` | ✓ PASS |

### Requirements Coverage

Status in this table reflects whether Phase 1 established the required baseline contract and verification evidence for the mapped requirement IDs, not whether later migration phases are already implemented.

| Requirement | Source Plan | Description | Status | Evidence |
| --- | --- | --- | --- | --- |
| `COMP-01` | `01-01`, `01-03` | Existing clients can call every remaining Python-only backend capability through a Go-owned route using the same path and HTTP method. | ✓ SATISFIED FOR PHASE 1 BASELINE | The parity inventory and matrix freeze every remaining Python-owned route, identify existing Go baseline routes, and assign explicit ownership targets for later migration phases. |
| `COMP-02` | `01-01`, `01-02`, `01-03` | Existing clients receive compatible JSON field names, status values, and error semantics from migrated Go endpoints. | ✓ SATISFIED FOR PHASE 1 BASELINE | Parity scope is recorded per route, task/websocket status semantics are frozen in the runtime contract, and the current compatibility script exercises registration/accounts/websocket parity checks. |
| `DATA-01` | `01-02` | Existing persisted records remain readable and writable after Go takeover without manual reshaping. | ✓ SATISFIED FOR PHASE 1 BASELINE | The shared schema contract explicitly maps shared tables, Python-only tables, field-level gaps, and the PostgreSQL/Redis production prerequisites without pretending the later domain migrations are already done. |
| `OPS-01` | `01-03` | Go backend operational controls are sufficient to replace Python backend duties safely in production for all migrated domains. | ✓ SATISFIED FOR PHASE 1 BASELINE | Phase 1 now has a green verification entrypoint, requirement-to-fixture manifest, and stop-ship runbook. The previous blocker was the non-green gate; that blocker is closed in the current workspace. |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- | --- |
| Phase 1 artifacts | - | No placeholder markers, TODO-style stubs, or broken artifact/key-link checks were found in the verified Phase 1 docs/scripts/tests. | ℹ️ Info | Verification runs did surface pre-existing framework deprecation warnings in app code, but they do not block the Phase 1 compatibility baseline. |

### Gaps Summary

No remaining Phase 1 goal-blocking gaps were found in this re-verification.

The previous blocker is closed: the verification entrypoint is now `uv`-friendly, the Team pytest suite is treated as advisory instead of a false Phase 1 hard gate, and the manifest/runbook/script/test contract is internally consistent and green in the current workspace.

---

_Verified: 2026-04-05T09:40:59Z_
_Verifier: Claude (gsd-verifier)_
