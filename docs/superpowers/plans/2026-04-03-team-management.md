# Team Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a native Team 管理模块 in `codex-console` that discovers Team 母号 from local accounts, persists Team/member/task state, supports Team sync and batch invite flows, exposes a usable `/auto-team` UI, and explicitly never auto-refreshes RT or auto-registers invited 子号 accounts.

**Architecture:** Add an isolated Team domain (`src/database/team_*`, `src/services/team/*`, `src/web/routes/team*.py`) and keep existing large account/registration files as integration edges only. Backend owns Team discovery, relation backfill, sync, invite, membership actions, and task persistence; frontend consumes those contracts through a Team-first `/auto-team` UI plus the minimum required account-page badges/entry points.

**Tech Stack:** FastAPI, SQLAlchemy, Jinja templates, vanilla JavaScript, existing WebSocket task stream (`/api/ws/task/{task_uuid}`), pytest, Node `node:test`

**Test Runtime Prerequisite:** Run `uv sync --extra dev` once in this worktree before Python route tests, then use `uv run --extra dev pytest ...` for Python tests and `node --test ...` for frontend tests. The repo’s plain `uv sync` omits `httpx`, so `fastapi.testclient` tests will fail during collection unless the `dev` extra is installed.

---

## File Map

### Team domain backend

- Create: `src/database/team_models.py`
  - Define `Team`, `TeamMembership`, `TeamTask`, `TeamTaskItem`
- Create: `src/database/team_crud.py`
  - Team upsert, membership merge, task persistence, aggregate queries
- Modify: `src/database/__init__.py`
  - Export Team models/CRUD
- Modify: `src/database/session.py`
  - Ensure Team metadata is imported and SQLite migration hook can create new Team tables
- Create: `src/services/team/utils.py`
  - Email normalization, status priority, active-member counting, seat math
- Create: `src/services/team/client.py`
  - Upstream Team API parsing and error classification
- Create: `src/services/team/tasks.py`
  - Task creation, accepted response payloads, same-Team write-task mutual exclusion
- Create: `src/services/team/discovery.py`
  - Discover Team 母号 from local accounts
- Create: `src/services/team/sync.py`
  - Sync Team info + merged memberships
- Create: `src/services/team/relation.py`
  - Backfill `local_account_id`, enforce manual-bind priority, relink memberships after account creation/import
- Create: `src/services/team/invite.py`
  - Batch invite local accounts / manual emails without mutating child accounts
- Create: `src/services/team/membership_actions.py`
  - `revoke`, `remove`, `bind-local-account`
- Create: `src/services/team/__init__.py`
  - Service exports

### Team API layer

- Create: `src/web/routes/team.py`
  - Discovery, Team list/detail, sync, invite, membership actions
- Create: `src/web/routes/team_tasks.py`
  - Team task list/detail endpoints
- Modify: `src/web/routes/__init__.py`
  - Register Team routers
- Modify: `src/web/task_manager.py`
  - Add minimal helper glue if Team tasks need explicit enqueue / broadcast wrappers
- Modify: `src/web/routes/accounts.py`
  - Add `team_role_badges`, `team_relation_summary`, `team_relation_count`
- Modify: `src/database/crud.py`
  - Add the thinnest possible post-account-upsert hook for Team relation backfill

### Team UI

- Modify: `src/web/app.py`
  - Keep `/auto-team` as Team 管理入口 and pass through any new route aliases if needed
- Modify: `templates/auto_team.html`
  - Replace placeholder with Team management shell
- Create: `static/js/auto_team.js`
  - Team page state, filters, task subscription, membership actions, and only the minimum Team-page/account-page query parsing
- Modify: `static/css/style.css`
  - Team page cards, badges, tabs, empty states, action bars

### Accounts-page integration

- Modify: `templates/accounts.html`
- Modify: `static/js/accounts.js`
  - Add Team badge rendering and the minimum “进入 Team 管理” entry from selected accounts / current filters

### Tests

