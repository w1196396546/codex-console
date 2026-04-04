# Phase 1 Runbook

## Goal

Bring up the `backend-go` control API and worker loop against local PostgreSQL and Redis.

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

5. Start the API in one terminal:

```bash
make run-api
```

6. Start the worker in another terminal:

```bash
make run-worker
```

## Verification

1. Run unit tests:

```bash
make test
```

2. Run the minimal healthz E2E suite:

```bash
BACKEND_GO_BASE_URL=http://127.0.0.1:18080 make test-e2e
```

3. Check the health endpoint:

```bash
curl http://127.0.0.1:18080/healthz
```

Expected response:

```text
ok
```

## Current Guardrails

- Phase 1 only guarantees the minimal jobs control plane and worker loop.
- Registration, payment, bind-card, and Python legacy worker bridge are not migrated yet.
- `make sqlc-generate` will fail if `sqlc` is not in `PATH`.
- `make migrate-up` will fail if `goose` or `DATABASE_URL` is missing.
- `make test-e2e` and `make verify-phase1` will fail fast if `BACKEND_GO_BASE_URL` is not provided.
- The current E2E suite only verifies the minimal `/healthz` path in Phase 1; it is not yet a full jobs workflow test.
