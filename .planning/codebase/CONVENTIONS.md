# Coding Conventions

**Analysis Date:** 2026-04-05

## Naming Patterns

**Files:**
- Python application files use `snake_case.py`; route modules follow resource-oriented names such as `src/web/routes/team.py` and `src/web/routes/team_tasks.py`.
- Python tests use `tests/test_*.py`; one standalone diagnostic script exists at `test_bitbrowser_checkout.py`, but it sits outside the configured pytest `testpaths`.
- Go source uses lowercase package directories under `backend-go/internal/...`; tests live beside the package as `*_test.go`, for example `backend-go/internal/accounts/service_test.go`.

**Functions:**
- Python functions and methods use `snake_case`; internal helpers are prefixed with `_`, for example `_enqueue_team_write_task` in `src/web/routes/team.py`, `_build_account_lookup` in `src/services/team/sync.py`, and `_run_with_sqlite_lock_retry` in `src/database/crud.py`.
- FastAPI route handlers are verb-led `async def` functions such as `run_team_discovery`, `sync_team`, and `get_team_detail` in `src/web/routes/team.py`.
- Go exports use PascalCase (`NewService`, `ListAccounts`, `UpsertAccount` in `backend-go/internal/accounts/service.go`); package-private helpers use mixedCase (`mergeAccount`, `removeTemporaryAccountExtraData`).

**Variables:**
- Python module constants use uppercase names with underscores (`_TEAM_API_BASE_URL`, `STATIC_DIR`, `GHOST_SUCCESS_WINDOW_SECONDS` in `src/services/team/client.py`, `src/web/app.py`, and `src/services/team/sync.py`).
- Python module loggers are consistently named `logger` (`src/web/app.py`, `src/services/team/runner.py`, `src/core/openai/payment.py`).
- Go locals are short and descriptive (`req`, `resp`, `repo`, `svc`); exported struct fields stay PascalCase to match package API types (`backend-go/internal/accounts/types.go`, `backend-go/internal/accounts/service.go`).

**Types:**
- Python request/response models, settings structs, enums, and exceptions use PascalCase: `TeamDiscoveryRunRequest` in `src/web/routes/team.py`, `SettingDefinition` and `SettingCategory` in `src/config/settings.py`, `TeamSyncNotFoundError` in `src/services/team/sync.py`.
- Go structs and interfaces use PascalCase for exports and capability-style names for boundaries: `Repository`, `Service`, `accountsService`, and `outlookRouteService` in `backend-go/internal/accounts/service.go` and `backend-go/internal/http/router.go`.

## Code Style

**Formatting:**
- No repository-wide formatter or editor config was detected. `.editorconfig`, `ruff`, `black`, `flake8`, `mypy`, and `golangci-lint` config files are absent from the project root.
- Python follows 4-space indentation, blank lines between import groups, and frequent module/function docstrings in Chinese (`src/web/app.py`, `src/config/settings.py`, `src/services/outlook/base.py`).
- Preserve file-local typing style. Newer Python modules use `from __future__ import annotations` plus built-in generics and `|` unions (`src/services/team/client.py`, `src/services/team/sync.py`, `src/web/routes/team.py`). Older modules still use `Optional`, `List`, and `Dict` from `typing` (`src/web/app.py`, `src/database/crud.py`, `src/config/settings.py`).
- Go follows `gofmt` layout: grouped imports, tab indentation, constructors named `New...`, and early returns (`backend-go/internal/accounts/service.go`, `backend-go/internal/http/router.go`, `backend-go/internal/accounts/http/handlers.go`).

**Linting:**
- Not detected as enforced tooling. Keep edits aligned with the surrounding file instead of normalizing the whole repository.
- Python uses local exceptions to the dominant style when needed, for example `# pragma: no cover` around optional dependency fallback in `src/services/team/client.py`.

## Import Organization

**Order:**
1. Standard library imports first.
2. Third-party imports second.
3. Local package imports last.

**Path Aliases:**
- Python has no custom alias config. Service/data modules often import from the project root package (`src.services...`, `src.database...`), as seen in `src/services/team/sync.py`.
- Python web modules mix package-relative imports inside `src/web/...`, for example `src/web/app.py` imports `..config.settings` and `.routes`.
- Go imports use full module paths rooted at `github.com/dou-jiang/codex-console/backend-go/...` and add aliases when names would collide, such as `internalhttp`, `accountshttp`, `registrationhttp`, and `accountspkg` in `backend-go/internal/http/router.go`, `backend-go/internal/accounts/http/handlers_test.go`, and `backend-go/tests/e2e/accounts_flow_test.go`.