- Create: `tests/test_team_crud.py`
- Create: `tests/test_team_utils.py`
- Create: `tests/test_team_client.py`
- Create: `tests/test_team_discovery_service.py`
- Create: `tests/test_team_relation_service.py`
- Create: `tests/test_team_sync_service.py`
- Create: `tests/test_team_invite_service.py`
- Create: `tests/test_team_membership_actions.py`
- Create: `tests/test_team_task_service.py`
- Create: `tests/test_team_routes.py`
- Create: `tests/test_team_tasks_routes.py`
- Modify: `tests/test_accounts_routes.py`
- Modify: `tests/test_account_crud.py`
- Modify: `tests/test_token_refresh_statuses.py`
- Create: `tests/frontend/auto_team.test.mjs`
- Create: `tests/frontend/accounts_team_entry.test.mjs`
- Modify: `tests/test_static_asset_versioning.py`

---

### Task 1: Enable Team Test Runtime And Persistence Skeleton

**Files:**
- Modify: `pyproject.toml`
- Modify: `requirements.txt`
- Create: `tests/test_team_crud.py`
- Create: `src/database/team_models.py`
- Create: `src/database/team_crud.py`
- Modify: `src/database/__init__.py`
- Modify: `src/database/session.py`
- Test: `tests/test_team_crud.py`

- [ ] **Step 1: Write the failing Team persistence tests**

```python
def test_upsert_team_keeps_owner_plus_upstream_account_unique(session):
    team_a = team_crud.upsert_team(
        session,
        owner_account_id=1,
        upstream_account_id="acc_team_1",
        team_name="Alpha",
    )
    team_b = team_crud.upsert_team(
        session,
        owner_account_id=1,
        upstream_account_id="acc_team_1",
        team_name="Alpha v2",
    )

    assert team_a.id == team_b.id
    assert team_b.team_name == "Alpha v2"


def test_membership_upsert_uses_normalized_email(session):
    first = team_crud.upsert_membership(
        session,
        team_id=1,
        member_email=" Foo@Example.com ",
        membership_status="invited",
    )
    second = team_crud.upsert_membership(
        session,
        team_id=1,
        member_email="foo@example.com",
        membership_status="joined",
    )

    assert first.id == second.id
    assert second.member_email == "foo@example.com"
    assert second.membership_status == "joined"
```

- [ ] **Step 2: Run the new persistence tests and verify RED**

Run: `uv run --extra dev pytest tests/test_team_crud.py -q`  
Expected: FAIL because Team models/CRUD do not exist yet

- [ ] **Step 3: Implement the minimal Team persistence layer**

```python
class Team(Base):
    __tablename__ = "teams"
    id = Column(Integer, primary_key=True)
    owner_account_id = Column(Integer, ForeignKey("accounts.id"), nullable=False)
    upstream_team_id = Column(String(255))
    upstream_account_id = Column(String(255), nullable=False)


def upsert_membership(session, team_id: int, member_email: str, **payload):
    normalized_email = normalize_team_email(member_email)
    existing = session.query(TeamMembership).filter_by(
        team_id=team_id,
        member_email=normalized_email,
    ).first()
```

Implementation notes:
- Import Team models from `src/database/session.py` before `create_tables()`
- Keep new Team tables in `src/database/team_models.py`, not `src/database/models.py`
- Add `httpx` to the plain requirements only if needed for non-`uv` workflows; `pyproject.toml` must support `uv sync --extra dev`

- [ ] **Step 4: Re-run the persistence tests**

Run: `uv run --extra dev pytest tests/test_team_crud.py -q`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pyproject.toml requirements.txt src/database/team_models.py src/database/team_crud.py src/database/__init__.py src/database/session.py tests/test_team_crud.py
git commit -m "feat: add team persistence foundation"
```

---

### Task 2: Lock Down Team Utility Rules Before Service Work

**Files:**
- Create: `tests/test_team_utils.py`
- Create: `src/services/team/utils.py`
- Test: `tests/test_team_utils.py`

- [ ] **Step 1: Write the failing utility tests**

```python
def test_pick_active_memberships_prefers_joined_over_invited():
    merged = pick_active_memberships([
        {"member_email": "foo@example.com", "membership_status": "invited"},
        {"member_email": "foo@example.com", "membership_status": "joined"},
    ])

    assert merged["foo@example.com"]["membership_status"] == "joined"


