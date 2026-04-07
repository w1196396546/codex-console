# Phase 1 Runbook

## Goal

Bring up the `backend-go` control API and worker loop against local PostgreSQL and Redis.

## Current Milestone Note

This document is still the Go bootstrap baseline, but it is no longer the whole migration story.

- By Phase 5, the Go runtime already owns registration, management, payment, and Team backend domains.
- Final cutover readiness now also depends on the Phase 5 topology / rollback / compatibility gate in `scripts/verify_phase5_cutover.sh`.
- If you are validating final cutover from inside `backend-go/`, use `make verify-phase5`.
- Treat this file as the bootstrap and local bring-up reference, not the sole final sign-off checklist.

## Prerequisites

1. PostgreSQL is reachable and the `DATABASE_URL` database exists.
2. Redis is reachable at `REDIS_ADDR`.
3. `sqlc` is installed and available in `PATH`.
4. `goose` is installed and available in `PATH` if you want to run migrations through `make migrate-up`.

## Startup Steps

1. Copy the environment template and adjust credentials:

```bash
cd backend-go
cp .env.example .env
```

2. Export the required variables for the current shell:

```bash
set -a
source .env
set +a
```

3. Regenerate sqlc code if schema or queries changed:

```bash
make sqlc-generate
```

4. Apply migrations:

```bash
make migrate-up
```

5. Run migration contract tests:

```bash
make test-migrations
```

6. Run the real PostgreSQL/goose upgrade verification when you have a disposable database URL available:

```bash
MIGRATION_TEST_DATABASE_URL="$DATABASE_URL" make test-migrations-pg
```

The integration test creates an isolated schema, seeds legacy tables, runs `goose up` through the official Go API, and verifies the upgraded schema plus representative repaired data. It runs without a system goose binary. If `MIGRATION_TEST_DATABASE_URL` is unset it falls back to `DATABASE_URL`; if neither URL is available, the test skips instead of failing.

7. Start the API in one terminal:

```bash
make run-api
```

8. Start the worker in another terminal:

```bash
make run-worker
```

## Verification

1. Run unit tests:

```bash
make test
```

2. Run migration-focused checks:

```bash
make test-migrations
MIGRATION_TEST_DATABASE_URL="$DATABASE_URL" make test-migrations-pg
```

3. Run the minimal healthz E2E suite:

```bash
BACKEND_GO_BASE_URL=http://127.0.0.1:18080 make test-e2e
```

4. Check the health endpoint:

```bash
curl http://127.0.0.1:18080/healthz
```

Expected response:

```text
ok
```

## Current Guardrails

- Phase 1 itself only guaranteed the minimal jobs control plane and worker loop.
- By the current milestone, Go also covers registration, management, payment, bind-card, and Team backend routes, but final production cutover still requires explicit Phase 5 verification and rollback evidence.
- `make sqlc-generate` will fail if `sqlc` is not in `PATH`.
- `make migrate-up` will fail if `goose` or `DATABASE_URL` is missing.
- `make test-migrations-pg` exercises real `goose up` execution against a disposable PostgreSQL schema through the official Go API, so it runs without a system goose binary and only skips if a migration test database URL is unavailable.
- `make test-e2e` and `make verify-phase1` will fail fast if `BACKEND_GO_BASE_URL` is not provided.
- The current E2E suite started as a minimal Phase 1 `/healthz` check; use the Phase 5 verification entrypoint for final cutover evidence across migrated domains.
