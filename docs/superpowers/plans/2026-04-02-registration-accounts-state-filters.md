# Registration And Accounts State Filters Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver unified Outlook execution-state filtering on the registration page plus RT filtering and single/batch status updates on the accounts page without breaking existing batch-selection semantics.

**Architecture:** Keep the existing route/template structure, but formalize the shared business-state contract at the edges: registration routes define the Outlook execution status semantics and accounts routes define RT filter + batch update semantics. Frontend code consumes those contracts through focused helpers and existing page controllers instead of introducing a new cross-page framework.

**Tech Stack:** FastAPI, SQLAlchemy, Jinja templates, vanilla JavaScript, Node `node:test`, pytest

**Test Runtime Prerequisite:** Python tests in this repo should run through `uv run pytest ...`, not `python3 -m pytest ...`, because the project dependencies live in the managed `.venv`.

---

## File Map

### Registration state flow

- Modify: `src/web/routes/registration.py`
  - Keep Outlook candidate fields authoritative
  - Encode the “skip only registered_complete” rule
  - Add a single helper for deriving execution status if needed during implementation
- Modify: `static/js/outlook_account_selector.js`
  - Extend the selector helper to map and filter the three execution states deterministically
- Modify: `static/js/app.js`
  - Consume the new execution-state filter options
  - Default-select only executable Outlook accounts
  - Keep additive multi-select behavior
- Modify: `templates/index.html`
  - Replace generic status choices with business-state choices
  - Update action text from “只选未注册” to executable-state wording if implementation chooses that path
- Modify: `tests/test_registration_routes.py`
  - Add route/helper-level regression tests for execution-state and skip semantics
- Modify: `tests/frontend/outlook_account_selector.test.mjs`
  - Extend selector tests for status mapping, default-selection, and additive behavior

### Accounts management flow

- Modify: `src/web/routes/accounts.py`
  - Extend `AccountResponse`
  - Add `refresh_token_state` filtering to `GET /accounts`
  - Tighten `PATCH /accounts/{id}` / `POST /accounts/batch-update` contract validation
  - Support `select_all + filters` in batch update
- Modify: `templates/accounts.html`
  - Add RT filter UI
  - Add batch status action entry point
- Modify: `static/js/accounts.js`
  - Track RT filter in current filters
  - Pass the new query/body parameters
  - Add single-account status actions and batch status actions
- Create: `static/js/accounts_state_actions.js`
  - Pure helpers for query/body mapping, action availability, and confirmation text
- Create: `tests/test_accounts_routes.py`
  - New focused route-contract tests for accounts filtering and status updates
- Create: `tests/frontend/accounts_state_actions.test.mjs`
  - Frontend TDD coverage for RT filter propagation and batch payload/status action behavior

### Shared verification/doc updates

- Modify: `tests/test_static_asset_versioning.py`
  - Only if new static assets are introduced during implementation

---

### Task 1: Codify Registration Route Semantics

**Files:**
- Modify: `tests/test_registration_routes.py`
- Modify: `src/web/routes/registration.py`
- Test: `tests/test_registration_routes.py`

- [ ] **Step 1: Write the failing route/helper tests**

```python
def test_outlook_registered_candidate_without_refresh_token_needs_token_refresh():
    account = SimpleNamespace(status="active", refresh_token="")

    assert registration_module._needs_token_refresh(account) is True
    assert registration_module._is_account_registration_complete(account) is False


def test_registered_complete_candidates_are_the_only_ones_skipped():
    complete = SimpleNamespace(status="active", refresh_token="rt-1")
    incomplete = SimpleNamespace(status="active", refresh_token="")

    assert registration_module._is_account_registration_complete(complete) is True
    assert registration_module._is_account_registration_complete(incomplete) is False
```

- [ ] **Step 2: Run the route test file to verify the new assertions fail or expose the missing contract**

Run: `uv run pytest tests/test_registration_routes.py -q`
Expected: FAIL on the new contract assertions or missing helper coverage