def test_count_current_members_counts_unique_active_emails_only():
    memberships = [
        {"member_email": "a@example.com", "membership_status": "joined"},
        {"member_email": "a@example.com", "membership_status": "invited"},
        {"member_email": "b@example.com", "membership_status": "already_member"},
        {"member_email": "c@example.com", "membership_status": "failed"},
    ]

    assert count_current_members(memberships) == 2
```

- [ ] **Step 2: Run the utility tests and verify RED**

Run: `uv run --extra dev pytest tests/test_team_utils.py -q`  
Expected: FAIL because `src/services/team/utils.py` does not exist yet

- [ ] **Step 3: Implement the pure Team rule helpers**

```python
STATUS_PRIORITY = {
    "joined": 50,
    "already_member": 40,
    "invited": 30,
    "revoked": 20,
    "removed": 10,
    "failed": 0,
}


def normalize_team_email(value: str) -> str:
    return extract_email(value).strip().lower()
```

Implementation notes:
- Keep all email normalization, active-status priority, seat math, and badge-summary helpers pure
- Encode the spec rule: `joined > already_member > invited > revoked/removed/failed`

- [ ] **Step 4: Re-run the utility tests**

Run: `uv run --extra dev pytest tests/test_team_utils.py -q`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/services/team/utils.py tests/test_team_utils.py
git commit -m "test: codify team utility rules"
```

---

### Task 3: Build The Upstream Team Client Contract

**Files:**
- Create: `tests/test_team_client.py`
- Create: `src/services/team/client.py`
- Test: `tests/test_team_client.py`

- [ ] **Step 1: Write the failing Team client parsing tests**

```python
def test_parse_account_check_keeps_only_team_accounts():
    payload = {
        "accounts": {
            "acc_team": {
                "account": {"plan_type": "team", "name": "Alpha", "account_user_role": "account-owner"},
                "entitlement": {"subscription_plan": "team", "has_active_subscription": True},
            },
            "acc_free": {
                "account": {"plan_type": "free", "name": "Beta"},
                "entitlement": {"has_active_subscription": False},
            },
        }
    }

    parsed = parse_team_accounts(payload)
    assert [item["upstream_account_id"] for item in parsed] == ["acc_team"]
```

- [ ] **Step 2: Run the client tests and verify RED**

Run: `uv run --extra dev pytest tests/test_team_client.py -q`  
Expected: FAIL because the Team client does not exist yet

- [ ] **Step 3: Implement the minimal client + parsing layer**

```python
class TeamApiClient:
    async def get_team_accounts(self, access_token: str) -> dict:
        ...

    async def list_members(self, access_token: str, upstream_account_id: str) -> dict:
        ...

    async def list_invites(self, access_token: str, upstream_account_id: str) -> dict:
        ...
```

Implementation notes:
- Reuse the existing `curl-cffi` request style from current OpenAI/Team-related code
- Parse errors from `detail`, `error`, `error.code`, then `response.text`
- Implement the spec’s pagination stop rules for `/users`

- [ ] **Step 4: Re-run the client tests**

Run: `uv run --extra dev pytest tests/test_team_client.py -q`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/services/team/client.py tests/test_team_client.py
git commit -m "feat: add team upstream client"
```

---

### Task 4: Persist Team Tasks And Discovery Flow

**Files:**
- Create: `tests/test_team_task_service.py`
- Create: `tests/test_team_discovery_service.py`
- Create: `src/services/team/tasks.py`
- Create: `src/services/team/discovery.py`
- Modify: `src/web/task_manager.py`
- Test: `tests/test_team_task_service.py`
- Test: `tests/test_team_discovery_service.py`

- [ ] **Step 1: Write the failing task + discovery tests**

```python
def test_create_team_task_returns_pending_payload(session):
    task = team_tasks_service.create_task(
        session,
        task_type="discover_owner_teams",
        owner_account_id=7,
    )
    assert task.status == "pending"


def test_discovery_marks_account_owner_teams(session, monkeypatch):
    monkeypatch.setattr(client, "get_team_accounts", AsyncMock(return_value=[{
        "upstream_account_id": "acc_team_1",
        "team_name": "Alpha",
        "account_role_snapshot": "account-owner",
    }]))
