# Stack Research

**Domain:** Brownfield Python-to-Go backend migration for Codex Console
**Researched:** 2026-04-05
**Confidence:** HIGH

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Go | 1.25.x | Primary backend runtime for the final migrated system | `backend-go/` already uses it successfully, so the remaining work should converge on the existing runtime rather than introduce another stack |
| Chi | 5.x | Compatibility HTTP router for `/api/*` and `/api/ws/*` | It is already used in `backend-go/internal/http/router.go` and is small enough to mirror current Python route shapes closely |
| PostgreSQL + pgx/sqlc + goose | Current repo versions | Durable shared schema, query layer, and migrations | The Go code already persists accounts and service configs here, making it the natural source of truth for the final backend |
| Redis + Asynq | Current repo versions | Long-running job orchestration, pause/resume/cancel, and worker fan-out | Go already uses this stack for registration/job execution, which is a better fit than Python's process-local task maps |

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `backend-go/internal/nativerunner` | repo-local | Native OpenAI registration/token-completion runtime | Use for every registration flow that can leave the Python bridge behind |
| `httptest` + existing Go tests | standard library + repo-local | Contract regression checks for HTTP/task behavior | Use while replacing Python-only endpoints so payload compatibility stays visible |
| Existing `templates/` + `static/js/` clients | repo-local | Compatibility consumers during backend cutover | Keep them unchanged until Go parity is proven |
| Python `uv` / `pytest` toolchain | current repo versions | Transition-only parity checks against the legacy backend | Keep only while Python remains a reference implementation |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `make test`, `make test-e2e`, `make verify-phase1` | Go verification entry points | Extend these instead of inventing a second migration-specific test harness |
| `sqlc` + `goose` | Query generation and schema migration | Keep schema evolution explicit and reviewable |
| `uv sync` + `pytest` | Legacy reference execution | Use only while comparing Go behavior against Python |

## Installation

```bash
# Go migration runtime
cd backend-go
go mod download
make sqlc-generate
make test

# Transition-only Python reference environment
cd ..
uv sync
pytest
```

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| Extend the existing `backend-go/` runtime | Start a separate clean-room Go repo | Only if the current repo structure becomes impossible to untangle, which is not justified yet |
| PostgreSQL as the final source of truth | Keep SQLite as the primary operational database | Only for short-lived local development, not for the final migrated backend |
| Strangler migration with compatibility facade | Big-bang swap of all backend responsibilities | Only if there were no existing users, clients, or persisted data to protect |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| Big-bang backend replacement | Too many hidden parity gaps across routes, payloads, and side effects | Domain-by-domain migration with regression evidence |
| Schema redesign during migration | It mixes compatibility risk with migration risk | Preserve current shapes first, then clean up later |
| Frontend rewrite during backend cutover | It hides backend regressions behind client-side churn | Keep the current templates/static JS until parity is complete |

## Stack Patterns by Variant

**If a domain already has Go foundations:**
- Extend the existing Go package and keep the route contract stable
- Because this reduces duplication and keeps the roadmap focused on the delta

**If a domain is still Python-only:**
- Add a Go compatibility facade first, then migrate the implementation behind it
- Because route and payload parity must become observable before full cutover

## Version Compatibility

| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| `go 1.25.0` | `chi/v5`, `pgx/v5`, `asynq v0.26.x` | Already verified by the current `backend-go/go.mod` |
| `pyproject.toml` Python 3.10+ | Existing Python reference stack | Needed only while Python remains a migration oracle |

## Sources

- Local codebase analysis - `.planning/codebase/STACK.md`
- Local architecture analysis - `.planning/codebase/ARCHITECTURE.md`
- Go backend scope statement - `backend-go/README.md`
- Go runtime entry points - `backend-go/cmd/api/main.go`, `backend-go/cmd/worker/main.go`

---
*Stack research for: Brownfield Python-to-Go backend migration for Codex Console*
*Researched: 2026-04-05*
