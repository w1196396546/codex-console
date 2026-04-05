# Phase 1 Ops Compatibility Runbook

## Inputs

- `.planning/phases/01-compatibility-baseline/01-route-client-parity-matrix.md`
- `.planning/phases/01-compatibility-baseline/01-shared-schema-contract.md`
- `.planning/phases/01-compatibility-baseline/01-runtime-boundary-contract.md`
- `.planning/phases/01-compatibility-baseline/01-compat-fixture-manifest.md`

## Automation Entry Point

- Run `scripts/verify_phase1_compat_baseline.sh` from the repository root.
- The script is the single fail-fast compatibility gate for Phase 1.
- Optional live infrastructure checks stay env-gated; the static contract suites are always mandatory.

## Manual Review Steps

1. Confirm the `/payment` page remains documented as the current unauthenticated page-level exception and is not silently reclassified during compatibility work.
2. Confirm `/api` and `/api/ws` are documented as currently lacking a shared auth middleware/dependency boundary, and that any future hardening is explicitly scoped rather than assumed.
3. Confirm the storage split is explicit: SQLite is a Python-side legacy reference, while PostgreSQL and Redis are prerequisites for the Go-owned cutover path.
4. Confirm Python-only tables still appear as live migration obligations: `registration_tasks`, `bind_card_tasks`, `app_logs`, `proxies`, `teams`, `team_memberships`, `team_tasks`, and `team_task_items`.
5. Confirm the parity matrix still names current template/static-js consumers instead of relying on memory or route-prefix guesses.

## Stop-Ship Conditions

- Missing `remaining-python` route mappings in the Phase 1 parity inventory or parity matrix.
- Missing shared-contract coverage for `team_*` or `bind_card_tasks`.
- `scripts/verify_phase1_compat_baseline.sh` exits non-zero.
- Any later Phase 2+ execution claims parity while still needing to modify dirty backend-go business files.
- Any cutover plan assumes SQLite is sufficient for the final Go-owned production path.

## Handoff to Phase 2+

This phase freezes contracts and safety rails only.

- Phase 2 may build on the registration compatibility baseline, but it must not re-implement existing Go jobs/registration/websocket foundations.
- Phase 3 inherits the admin/config/storage gaps documented here for accounts, settings, email services, upload configs, proxies, and logs.
- Phase 4 inherits the payment and team domain gaps documented here, including Python-only runtime tables and current operator-facing behavior.
- Phase 5 must gather the external deployment inventory that is outside git before final cutover claims are made.

---
*Phase: 01-compatibility-baseline*
*Runbook created: 2026-04-05*