```

- [ ] **Step 2: Run the two service test files and verify RED**

Run: `uv run --extra dev pytest tests/test_team_task_service.py tests/test_team_discovery_service.py -q`  
Expected: FAIL because task/discovery services do not exist yet

- [ ] **Step 3: Implement Team task persistence and discovery service**

```python
def build_accepted_response(task, *, ws_channel: str | None = None) -> dict:
    return {
        "success": True,
        "task_uuid": task.task_uuid,
        "task_type": task.task_type,
        "status": task.status,
        "ws_channel": ws_channel or f"/api/ws/task/{task.task_uuid}",
    }
```

Implementation notes:
- Keep same-Team write-task mutual exclusion in `src/services/team/tasks.py`
- Discovery should read local accounts and only persist Team-capable accounts
- Do not call any child refresh/register paths here

- [ ] **Step 4: Re-run the discovery/task tests**

Run: `uv run --extra dev pytest tests/test_team_task_service.py tests/test_team_discovery_service.py -q`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/services/team/tasks.py src/services/team/discovery.py src/web/task_manager.py tests/test_team_task_service.py tests/test_team_discovery_service.py
git commit -m "feat: add team discovery and task services"
```

---

### Task 5: Implement Team Sync Service With Membership Merge Rules

**Files:**
- Create: `tests/test_team_sync_service.py`
- Create: `src/services/team/sync.py`
- Modify: `src/services/team/__init__.py`
- Test: `tests/test_team_sync_service.py`

- [ ] **Step 1: Write the failing sync tests**

```python
def test_sync_team_merges_members_and_invites_by_normalized_email(session, monkeypatch):
    monkeypatch.setattr(client, "list_members", AsyncMock(return_value=[
        {"id": "user_1", "email": "Foo@Example.com", "role": "standard-user"},
    ]))
    monkeypatch.setattr(client, "list_invites", AsyncMock(return_value=[
        {"email_address": " foo@example.com ", "role": "standard-user"},
    ]))

    result = asyncio.run(sync_service.sync_team(team_id=1))
    assert result["active_member_count"] == 1
    assert result["joined_count"] == 1
    assert result["invited_count"] == 0
```

- [ ] **Step 2: Run the sync tests and verify RED**

Run: `uv run --extra dev pytest tests/test_team_sync_service.py -q`  
Expected: FAIL because sync service does not exist yet

- [ ] **Step 3: Implement the minimal Team sync flow**

```python
async def sync_team(self, team_id: int) -> dict:
    members = await self.client.list_members(...)
    invites = await self.client.list_invites(...)
    merged = merge_memberships(members, invites)
    current_members = count_current_members(merged)
```

Implementation notes:
- Use the spec’s `ghost success` window constants in one place
- Populate `local_account_id` via normalized email lookup into local `accounts`
- Treat `already_member` as active-member state for counts and Team detail tabs
- Keep `joined > already_member > invited > revoked/removed/failed`

- [ ] **Step 4: Re-run the sync tests**

Run: `uv run --extra dev pytest tests/test_team_sync_service.py -q`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/services/team/sync.py src/services/team/__init__.py tests/test_team_sync_service.py
git commit -m "feat: add team sync service"
```

---

### Task 6: Add Team Relation Backfill And Safe Rebinding

**Files:**
- Create: `tests/test_team_relation_service.py`
- Modify: `tests/test_account_crud.py`
- Create: `src/services/team/relation.py`
- Modify: `src/database/crud.py`
- Test: `tests/test_team_relation_service.py`
- Test: `tests/test_account_crud.py`

- [ ] **Step 1: Write the failing relation tests**

```python
def test_relink_account_memberships_backfills_local_account_id(session):
    account = make_account(email="foo@example.com")
    membership = make_membership(member_email=" Foo@Example.com ", local_account_id=None)

    relink_account_memberships(session, account.id, account.email)
    session.refresh(membership)
    assert membership.local_account_id == account.id