- [ ] **Step 3: Implement the minimal registration-route contract**

```python
def _derive_outlook_execution_state(account):
    if _is_account_registration_complete(account):
        return "registered_complete"
    if account:
        return "registered_needs_token_refresh"
    return "unregistered"
```

Implementation notes:
- Keep the existing route payload fields
- Make `start_outlook_batch_registration()` skip only `registered_complete`
- Do not introduce a fourth executable state

- [ ] **Step 4: Run the route test file again**

Run: `uv run pytest tests/test_registration_routes.py -q`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add tests/test_registration_routes.py src/web/routes/registration.py
git commit -m "test: codify outlook execution state contract"
```

---

### Task 2: Lock Registration Selector Behavior With Frontend Tests

**Files:**
- Modify: `tests/frontend/outlook_account_selector.test.mjs`
- Modify: `static/js/outlook_account_selector.js`
- Test: `tests/frontend/outlook_account_selector.test.mjs`

- [ ] **Step 1: Capture the current selector red-test baseline**

Run: `node --test tests/frontend/outlook_account_selector.test.mjs`
Expected: RED on the current `createInitialSelectedIds` / `filterAccounts` contract, establishing that this task must absorb the existing mismatch before adding new behavior

- [ ] **Step 2: Extend the selector test file with the missing state-contract failures**

```javascript
test('mapExecutionState classifies registered accounts without RT as token refresh candidates', () => {
  assert.equal(mapExecutionState({
    is_registered: true,
    needs_token_refresh: true,
    is_registration_complete: false,
  }), 'registered_needs_token_refresh');
});

test('createInitialSelectedIds only selects executable accounts', () => {
  const selected = createInitialSelectedIds(sampleAccounts);
  assert.deepEqual([...selected].sort((a, b) => a - b), [1, 2]);
});

test('mapExecutionState resolves inconsistent booleans through the documented priority', () => {
  assert.equal(mapExecutionState({
    is_registered: true,
    needs_token_refresh: false,
    is_registration_complete: false,
  }), 'registered_needs_token_refresh');
});

test('reloading accounts preserves default selection for executable states only', () => {
  const selected = createInitialSelectedIds(reloadedAccounts);
  assert.deepEqual([...selected].sort((a, b) => a - b), [1, 2]);
});

test('mapExecutionState resolves is_registered=false and is_registration_complete=true through priority', () => {
  assert.equal(mapExecutionState({
    is_registered: false,
    is_registration_complete: true,
  }), 'registered_complete');
});

test('mapExecutionState treats missing fields as unregistered', () => {
  assert.equal(mapExecutionState({}), 'unregistered');
});

test('mapExecutionState tolerates invalid field types without creating a fourth state', () => {
  assert.equal(mapExecutionState({
    is_registered: 'yes',
    is_registration_complete: 0,
  }), 'registered_needs_token_refresh');
});
```

- [ ] **Step 3: Run the selector test file and verify RED**

Run: `node --test tests/frontend/outlook_account_selector.test.mjs`
Expected: RED on the existing mismatch plus the new state-contract assertions

- [ ] **Step 4: Implement the minimal selector updates**

```javascript
function mapExecutionState(account) {
  if (account.is_registration_complete) return 'registered_complete';
  if (account.is_registered) return 'registered_needs_token_refresh';
  return 'unregistered';
}
```

Implementation notes:
- Reuse the same priority defined in the spec
- Keep additive select/deselect helpers pure
- Extend filtering to support `executionState`
- Ensure the current failing baseline is absorbed instead of worked around

- [ ] **Step 5: Re-run the selector tests**

Run: `node --test tests/frontend/outlook_account_selector.test.mjs`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add tests/frontend/outlook_account_selector.test.mjs static/js/outlook_account_selector.js
git commit -m "test: cover outlook selector execution states"
```

---

### Task 3: Wire Registration Page UI To The New Execution States

