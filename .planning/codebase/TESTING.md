# Testing Patterns

**Analysis Date:** 2026-04-05

## Test Framework

**Runner:**
- Python uses `pytest>=7.0.0`; configuration is declared in `pyproject.toml` under `[tool.pytest.ini_options]` with `testpaths = ["tests"]`.
- Go uses the standard library `go test`; convenience targets are defined in `backend-go/Makefile` (`test`, `test-migrations`, `test-e2e`).

**Assertion Library:**
- Python uses plain `assert`, `pytest.raises`, `pytest.mark.parametrize`, and `pytest.MonkeyPatch`, as seen in `tests/test_team_client.py`, `tests/test_team_sync_service.py`, and `tests/test_bitbrowser_connect_script.py`.
- Go uses the standard `testing` package with `t.Fatal`, `t.Fatalf`, `errors.Is`, and direct JSON decoding; no `testify`-style assertion library is present in `backend-go/go.mod`.

**Run Commands:**
```bash
pytest
cd backend-go && go test ./...
cd backend-go && go test ./db/migrations -v
cd backend-go && BACKEND_GO_BASE_URL=http://127.0.0.1:18080 go test ./tests/e2e -v
python test_bitbrowser_checkout.py
```

## Test File Organization

**Location:**
- Python tests are centralized in `tests/` with 36 `test_*.py` files. One additional top-level diagnostic script exists at `test_bitbrowser_checkout.py`.
- Go unit and integration tests are colocated with their packages under `backend-go/...`; end-to-end coverage lives in `backend-go/tests/e2e`.

**Naming:**
- Python files use `tests/test_*.py`, for example `tests/test_accounts_routes.py` and `tests/test_team_sync_service.py`.
- Go files use `*_test.go`, for example `backend-go/internal/accounts/service_test.go` and `backend-go/internal/registration/http/handlers_test.go`.
- Go package names alternate between same-package white-box tests (`package mail` in `backend-go/internal/nativerunner/mail/outlook_test.go`) and external black-box tests (`package http_test`, `package config_test`, `package e2e_test`).

**Structure:**
```text
tests/
  test_accounts_routes.py
  test_registration_routes.py
  test_team_routes.py
  test_team_sync_service.py
  ...
test_bitbrowser_checkout.py          # standalone real-browser diagnostic script
backend-go/internal/.../*_test.go    # package-level unit/integration tests
backend-go/tests/e2e/*_test.go       # compatibility/e2e flows
```

## Test Structure

**Suite Organization:**
```python
def _build_session(db_name: str):
    runtime_dir = Path("tests_runtime")
    runtime_dir.mkdir(exist_ok=True)
    ...

def test_run_team_discovery_returns_accepted_payload_with_ws_channel(monkeypatch):
    client = _create_client(monkeypatch, "team_routes_discovery.db")
    response = client.post("/api/team/discovery/run", json={"ids": [1]})
    assert response.status_code == 202
```

```go
func TestStartBatchRejectsInvalidSchedulingOptions(t *testing.T) {
    tests := []struct {
        name string
        req  registration.BatchStartRequest
        want error
    }{...}

    for _, test := range tests {
        t.Run(test.name, func(t *testing.T) {
            _, err := svc.StartBatch(context.Background(), test.req)
            if !errors.Is(err, test.want) {
                t.Fatalf("expected %v, got %v", test.want, err)
            }
        })
    }
}
```

**Patterns:**
- Setup pattern: Python creates ephemeral SQLite databases under `tests_runtime/` through `DatabaseSessionManager` and `Base.metadata.create_all`, then wires those into FastAPI or service tests (`tests/test_team_routes.py`, `tests/test_team_sync_service.py`, `tests/test_accounts_routes.py`).
- Setup pattern: Go creates in-memory services or handwritten fakes, then mounts real routers or services around them (`backend-go/internal/accounts/service_test.go`, `backend-go/internal/registration/http/handlers_test.go`, `backend-go/tests/e2e/accounts_flow_test.go`).
- Teardown pattern: Python closes sessions with `finally: session.close()` or context managers. Go uses `defer server.Close()` for `httptest.NewServer` instances.
- Assertion pattern: Python compares full dict/JSON payloads and status transitions directly. Go checks exact fields with `t.Fatalf` after decoding JSON into `map[string]any` or typed structs.

## Mocking

**Framework:** Python uses `pytest` monkeypatching plus handwritten fake classes. Go uses handwritten fake structs, in-memory repositories, `httptest.NewRecorder`, and `httptest.NewServer`.

**Patterns:**
```python
monkeypatch.setattr(team_module, "get_db", fake_get_db)
monkeypatch.setattr(team_module, "_schedule_team_task", lambda task_uuid: scheduled.append(task_uuid), raising=False)
client = TestClient(create_app())
```

```go
router := internalhttp.NewRouter(nil, fakeAccountsService{response: ...})
rec := httptest.NewRecorder()
req := httptest.NewRequest(http.MethodGet, "/api/accounts?page=1&page_size=10", nil)
router.ServeHTTP(rec, req)
```