def test_manual_bind_is_not_overwritten_by_auto_relink(session):
    account = make_account(email="foo@example.com")
    membership = make_membership(member_email="foo@example.com", local_account_id=99, source="manual_bind")

    relink_account_memberships(session, account.id, account.email)
    session.refresh(membership)
    assert membership.local_account_id == 99


def test_bind_local_account_rejects_cross_email_without_confirmation(session):
    account = make_account(email="bar@example.com")
    membership = make_membership(member_email="foo@example.com", local_account_id=None)

    result = bind_local_account(session, membership.id, account.id)
    assert result["success"] is False
    assert result["error_code"] == 400
```

- [ ] **Step 2: Run the relation tests and verify RED**

Run: `uv run --extra dev pytest tests/test_team_relation_service.py tests/test_account_crud.py -q`  
Expected: FAIL because relation service/backfill hook does not exist yet

- [ ] **Step 3: Implement Team relation backfill**

```python
def relink_account_memberships(session, account_id: int, email: str) -> int:
    normalized_email = normalize_team_email(email)
    ...
```

Implementation notes:
- Trigger backfill after account create/import/merge through the thinnest possible hook in `src/database/crud.py`
- Respect `manual_bind > auto mapping`
- Keep rebind/clear logic inside `src/services/team/relation.py`, not `crud.py`
- First phase bind/rebind only allows same-email binding and must write an audit log entry on success

- [ ] **Step 4: Re-run the relation tests**

Run: `uv run --extra dev pytest tests/test_team_relation_service.py tests/test_account_crud.py -q`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/services/team/relation.py src/database/crud.py tests/test_team_relation_service.py tests/test_account_crud.py
git commit -m "feat: add team relation backfill hooks"
```

---

### Task 7: Implement Invite And Membership Action Services With Child-Account Guards

**Files:**
- Create: `tests/test_team_invite_service.py`
- Create: `tests/test_team_membership_actions.py`
- Create: `src/services/team/invite.py`
- Create: `src/services/team/membership_actions.py`
- Test: `tests/test_team_invite_service.py`
- Test: `tests/test_team_membership_actions.py`

- [ ] **Step 1: Write the failing invite/action tests**

```python
def test_invite_service_never_mutates_child_account_tokens_or_status(session, monkeypatch):
    account = make_account(refresh_token="rt_1", access_token="at_1", status="active")
    invite_result = asyncio.run(invite_service.invite_account_ids(team_id=1, account_ids=[account.id]))
    session.refresh(account)
    assert account.refresh_token == "rt_1"
    assert account.access_token == "at_1"
    assert account.status == "active"
    assert invite_result["child_refresh_triggered"] is False
    assert invite_result["child_registration_triggered"] is False


def test_invite_service_logs_child_guard_messages(session):
    result = asyncio.run(invite_service.invite_manual_emails(team_id=1, emails=["foo@example.com"]))
    assert "未触发子号自动注册" in result["logs"][-1]
    assert "未触发子号自动刷新 RT" in result["logs"][-1]


def test_invite_flow_never_calls_child_refresh_or_registration(monkeypatch):
    refresh_spy = Mock(side_effect=AssertionError("child refresh must not be called"))
    register_spy = Mock(side_effect=AssertionError("child registration must not be called"))
    monkeypatch.setattr("src.core.openai.token_refresh.refresh_account_token", refresh_spy)
    monkeypatch.setattr("src.core.register.RegistrationEngine.run", register_spy)
```

- [ ] **Step 2: Run the invite/action tests and verify RED**

Run: `uv run --extra dev pytest tests/test_team_invite_service.py tests/test_team_membership_actions.py -q`  
Expected: FAIL because invite/action services do not exist yet

- [ ] **Step 3: Implement the minimal invite and membership-action services**

```python
result_payload = {
    "child_refresh_triggered": False,
    "child_registration_triggered": False,
}
```

Implementation notes:
- Support local-account invite and manual-email invite paths
- Return `already_member` as success state and expose it in the active-member view
- Write both structured booleans and the explicit guard log lines
- Add mock/spy coverage proving child refresh/register functions are never called from invite paths or invite follow-up confirmation
- Illegal action matrix:
  - `invited -> revoke` allowed
  - `joined/already_member -> remove` allowed
  - `joined/already_member -> revoke` returns `400`
  - `invited -> remove` returns `400`
  - `bind-local-account` first phase only allows same-email binding; cross-email bind returns `400`

