# backend-go

Go backend bootstrap for Codex Console.

## Current Scope

- Go control API bootstrap
- PostgreSQL / Redis bootstrap
- Phase 1 jobs schema and sqlc repository
- Minimal jobs control API
- Minimal Asynq queue + worker execution loop

## Not Yet Migrated

- Real registration state machine
- Python legacy worker bridge
- Payment and bind-card flows
- Production-grade retry / compensation / failure recovery policies

## Environment

Copy `.env.example` and adjust values for your local PostgreSQL / Redis:

```bash
cp .env.example .env
```

## Commands

- `make test`
- `make test-e2e`
- `make sqlc-generate`
- `make migrate-up`
- `make verify-phase1`
- `make run-api`
- `make run-worker`

## Guardrails

- `make sqlc-generate` now fails fast if `sqlc` is missing from `PATH`
- `make migrate-up` now fails fast if `goose` or `DATABASE_URL` is missing
- `make test-e2e` and `make verify-phase1` require `BACKEND_GO_BASE_URL`, so they cannot silently pass against no target
