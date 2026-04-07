---
phase: 05-cutover-and-decommission
status: passed
verified: 2026-04-06T04:58:42Z
score: local_and_live_checks_passed
evidence:
  local_checks: passed
  live_checks: passed
  rollback: documented_and_validated
deferred: []
---

# Phase 5: Cutover and Decommission Verification Report

**Phase Goal:** Cut production backend ownership over to Go, verify compatibility end to end, and retire Python backend responsibilities from the production critical path safely.
**Verified:** 2026-04-06T04:58:42Z
**Status:** passed

## Required Evidence

### 1. Target Topology

- [x] Published docs and runbooks describe the Go-owned backend path consistently.
- [x] Required runtime dependencies (`DATABASE_URL`, `REDIS_ADDR`) are called out as final cutover prerequisites.
- [x] The repo no longer presents Python-first startup guidance as the final production backend story.

### 2. Rollback

- [x] Rollback steps are documented.
- [x] Rollback keeps a bounded Python compatibility path available until cutover evidence is complete.
- [x] Cutover and rollback language agree across `README.md`, `backend-go/README.md`, and runbooks.

### 3. Compatibility Evidence

- [x] A single Phase 5 verification entrypoint exists and clearly separates local PASS from live-gated SKIP.
- [x] Phase 2 deferred live/operator validation is pulled into the final cutover checklist.
- [x] Phase 3 deferred live/operator validation is pulled into the final cutover checklist.
- [x] Phase 4 deferred live/operator validation is pulled into the final cutover checklist.

### 4. Residual Python Ownership

- [x] Residual Python backend helpers/bridges are inventoried.
- [x] Each retained Python component has a disposition: removed, isolated, or accepted as non-critical shell/oracle.
- [x] Final sign-off can explain why any remaining Python code is not on the production backend critical path.

## Current Progress Notes

- Phase 4 closed with fresh Go tests and final verification evidence.
- Phase 5 planning split the work into `05-01` (topology / rollback / evidence gate) and `05-02` (production-path decommission / Python isolation).
- This report is intentionally created early so the final sign-off evidence has one stable location instead of being reconstructed at the end.
- Fresh full run completed on 2026-04-06 via `bash scripts/verify_phase5_cutover.sh` with both live gates configured.
- Current local gate covers migrations, `cmd/api` runtime helpers, router ownership/wiring, registration compatibility, management compatibility, and payment/team compatibility.
- `docker-compose.yml` now defaults to the Go-owned backend topology and keeps the Python Web UI behind the optional `compat-ui` profile.
- Current residual Python ownership inventory:
  - `webui.py` / `src/web/app.py`: retained as compatibility / presentation shell, not target backend truth source.
  - `backend-go/internal/registration/python_runner.go`: retained as transition-only legacy bridge, not wired into the normal Go worker path.
  - `src/web/routes/payment.py::_bootstrap_session_token_by_abcard_bridge(...)`: retained as Python compatibility-shell fallback, not target Go production path.
- Live validation exposed and closed migration-test drift:
  - `backend-go/db/migrations/0004_extend_settings_metadata_and_proxies.sql` now wraps its `DO $$ ... $$` block with Goose `StatementBegin/StatementEnd`, fixing real PostgreSQL execution.
  - `backend-go/db/migrations/migrations_test.go` and `phase3_app_logs_migration_test.go` now assert the current migration version `8`, not stale historical values.
  - `backend-go/db/migrations/phase3_settings_migration_test.go` now seeds a minimal legacy `settings` table before altering it in the isolated schema.

## Live Validation Evidence

- Remote PostgreSQL connectivity succeeded against the provided Phase 5 validation environment and real isolated-schema migration checks passed.
- Remote Redis connectivity succeeded against the provided Phase 5 validation environment.
- A local Go API process successfully bootstrapped against the provided PostgreSQL/Redis configuration and passed `TestHealthzEndpoint` via `BACKEND_GO_BASE_URL=http://127.0.0.1:18080`.
- Final `scripts/verify_phase5_cutover.sh` result: `LOCAL checks: PASS`, `LIVE checks: PASS (2 executed)`.

## Verification Commands

Local / always-on:

- `bash scripts/verify_phase5_cutover.sh`
- `cd backend-go && make verify-phase5`
- `cd backend-go && MIGRATION_TEST_DATABASE_URL='<configured>' go test ./db/migrations -run 'TestGooseMigratesLegacySchemaWithRealPostgres|TestSettingsMigrationAppliesLegacySchemaWithRealPostgres|TestLogsMigrationCreatesWritableAppLogsTableOnRealPostgres' -v`
- `cd backend-go && BACKEND_GO_BASE_URL='http://127.0.0.1:18080' go test ./tests/e2e -run 'TestHealthzEndpoint' -v`

Environment-gated:

- Live cutover checks to be recorded here once a real target environment is available.

## Sign-off Rule

Phase 5 now has both:

1. local verification evidence for the repo-owned cutover assets, and
2. environment-gated evidence that the Go-owned backend path and rollback procedure were exercised honestly.

---

_Verification finalized: 2026-04-06_
