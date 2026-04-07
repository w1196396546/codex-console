# backend-go

Go backend bootstrap for Codex Console.

## Current Scope

- Go control API bootstrap
- PostgreSQL / Redis bootstrap
- Registration runtime and worker execution loop
- Management APIs for accounts, settings, logs, email services, and uploader configs
- Payment / bind-card and Team domain APIs mounted through the Go router

## Not Yet Migrated

- Final production cutover / rollback execution
- Python-first startup assets and production-path decommission
- Explicit retirement or isolation of residual Python bridge code
- Production-grade retry / compensation / failure recovery policies

## Environment

Copy `.env.example` and adjust values for your local PostgreSQL / Redis:

```bash
cp .env.example .env
```

## Target Production Topology

Phase 5 的目标生产后端路径是：

- Go API：`go run ./cmd/api`
- Go worker：`go run ./cmd/worker`
- PostgreSQL：`DATABASE_URL`
- Redis：`REDIS_ADDR`

Python Web UI 可以在过渡期继续作为页面兼容壳或本地观察壳存在，但不应继续承担生产后端关键路径。真正的 cutover/rollback 判断应以仓库根目录的 `scripts/verify_phase5_cutover.sh` 和 Phase 5 verification artifact 为准。

`docker-compose.yml` 现在的默认启动面也应以这条 Go topology 为准；Python Web UI 只应通过可选兼容 profile 启动。

## Commands

- `make test`
- `make test-migrations`
- `make test-migrations-pg`
- `make test-e2e`
- `make sqlc-generate`
- `make migrate-up`
- `make verify-phase1`
- `make verify-phase5`
- `make run-api`
- `make run-worker`

## Guardrails

- `make sqlc-generate` now fails fast if `sqlc` is missing from `PATH`
- `make migrate-up` now fails fast if `goose` or `DATABASE_URL` is missing
- `make test-migrations` runs the migration contract tests in `db/migrations`
- `make test-migrations-pg` runs the real PostgreSQL/goose upgrade verification without a system goose binary; it only skips when neither `MIGRATION_TEST_DATABASE_URL` nor `DATABASE_URL` is set
- `make test-e2e` and `make verify-phase1` require `BACKEND_GO_BASE_URL`, so they cannot silently pass against no target
- `make verify-phase5` runs the repo-root Phase 5 cutover verification harness and keeps live checks explicitly gated when no real environment is configured

## Cutover Notes

- `README.md`、本文件和运行脚本必须共同指向同一条 Go-owned backend cutover 拓扑。
- 如果 Go cutover 验证失败，先停止 Go API / Go worker，再回到既有 Python 兼容启动路径；不要在 rollback 路径不明确时提前删除 Python 入口。