**Files:**
- Modify: `templates/index.html`
- Modify: `static/js/app.js`
- Modify: `tests/frontend/outlook_account_selector.test.mjs`
- Test: `tests/frontend/outlook_account_selector.test.mjs`

- [ ] **Step 1: Add failing registration-page behavior tests to the selector test file**

Extend the existing frontend test file with behavior-level assertions, for example:

```javascript
test('selectExecutableVisibleAccounts excludes registered_complete entries', () => {
  const nextSelected = selectExecutableVisibleAccounts(new Set(), visibleAccounts);
  assert.deepEqual([...nextSelected].sort((a, b) => a - b), [1, 2]);
});

test('filterAccounts supports executionState=registered_needs_token_refresh', () => {
  const filtered = filterAccounts(accounts, { executionState: 'registered_needs_token_refresh' });
  assert.deepEqual(filtered.map((item) => item.id), [2]);
});
```

Use `tests/test_static_asset_versioning.py` only if the implementation actually introduces a new static asset reference.

- [ ] **Step 2: Run the frontend test file to verify RED**

Run: `node --test tests/frontend/outlook_account_selector.test.mjs`
Expected: FAIL on the newly added behavior assertions

- [ ] **Step 3: Implement the minimal page wiring**

```javascript
outlookAccountFilters.executionState = elements.outlookAccountStatusFilter.value || 'all';
selectedOutlookAccountIds = createInitialSelectedIds(outlookAccounts);
```

Implementation notes:
- Replace generic status labels with:
  - `全部`
  - `未注册`
  - `已注册待补Token`
  - `注册已完成`
- Rename the quick action from “只选未注册” to executable wording if the UI copy would otherwise be misleading
- Preserve additive selection and hidden-selection summary behavior

- [ ] **Step 4: Run focused frontend verification**

Run:

```bash
node --test tests/frontend/outlook_account_selector.test.mjs
node --check static/js/outlook_account_selector.js
node --check static/js/app.js
```

Expected: all commands PASS

- [ ] **Step 5: Commit**

```bash
git add templates/index.html static/js/app.js tests/frontend/outlook_account_selector.test.mjs
git commit -m "feat: wire registration outlook execution filters"
```

---

### Task 4: Add Accounts Route Tests For RT Filtering And Status Update Semantics

**Files:**
- Create: `tests/test_accounts_routes.py`
- Modify: `src/web/routes/accounts.py`
- Test: `tests/test_accounts_routes.py`

- [ ] **Step 1: Write failing route tests**

```python
def test_list_accounts_filters_by_refresh_token_state_has():
    response = accounts_module.list_accounts(refresh_token_state="has")
    assert [item.email for item in response.accounts] == ["has-rt@example.com"]


def test_batch_update_with_select_all_false_ignores_filters():
    result = accounts_module.batch_update_accounts(
        BatchUpdateRequest(
            ids=[target_id],
            status="banned",
            select_all=False,
            status_filter="active",
            refresh_token_state_filter="missing",
        )
    )
    assert result["updated_count"] == 1
```

Required scenarios in this file:
- `GET /accounts?refresh_token_state=has`
- `GET /accounts?refresh_token_state=missing`
- invalid `refresh_token_state` returns `400`
- `GET /accounts` with `refresh_token_state + status + email_service + search` returns the combined filtered set
- `PATCH /accounts/{id}` updates one status
- `POST /accounts/batch-update` with `ids=[...]` and `select_all=False` updates exactly those ids
- `POST /accounts/batch-update` with `select_all=False` ignores filters
- `POST /accounts/batch-update` with `select_all=True` ignores `ids`
- `POST /accounts/batch-update` with `select_all=True + refresh_token_state_filter=invalid` returns `400`
- invalid `refresh_token_state_filter` returns `400`
- `select_all + filters` updates the service-side filtered set
- `select_all + filters` with 0 matched accounts returns `updated_count == 0`
- partial failures return `updated_count + errors`