- [ ] **Step 4: Re-run the invite/action tests**

Run: `uv run --extra dev pytest tests/test_team_invite_service.py tests/test_team_membership_actions.py -q`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/services/team/invite.py src/services/team/membership_actions.py tests/test_team_invite_service.py tests/test_team_membership_actions.py
git commit -m "feat: add team invite and membership actions"
```

---

### Task 8: Expose Team APIs, Task APIs, And Accounts Aggregation Contracts

**Files:**
- Create: `tests/test_team_routes.py`
- Create: `tests/test_team_tasks_routes.py`
- Create: `src/web/routes/team.py`
- Create: `src/web/routes/team_tasks.py`
- Modify: `src/web/routes/__init__.py`
- Modify: `src/web/routes/accounts.py`
- Modify: `tests/test_accounts_routes.py`
- Modify: `tests/test_token_refresh_statuses.py`
- Test: `tests/test_team_routes.py`
- Test: `tests/test_team_tasks_routes.py`
- Test: `tests/test_accounts_routes.py`
- Test: `tests/test_token_refresh_statuses.py`

- [ ] **Step 1: Write the failing Team API tests**

```python
def test_team_discovery_run_returns_accepted_payload(client):
    response = client.post("/api/team/discovery/run", json={"ids": [1]})
    payload = response.json()
    assert payload["status"] == "pending"
    assert payload["ws_channel"] == f"/api/ws/task/{payload['task_uuid']}"


def test_team_memberships_list_returns_id_for_actions(client):
    response = client.get("/api/team/teams/1/memberships?status=active")
    assert "id" in response.json()["items"][0]


def test_team_memberships_support_binding_filters(client):
    response = client.get("/api/team/teams/1/memberships?binding=external")
    assert response.status_code == 200


def test_invite_routes_never_call_registration_or_account_refresh(client, monkeypatch):
    monkeypatch.setattr("src.core.openai.token_refresh.refresh_account_token", Mock(side_effect=AssertionError))
    monkeypatch.setattr("src.core.register.RegistrationEngine.run", Mock(side_effect=AssertionError))


def test_team_task_detail_exposes_child_guard_logs(client):
    response = client.get("/api/team/tasks/task-123")
    payload = response.json()
    assert "未触发子号自动注册" in payload["logs"]
    assert "未触发子号自动刷新 RT" in payload["logs"]
```

- [ ] **Step 2: Run the route/account tests and verify RED**

Run: `uv run --extra dev pytest tests/test_team_routes.py tests/test_team_tasks_routes.py tests/test_accounts_routes.py tests/test_token_refresh_statuses.py -q`  
Expected: FAIL because Team routers and account aggregations do not exist yet

- [ ] **Step 3: Implement the Team APIs and account aggregation edge**

```python
class AccountResponse(BaseModel):
    ...
    team_role_badges: list[str] = []
    team_relation_summary: Optional[dict] = None
    team_relation_count: int = 0
```

Implementation notes:
- Keep all async-write entrypoints on the accepted-response contract
- Reuse `/api/ws/task/{task_uuid}` only; do not add a Team-specific websocket
- Keep `src/web/routes/accounts.py` to aggregation-only logic
- Add negative route tests that fail immediately if Team invite endpoints reach registration or child-refresh entrypoints
- Ensure Team task detail responses surface the two child-guard log lines for UI/task-center verification

- [ ] **Step 4: Re-run the route/account tests**

Run: `uv run --extra dev pytest tests/test_team_routes.py tests/test_team_tasks_routes.py tests/test_accounts_routes.py tests/test_token_refresh_statuses.py -q`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/web/routes/team.py src/web/routes/team_tasks.py src/web/routes/__init__.py src/web/routes/accounts.py tests/test_team_routes.py tests/test_team_tasks_routes.py tests/test_accounts_routes.py tests/test_token_refresh_statuses.py
git commit -m "feat: add team api contracts"
```

---

### Task 9: Build Team Overview, Detail Tabs, And Task Center UI