## Error Handling

**Patterns:**
- Python validates early and raises domain exceptions or `HTTPException` at route boundaries (`src/services/team/client.py`, `src/services/team/sync.py`, `src/web/routes/team.py`).
- Python write paths wrap `commit` and `rollback` explicitly rather than hiding transaction state, as in `src/database/crud.py`.
- Python retries transient SQLite lock failures through small helpers instead of open-coded loops (`src/database/crud.py`).
- Go service and repository code returns `(value, error)` and wraps lower-level failures with `fmt.Errorf("context: %w", err)` (`backend-go/internal/accounts/service.go`).
- Go HTTP adapters translate decode and service errors with `http.Error` and centralize JSON writing in helper functions (`backend-go/internal/accounts/http/handlers.go`).

## Logging

**Framework:** Python uses the standard `logging` module. Go uses the standard `log` package.

**Patterns:**
- Python operational modules define `logger = logging.getLogger(__name__)` at module scope (`src/web/app.py`, `src/core/http_client.py`, `src/services/team/runner.py`).
- Python log messages are operator-facing and often Chinese; keep that tone for new runtime logs.
- Go logging is concentrated in command entrypoints such as `backend-go/cmd/api/main.go` and `backend-go/cmd/worker/main.go`. The sampled internal packages are mostly logger-free, so keep domain/service packages quiet unless the sibling code already logs.

## Comments

**When to Comment:**
- Use short Chinese module docstrings and targeted inline comments for compatibility branches, environment differences, or state-machine edge cases (`src/web/app.py`, `src/config/settings.py`, `src/services/outlook_legacy_mail.py`).
- Prefer comments that explain why a branch exists, not what a straightforward line already states.
- Keep compatibility contracts explicit when a module preserves legacy or upstream response shapes (`src/services/team/client.py`, `backend-go/internal/http/router.go`).

**JSDoc/TSDoc:**
- Not applicable in this repository.
- Python docstrings are common on modules, classes, and non-trivial helpers. Go relies more on names and short inline comments than on exported docblocks in the sampled packages.

## Function Design

**Size:**
- Python route modules favor many small private helpers plus thin endpoints (`src/web/routes/team.py`, `src/web/routes/__init__.py`).
- Longer Python service modules still isolate parsing and merge logic into small helpers before the main workflow (`src/services/team/sync.py`, `src/database/crud.py`).
- Go service files keep orchestration methods short and move merge/normalization logic into package-private helpers (`backend-go/internal/accounts/service.go`).

**Parameters:**
- Python switches to keyword-only parameters once a function grows beyond one or two control inputs, for example `_enqueue_team_write_task` in `src/web/routes/team.py` and `_fetch_member_pages` in `src/services/team/sync.py`.
- FastAPI request bodies are modeled as `BaseModel` subclasses close to the route handler (`src/web/routes/team.py`).
- Go passes explicit request structs through service layers (`ListAccountsRequest`, `UpsertAccountRequest` in `backend-go/internal/accounts/service.go`) and keeps interfaces narrow (`backend-go/internal/accounts/http/handlers.go`).

**Return Values:**
- Python commonly returns plain `dict[str, Any]` payloads, ORM models, or Pydantic models instead of dedicated DTO wrappers (`src/services/team/client.py`, `src/web/routes/team.py`).
- Go services return typed structs and errors; `map[string]any` is mostly reserved for compatibility JSON payloads and tests (`backend-go/internal/accounts/service.go`, `backend-go/internal/jobs/http/handlers.go`).

## Module Design

**Exports:**
- Python exposes package APIs through `__init__.py` re-exports and `__all__`, including lazy exports in `src/services/team/__init__.py`.
- FastAPI route mounting is centralized in `src/web/routes/__init__.py`; add a new router module there after implementing it.
- Go packages expose constructors and request/response types from the package root, while HTTP adapters live in nested `http` subpackages such as `backend-go/internal/accounts/http`.

**Barrel Files:**
- Python barrel files are used and should be updated when public package APIs change: `src/services/team/__init__.py`, `src/services/outlook/__init__.py`, and `src/web/routes/__init__.py`.
- Go does not use barrel files. Package boundaries are the organizational unit.

---

*Convention analysis: 2026-04-05*