- [ ] **Step 2: Run the new test file to verify RED**

Run: `uv run pytest tests/test_accounts_routes.py -q`
Expected: FAIL because the route contract does not exist yet

- [ ] **Step 3: Implement the minimal backend contract**

```python
if refresh_token_state == "has":
    query = query.filter(Account.refresh_token.isnot(None), Account.refresh_token != "")
elif refresh_token_state == "missing":
    query = query.filter(or_(Account.refresh_token.is_(None), Account.refresh_token == ""))
else:
    raise HTTPException(status_code=400, detail="无效的 refresh_token_state")
```

Implementation notes:
- Extend `AccountResponse` with `has_refresh_token`
- Add `refresh_token_state` to `list_accounts()`
- Extend `BatchUpdateRequest` to include:
  - `select_all`
  - `status_filter`
  - `email_service_filter`
  - `search_filter`
  - `refresh_token_state_filter`
- Use the same service-side filtering semantics in batch update that the spec defines
- Validate enum-like fields before branching on `select_all`

- [ ] **Step 4: Run the route tests again**

Run: `uv run pytest tests/test_accounts_routes.py -q`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add tests/test_accounts_routes.py src/web/routes/accounts.py
git commit -m "test: add accounts route state filter coverage"
```

---

### Task 5: Create Accounts Frontend Helper Tests And Helper Module

**Files:**
- Create: `tests/frontend/accounts_state_actions.test.mjs`
- Create: `static/js/accounts_state_actions.js`
- Test: `tests/frontend/accounts_state_actions.test.mjs`

- [ ] **Step 1: Write failing frontend helper tests**

```javascript
test('buildAccountsQueryParams includes refresh_token_state when selected', () => {
  assert.equal(
    buildAccountsQueryParams({ status: 'active', refresh_token_state: 'missing' }).toString(),
    'status=active&refresh_token_state=missing'
  );
});

test('buildBatchStatusPayload maps current filters to batch-update fields', () => {
  assert.deepEqual(
    buildBatchStatusPayload({
      ids: [1, 2],
      select_all: true,
      status: 'banned',
      filters: { refresh_token_state: 'has', status: 'active' },
    }),
    {
      ids: [1, 2],
      select_all: true,
      status: 'banned',
      status_filter: 'active',
      refresh_token_state_filter: 'has',
    }
  );
});
```

Also include helper tests for:
- single-account status action labels
- batch confirmation text
- count-mismatch warning copy
- filter-change reset behavior for pagination and selection
- batch action disabled-state logic

- [ ] **Step 2: Run the new frontend helper test file to verify RED**

Run: `node --test tests/frontend/accounts_state_actions.test.mjs`
Expected: FAIL because the helper module does not exist yet

- [ ] **Step 3: Implement the minimal helper module**

```javascript
function buildBatchStatusPayload({ ids, select_all, status, filters }) {
  return {
    ids,
    select_all,
    status,
    status_filter: filters.status || null,
    email_service_filter: filters.email_service || null,
    search_filter: filters.search || null,
    refresh_token_state_filter: filters.refresh_token_state || null,
  };
}
```

- [ ] **Step 4: Re-run the helper tests**

Run: `node --test tests/frontend/accounts_state_actions.test.mjs`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add tests/frontend/accounts_state_actions.test.mjs static/js/accounts_state_actions.js
git commit -m "test: add accounts state action helpers"
```

---

### Task 6: Wire Accounts Page RT Filter And Single Status Actions

**Files:**
- Modify: `templates/accounts.html`
- Modify: `static/js/accounts.js`
- Modify: `static/js/accounts_state_actions.js`
- Modify: `tests/frontend/accounts_state_actions.test.mjs`
- Test: `tests/test_accounts_routes.py`
- Test: `tests/frontend/accounts_state_actions.test.mjs`

- [ ] **Step 1: Add the UI contract in the template**

Use exact UI labels:

```html
<select id="filter-refresh-token" class="form-select">
  <option value="">全部RT状态</option>
  <option value="has">有RT</option>
  <option value="missing">无RT</option>
</select>
```

- [ ] **Step 2: Write failing helper tests for single-account status behavior**

Extend `tests/frontend/accounts_state_actions.test.mjs` with assertions for:

```javascript
test('buildSingleStatusRequest targets the selected status', () => {
  assert.deepEqual(buildSingleStatusRequest(42, 'failed'), {
    accountId: 42,
    body: { status: 'failed' },
  });
});

test('applyFilterChange resets pagination and clears selection state', () => {
  assert.deepEqual(
    applyFilterChange({
      currentPage: 4,
      selectedIds: new Set([1, 2]),
      selectAllPages: true,
      currentFilters: { status: 'active' },
    }, { refresh_token_state: 'missing' }),
    {
      currentPage: 1,
      selectedIds: new Set(),
      selectAllPages: false,
      currentFilters: { status: 'active', refresh_token_state: 'missing' },
    }
  );
});
```

- [ ] **Step 3: Run RED checks before wiring the frontend**

Run: `uv run pytest tests/test_accounts_routes.py -q`
Run: `node --test tests/frontend/accounts_state_actions.test.mjs`
Expected: backend PASS via `uv run pytest`; frontend helper test FAIL until the helper is extended

- [ ] **Step 4: Implement the minimal accounts-page wiring**

Run backend check with:

```bash
uv run pytest tests/test_accounts_routes.py -q
```

Then wire:

```javascript
currentFilters.refresh_token_state = elements.filterRefreshToken.value;
params.append('refresh_token_state', currentFilters.refresh_token_state);
```

Implementation notes:
- Add `filterRefreshToken` to `elements`
- Add change listeners that reset pagination and selection like the existing filters
- Add single-account “设为 active/expired/banned/failed” actions under the “更多” menu
- Reuse `PATCH /accounts/{id}` instead of creating a new single-account endpoint
- Consume the pure helper module for query/body construction instead of duplicating mapping logic in `accounts.js`

- [ ] **Step 5: Run focused frontend verification**

Run:

```bash
node --test tests/frontend/accounts_state_actions.test.mjs
node --check static/js/accounts.js
node --check static/js/accounts_state_actions.js
uv run pytest tests/test_accounts_routes.py -q
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add templates/accounts.html static/js/accounts.js static/js/accounts_state_actions.js tests/frontend/accounts_state_actions.test.mjs
git commit -m "feat: add accounts RT filter and single status actions"
```

---

### Task 7: Add Batch Status Actions To Accounts Page

**Files:**
- Modify: `templates/accounts.html`
- Modify: `static/js/accounts.js`
- Modify: `static/js/accounts_state_actions.js`
- Modify: `tests/frontend/accounts_state_actions.test.mjs`
- Test: `tests/test_accounts_routes.py`
- Test: `tests/frontend/accounts_state_actions.test.mjs`

- [ ] **Step 1: Add failing batch-action frontend tests**

Extend `tests/frontend/accounts_state_actions.test.mjs` with:

```javascript
test('buildBatchStatusPayload keeps ids but batch-update ignores them when select_all=true', () => {
  const payload = buildBatchStatusPayload({
    ids: [1, 2],
    select_all: true,
    status: 'banned',
    filters: { refresh_token_state: 'missing' },
  });
  assert.equal(payload.select_all, true);
  assert.equal(payload.refresh_token_state_filter, 'missing');
});

test('buildBatchStatusResultNotice warns when confirmed count differs from updated_count', () => {
  assert.match(buildBatchStatusResultNotice({ confirmedCount: 5, updatedCount: 3 }), /筛选结果/);
});

test('shouldDisableBatchStatusAction returns true when nothing is selected', () => {
  assert.equal(shouldDisableBatchStatusAction({ selectedCount: 0, selectAllPages: false }), true);
});
```