**What to Mock:**
- Python mocks route dependencies (`get_db`, `get_settings`), registration engines, upload clients, HTTP transports, and time/task-manager behavior (`tests/test_registration_task_binding.py`, `tests/test_anyauto_register_flow.py`, `tests/test_cpa_upload.py`).
- Go mocks service or repository interfaces and queue boundaries with local fake structs (`backend-go/internal/accounts/service_test.go`, `backend-go/internal/registration/http/handlers_test.go`, `backend-go/internal/jobs/worker_test.go`).
- Go uses `httptest.NewServer` for external HTTP-like dependencies in provider and compatibility tests (`backend-go/internal/nativerunner/mail/outlook_test.go`, `backend-go/internal/nativerunner/auth/client_test.go`, `backend-go/tests/e2e/jobs_flow_test.go`).

**What NOT to Mock:**
- Python usually exercises real SQLite behavior instead of mocking ORM sessions, especially for CRUD and route tests (`tests/test_team_crud.py`, `tests/test_team_routes.py`, `tests/test_payment_routes.py`).
- Go compatibility and e2e tests prefer real `chi` routers and actual HTTP requests against `httptest.NewServer` rather than mocking handlers (`backend-go/internal/registration/http/integration_test.go`, `backend-go/tests/e2e/accounts_flow_test.go`).

## Fixtures and Factories

**Test Data:**
```python
owner = Account(email="owner@example.com", ...)
team = upsert_team(...)
membership = upsert_team_membership(...)
```

```go
repo := &fakeRepository{foundAccount: Account{Email: "user@example.com"}}
service := NewService(repo)
saved, err := service.UpsertAccount(context.Background(), UpsertAccountRequest{...})
```

**Location:**
- No shared `conftest.py` or fixture package is present.
- Python helper builders are defined inside the test module that uses them, for example `_build_session`, `_create_client`, `_make_account`, and `_seed_owner_and_team` in `tests/test_team_routes.py`, `tests/test_team_sync_service.py`, and `tests/test_accounts_routes.py`.
- Go fake constructors and helper assertions usually live at the bottom of the same test file, for example `newRegistrationRouter` in `backend-go/internal/registration/http/handlers_test.go` and `fakeAccountsService` in `backend-go/internal/accounts/http/handlers_test.go`.

## Coverage

**Requirements:** None enforced.
- No `pytest-cov`, coverage config, or Go coverage target was detected in `pyproject.toml`, `requirements.txt`, or `backend-go/Makefile`.
- `.github/workflows/build.yml` builds release artifacts but does not run the Python or Go suites.

**View Coverage:**
```bash
Not configured in repository state.
```

## Test Types

**Unit Tests:**
- Python unit-style tests focus on pure parsing and domain rules: `tests/test_team_client.py`, `tests/test_team_utils.py`, `tests/test_temp_mail_service.py`, `tests/test_anyauto_oauth_client.py`.
- Go unit-style tests focus on service merge logic, provider behavior, and config normalization: `backend-go/internal/accounts/service_test.go`, `backend-go/internal/nativerunner/mail/outlook_test.go`, `backend-go/internal/config/config_test.go`.

**Integration Tests:**
- Python integration tests wire real FastAPI app instances plus SQLite-backed sessions (`tests/test_team_routes.py`, `tests/test_accounts_routes.py`, `tests/test_registration_routes.py`, `tests/test_payment_routes.py`).
- Go integration tests hit actual routers, repositories, or HTTP layers with in-memory collaborators (`backend-go/internal/registration/http/handlers_test.go`, `backend-go/internal/http/router_test.go`, `backend-go/internal/jobs/repository_test.go`).

**E2E Tests:**
- Go has explicit e2e coverage in `backend-go/tests/e2e`.
- A Python e2e framework is not detected.
- `test_bitbrowser_checkout.py` is a manual real-browser diagnostic script with a `__main__` entrypoint, not part of the default `pytest` collection because `pyproject.toml` restricts `testpaths` to `tests`.

## Common Patterns

**Async Testing:**
```python
summary = asyncio.run(sync_team_memberships(session, team_id=team.id, client=client))
payload = asyncio.run(registration_module.get_task_logs("task-runtime-log"))
```
- `pytest-asyncio` markers are not used in the sampled suite. Async Python code is executed explicitly with `asyncio.run(...)`.
- Go does not use a separate async test framework; concurrency-heavy tests rely on `t.Parallel()`, helper queues, and HTTP/WebSocket harnesses (`backend-go/internal/nativerunner/token_completion_test.go`, `backend-go/internal/registration/ws/task_socket_test.go`).

**Error Testing:**
```python
with pytest.raises(TeamSyncNotFoundError, match="team 999 not found"):
    asyncio.run(sync_team_memberships(session, team_id=999, client=FakeTeamSyncClient()))
```

```go
_, err := svc.StartBatch(context.Background(), test.req)
if !errors.Is(err, test.want) {
    t.Fatalf("expected %v, got %v", test.want, err)
}
```
- Go negative-path tests frequently use table-driven `t.Run(...)` cases and `t.Parallel()` in pure unit/provider suites (`backend-go/internal/registration/batch_service_test.go`, `backend-go/internal/nativerunner/mail/outlook_test.go`).

---

*Testing analysis: 2026-04-05*