**Files:**
- Modify: `src/web/app.py`
- Modify: `templates/auto_team.html`
- Create: `static/js/auto_team.js`
- Modify: `static/css/style.css`
- Create: `tests/frontend/auto_team.test.mjs`
- Modify: `tests/test_static_asset_versioning.py`
- Test: `tests/frontend/auto_team.test.mjs`
- Test: `tests/test_static_asset_versioning.py`

- [ ] **Step 1: Write the failing Team page frontend tests**

```javascript
test('auto team page defaults to active teams and active-member tab', () => {
  const state = deriveInitialTeamState({ search: '', status: '' });
  assert.equal(state.filters.status, 'active');
  assert.equal(state.detailTab, 'active');
});

test('auto team page discovery and sync-batch actions subscribe then refresh overview', async () => {
  const flow = await runAcceptedTaskFlow({
    action: 'discoverOwners',
    taskUuid: 'task-1',
    wsChannel: '/api/ws/task/task-1',
  });
  assert.deepEqual(flow.refreshedEndpoints, ['/api/team/teams']);
});
```

- [ ] **Step 2: Run the Team page tests and verify RED**

Run: `node --test tests/frontend/auto_team.test.mjs`  
Expected: FAIL because `static/js/auto_team.js` does not exist yet

- [ ] **Step 3: Implement Team overview + detail + task center shell**

```javascript
export function deriveInitialTeamState(query) {
  return {
    filters: { status: query.status || 'active', search: query.search || '' },
    detailTab: query.tab || 'active',
  };
}
```

Implementation notes:
- Replace the placeholder in `templates/auto_team.html`
- Include Team overview list, Team detail tabs (`active/invited/external`), and task center region in the initial DOM
- Wire the page to `/api/team/teams`, `/api/team/teams/{id}`, `/api/team/tasks`, and `/api/ws/task/{task_uuid}`
- Add “发现母号” and “批量同步” buttons on the Team overview toolbar
- Cover empty-state -> discovered-state transition in `tests/frontend/auto_team.test.mjs`

- [ ] **Step 4: Re-run the Team page tests**

Run: `node --test tests/frontend/auto_team.test.mjs && uv run --extra dev pytest tests/test_static_asset_versioning.py -q`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/web/app.py templates/auto_team.html static/js/auto_team.js static/css/style.css tests/frontend/auto_team.test.mjs tests/test_static_asset_versioning.py
git commit -m "feat: add team overview ui"
```

---

### Task 10: Implement Membership Action UI, Batch Invite Modal, And Empty/Conflict States

**Files:**
- Modify: `templates/auto_team.html`
- Modify: `static/js/auto_team.js`
- Modify: `static/css/style.css`
- Modify: `templates/accounts.html`
- Modify: `static/js/accounts.js`
- Create: `tests/frontend/accounts_team_entry.test.mjs`
- Modify: `tests/frontend/auto_team.test.mjs`
- Test: `tests/frontend/auto_team.test.mjs`
- Test: `tests/frontend/accounts_team_entry.test.mjs`

- [ ] **Step 1: Extend the failing frontend tests for actions and modal states**

```javascript
test('membership actions require membership id and refresh detail after success', () => {
  const action = buildMembershipActionRequest({ id: 7, action: 'remove' });
  assert.deepEqual(action, { membershipId: 7, action: 'remove' });
});

test('batch invite modal renders full-team conflict state', () => {
  assert.equal(resolveInviteAvailability({ status: 'full', syncStatus: 'success' }).disabled, true);
});
```

- [ ] **Step 2: Run the frontend action tests and verify RED**

Run: `node --test tests/frontend/auto_team.test.mjs tests/frontend/accounts_team_entry.test.mjs`  
Expected: FAIL because action/modal helpers do not exist yet

- [ ] **Step 3: Implement membership action UI + invite modal**

```javascript
function afterSuccessfulMembershipAction(teamId, tab) {
  return Promise.all([
    loadTeamDetail(teamId),
    loadMemberships(teamId, tab),
  ]);
}
```

Implementation notes:
- Support `remove`, `revoke`, and `bind-local-account`
- Include Team full / 409 conflict / empty state messaging
- Add the minimum “进入 Team 管理” button to `accounts.html` and `static/js/accounts.js`

- [ ] **Step 4: Re-run the frontend action tests**

Run: `node --test tests/frontend/auto_team.test.mjs tests/frontend/accounts_team_entry.test.mjs`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add templates/auto_team.html static/js/auto_team.js static/css/style.css templates/accounts.html static/js/accounts.js tests/frontend/auto_team.test.mjs tests/frontend/accounts_team_entry.test.mjs
git commit -m "feat: add team membership action ui"
```