- [ ] **Step 2: Run RED checks before implementation**

Run: `uv run pytest tests/test_accounts_routes.py -q`
Run: `node --test tests/frontend/accounts_state_actions.test.mjs`
Expected: backend PASS via `uv run pytest`; frontend helper test FAIL until batch helpers are implemented

Backend command:

```bash
uv run pytest tests/test_accounts_routes.py -q
```

- [ ] **Step 3: Implement the minimal batch-action UI**

```javascript
await api.post('/accounts/batch-update', buildBatchStatusPayload({
  ids: Array.from(selectedAccounts),
  select_all: selectAllPages,
  status: targetStatus,
  filters: currentFilters,
}));
```

Implementation notes:
- Add a toolbar dropdown or grouped buttons for:
  - `设为 active`
  - `设为 expired`
  - `设为 banned`
  - `设为 failed`
- Use `buildBatchStatusPayload()` so `select_all` works like the other bulk actions
- Map current RT filter through the helper instead of hand-assembling `refresh_token_state_filter`
- Show confirmation text with:
  - local known count before submit
  - server returned `updated_count` after submit
- If counts differ, show a warning that the filtered set changed while submitting
- Consume helper functions instead of inlining request-body mapping or result-copy logic

- [ ] **Step 4: Re-run focused verification**

Run:

```bash
node --test tests/frontend/accounts_state_actions.test.mjs
node --check static/js/accounts.js
node --check static/js/accounts_state_actions.js
uv run pytest tests/test_accounts_routes.py -q
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add templates/accounts.html static/js/accounts.js static/js/accounts_state_actions.js tests/frontend/accounts_state_actions.test.mjs
git commit -m "feat: add batch account status actions"
```

---

### Task 8: Final Verification And Integration Pass

**Files:**
- Modify: `tests/test_static_asset_versioning.py` (only if required by actual changes)
- Test: `tests/frontend/outlook_account_selector.test.mjs`
- Test: `tests/frontend/accounts_state_actions.test.mjs`
- Test: `tests/test_registration_routes.py`
- Test: `tests/test_accounts_routes.py`

- [ ] **Step 1: Run the frontend selector tests**

Run: `node --test tests/frontend/outlook_account_selector.test.mjs`
Expected: PASS

- [ ] **Step 2: Run the accounts frontend helper tests**

Run: `node --test tests/frontend/accounts_state_actions.test.mjs`
Expected: PASS

- [ ] **Step 3: Run route-focused pytest**

Run:

```bash
uv run pytest tests/test_registration_routes.py tests/test_accounts_routes.py -q
```

Expected: PASS

- [ ] **Step 4: Run JS syntax validation**

Run:

```bash
node --check static/js/outlook_account_selector.js
node --check static/js/app.js
node --check static/js/accounts_state_actions.js
node --check static/js/accounts.js
```

Expected: PASS

- [ ] **Step 5: Run diff hygiene**

Run: `git diff --check`
Expected: PASS

- [ ] **Step 6: Commit the final integration pass**

```bash
git status --short
# 仅当 Task 8 为修复 integration issue 产生新 diff 时才执行后两行
git add <only-files-changed-during-final-fix>
git commit -m "fix: resolve final integration issues"
```

If Task 8 没有新增 diff，就跳过这一步，保留 Task 1-7 的原子提交边界作为最终历史。

---

## Parallel Execution Notes

- **Parallel lane A (serial inside the lane):** Task 1 -> Task 2 -> Task 3
  - Registration route semantics and registration page wiring
- **Parallel lane B (serial inside the lane):** Task 4 -> Task 5 -> Task 6 -> Task 7
  - Accounts route semantics, helper tests, and accounts page UI/actions
- **Lane-level parallelism:** lane A and lane B can run in parallel because they touch different primary write sets until integration
- **Integration gate:** Task 8 only after both lanes are complete

Never run Task 5/6/7 in parallel with each other because they all write to the same accounts-page files.