---

### Task 11: Run Focused Verification And Capture Final Regression Evidence

**Files:**
- Modify: `docs/superpowers/specs/2026-04-03-team-management-design.md` (only if implementation reveals a contract mismatch)
- Test: `tests/test_team_crud.py`
- Test: `tests/test_team_utils.py`
- Test: `tests/test_team_client.py`
- Test: `tests/test_team_discovery_service.py`
- Test: `tests/test_team_relation_service.py`
- Test: `tests/test_team_sync_service.py`
- Test: `tests/test_team_invite_service.py`
- Test: `tests/test_team_membership_actions.py`
- Test: `tests/test_team_task_service.py`
- Test: `tests/test_team_routes.py`
- Test: `tests/test_team_tasks_routes.py`
- Test: `tests/test_accounts_routes.py`
- Test: `tests/test_token_refresh_statuses.py`
- Test: `tests/frontend/auto_team.test.mjs`
- Test: `tests/frontend/accounts_team_entry.test.mjs`
- Test: `tests/test_static_asset_versioning.py`

- [ ] **Step 1: Run the focused Python Team suite**

Run:

```bash
uv run --extra dev pytest \
  tests/test_team_crud.py \
  tests/test_team_utils.py \
  tests/test_team_client.py \
  tests/test_team_discovery_service.py \
  tests/test_team_relation_service.py \
  tests/test_team_sync_service.py \
  tests/test_team_invite_service.py \
  tests/test_team_membership_actions.py \
  tests/test_team_task_service.py \
  tests/test_team_routes.py \
  tests/test_team_tasks_routes.py \
  tests/test_accounts_routes.py \
  tests/test_token_refresh_statuses.py -q
```

Expected: PASS

- [ ] **Step 2: Run the focused frontend Team suite**

Run:

```bash
node --test \
  tests/frontend/auto_team.test.mjs \
  tests/frontend/accounts_team_entry.test.mjs
```

Expected: PASS

- [ ] **Step 3: Run the multi-role aggregation regression**

Run:

```bash
uv run --extra dev pytest tests/test_accounts_routes.py -k "team_role_badges or multi_role" -q
```

Expected: PASS, including the case where one local account is Team A 母号 and Team B 子号

- [ ] **Step 4: Smoke-check static asset versioning**

Run: `uv run --extra dev pytest tests/test_static_asset_versioning.py -q`  
Expected: PASS

- [ ] **Step 5: Review the diff**

Run: `git diff --stat && git diff`  
Expected: Only Team-domain files, the minimal account-page integration, and related tests/assets changed

- [ ] **Step 6: Commit**

```bash
git add .
git commit -m "test: verify team management implementation"
```

---

## Optional Follow-up After Core Team Module Is Stable

- Add paid-overview handoff from `templates/accounts_overview.html` / `static/js/accounts_overview.js`
- Add registration-page CTA from `templates/index.html` / `static/js/app.js`
- Add payment-page CTA from `templates/payment.html` / `static/js/payment.js`
- Add settings-page quick-open from `templates/settings.html` / `static/js/settings.js`

These are intentionally **not** part of the first implementation wave so the Team domain, Team page, and account-page integration can stabilize first.

---

## Execution Notes

- Prefer implementing Tasks 1-8 before touching the Team UI
- Backend tasks 4-8 can be split across GPT-5.4 subagents if each subagent owns a disjoint write set
- Frontend Tasks 9-10 should start only after Task 8 stabilizes the Team route/API contract
- Do not claim success off the full repo `pytest` baseline until the feature-specific suite above is green and the `dev` extra is installed
