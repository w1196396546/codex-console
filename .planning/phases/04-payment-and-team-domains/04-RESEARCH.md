# Phase 4: Payment and Team Domains - Research

**Researched:** 2026-04-05
**Domain:** Payment/bind-card orchestration and Team discovery/sync/invite/runtime migration for the Codex Console Go backend
**Confidence:** MEDIUM

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
### Compatibility boundary
- **D-01:** Current Python payment and team route behavior remains the compatibility oracle until Go parity is proven.
- **D-02:** Existing templates and `static/js` consumers must keep working without a frontend rewrite; preserve current `/api/payment*` and `/api/team*` contracts first.

### Scope boundary
- **D-03:** Phase 4 covers payment, bind-card tasks, subscription sync, and all team discovery/sync/invite/membership/task flows.
- **D-04:** Phase 4 must build on the completed Phase 2 runtime semantics and completed Phase 3 management/API wiring, not rework them.
- **D-05:** Final production cutover, rollback, and operator runbooks remain Phase 5 scope even if Phase 4 closes the remaining backend domain gaps.

### Implementation approach
- **D-06:** Reuse existing Go registration/accounts/uploader foundations and add the missing payment/team domain slices rather than copying Python monolith structure.
- **D-07:** Preserve current operator-facing task/session/status semantics, especially around bind-card task lifecycle and team task accepted-response flows.

### Claude's Discretion
The agent may choose the exact decomposition across payment and team slices, persistence adapters, and compatibility fixtures as long as the existing UI and API contract remain stable and no Phase 5 cutover work is pulled forward.

### Deferred Ideas (OUT OF SCOPE)
- Final production cutover and rollback choreography remain Phase 5.
- Any schema cleanup or contract simplification after parity remains Phase 5+.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PAY-01 | Operators can run payment, bind-card, and subscription-sync workflows through Go-owned APIs with current task and session semantics. | Payment section below isolates the Python state machine, account/session/bootstrap delta, current JS consumers, reusable Go account/uploader/router foundations, and the missing automation/persistence slices that planning must cover first. [VERIFIED: codebase grep] |
| TEAM-01 | Operators can run team discovery, sync, invite, membership, and team-task workflows through Go-owned APIs with current behavior. | Team section below isolates the Python accepted-task/task_manager contract, current Team ORM and upstream client semantics, the partial invite/runtime gaps, reusable Go jobs/ws/router foundations, and the parity verification strategy. [VERIFIED: codebase grep] |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- No `CLAUDE.md` exists at the repository root during this research pass. [VERIFIED: codebase grep]
- `workflow.nyquist_validation` is explicitly `false` in `.planning/config.json`, so Phase 4 planning must carry its own verification tasks instead of depending on a Nyquist-generated validation architecture section. [VERIFIED: codebase grep]

## Summary

Phase 4 should be planned as three tightly-scoped slices: payment/bind-card orchestration, Team domain/runtime migration, and parity verification without final cutover. [VERIFIED: codebase grep] Payment and Team are the last major Python-owned backend domains in the roadmap, and both are still intentionally left unmounted in the current Go router and Phase 3 e2e boundary tests. [VERIFIED: codebase grep]

The two domains do not share the same runtime model. [VERIFIED: codebase grep] Payment today is mostly synchronous HTTP orchestration around `accounts` plus a DB-backed `bind_card_tasks` state machine, with no websocket channel and no queue-backed worker contract. [VERIFIED: codebase grep] Team today mixes synchronous CRUD-like actions with accepted async tasks that project `task_uuid` and `ws_channel` through `task_manager`, plus persistent `team_*` tables that remain fully Python-owned. [VERIFIED: codebase grep]

**Primary recommendation:** plan Payment and Team as separate migration tracks that both reuse existing Go `accounts`, `jobs`, `registration/ws`, `uploader`, `router`, and `cmd/api` patterns, but do not force them into one shared runtime abstraction too early. [VERIFIED: codebase grep]

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `backend-go/internal/accounts` | `repo-local` | Shared account/session/subscription read-write surface for both payment and team side effects. | The Go accounts slice already persists `session_token`, `cookies`, `subscription_type`, `subscription_at`, and account detail compatibility fields, and it already exposes uploader store adapters used in Phase 3. [VERIFIED: codebase grep] |
| `backend-go/internal/jobs` | `repo-local` | Durable job, status, and log primitives for Go-owned async work. | Phase 2 already established jobs as the durable task truth source for Go runtime flows, and `jobs.Service` accepts arbitrary `job_type` / `scope_type` / `scope_id`, which is directly relevant to Team task planning. [VERIFIED: codebase grep] |
| `backend-go/internal/registration/ws` | `repo-local` | Existing `/api/ws/task/{task_uuid}` and `/api/ws/batch/{batch_id}` websocket projection layer. | The Team accepted payload already points to `/api/ws/task/{task_uuid}` through Python `task_manager`, and the Go task socket is job-backed and generic enough to project task status/log frames without inventing a second websocket protocol. [VERIFIED: codebase grep] |
| `github.com/hibiken/asynq` | `v0.26.0` | Queue runtime under the Go jobs service. | This is already the Phase 2 worker baseline and should be reused if Team async work moves onto Go jobs instead of a new in-memory runtime. [VERIFIED: codebase grep] |
| `github.com/go-chi/chi/v5` | `v5.2.3` | Additive API route mounting for new slices. | Payment and Team are the only major `/api/*` domains still intentionally left unmounted, so Phase 4 should extend the same `RegisterRoutes` and `NewRouter` pattern used by Phases 2-3. [VERIFIED: codebase grep] |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `backend-go/internal/uploader` | `repo-local` | Existing uploader config repository, sender, and account-store wiring. | Reuse for any payment or team side effects that touch Sub2API/TM/CPA-style service configuration or account writeback patterns; do not duplicate sender/client logic. [VERIFIED: codebase grep] |
| `github.com/jackc/pgx/v5` | `v5.9.1` | PostgreSQL repository access. | Use for new `bind_card_tasks` / `team_*` repositories and migrations because all current Go-owned production data slices are Postgres-backed. [VERIFIED: codebase grep] |
| `github.com/redis/go-redis/v9` | `v9.18.0` | Redis connectivity for queue-backed runtime work. | Needed only for queue-backed Team async execution or any payment work intentionally moved onto jobs/worker semantics. [VERIFIED: codebase grep] |
| Python payment helpers in `src/web/routes/payment.py` | `repo-local` | Current compatibility oracle for session bootstrap, browser bind-card, and subscription-sync semantics. | Keep as the oracle, and if needed use bounded transition adapters behind Go-owned APIs for high-risk browser automation before Phase 5 cutover. [VERIFIED: codebase grep] |
| Python Team upstream contract in `src/services/team/*.py` | `repo-local` | Current compatibility oracle for upstream Team API parsing, sync rules, invite rules, and membership action semantics. | Keep as the oracle while designing the Go Team slice and compatibility tests. [VERIFIED: codebase grep] |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Reusing `backend-go/internal/jobs` + existing task websocket projection for Team async work | A brand-new Team-only in-memory runtime | This would duplicate Phase 2 runtime semantics and create a second task/log control plane to keep compatible. [VERIFIED: codebase grep] |
| Reusing `backend-go/internal/accounts` for session/cookie/subscription writes | A new payment-specific account/session persistence layer | This would fork the `accounts` contract that Phase 1 explicitly froze and that Phase 3 already migrated to Go. [VERIFIED: codebase grep] |
| Reusing `backend-go/internal/uploader` sender/config patterns | New payment/team side-effect clients | This would re-implement already migrated service-config/admin patterns for no parity benefit. [VERIFIED: codebase grep] |

**Installation / bootstrap:**

```bash
uv sync
cd backend-go && go mod download
```

**Version verification:** Third-party Go versions above are pinned in `backend-go/go.mod`; Python compatibility dependencies relevant to this phase (`curl-cffi`, `fastapi`, `sqlalchemy`) are declared in `pyproject.toml` / `requirements.txt`. [VERIFIED: codebase grep]

## Architecture Patterns

### Recommended Project Structure

```text
backend-go/
├── internal/payment/         # New payment domain slice: handlers, service, repository, transition adapters
├── internal/team/            # New team domain slice: handlers, service, repository, task orchestration
├── internal/accounts/        # Reused account/session/subscription truth source
├── internal/jobs/            # Reused durable job control plane
├── internal/registration/ws/ # Reused task/batch websocket projection
├── internal/http/            # Router wiring for additive mounts
└── db/migrations/            # New Postgres tables for bind_card_tasks and team_*
```

Use this shape because the current Go backend already groups migrated domains as `internal/<slice> + internal/<slice>/http + router mount`, and Phase 4 should extend that pattern rather than create a cross-cutting monolith. [VERIFIED: codebase grep]

### Compatibility Consumers

- `static/js/payment.js` is the primary payment page consumer and expects the existing `/api/payment/*` route family, including `generate-link`, `open-incognito`, `session-diagnostic`, `session-bootstrap`, bind-card task CRUD, `mark-user-action`, and `sync-subscription`. [VERIFIED: codebase grep]
- `static/js/accounts.js` also depends on payment APIs for `/payment/accounts/{id}/session-bootstrap`, `/payment/accounts/{id}/mark-subscription`, and `/payment/accounts/batch-check-subscription`, so payment parity is not isolated to `payment.html`. [VERIFIED: codebase grep]
- `static/js/auto_team.js` currently consumes `/api/team/discovery/run`, `/api/team/teams`, `/api/team/teams/{id}`, `/api/team/tasks?team_id=...`, and the accepted payload field `ws_channel` that resolves to `/api/ws/task/{task_uuid}`. [VERIFIED: codebase grep]
- `templates/auto_team.html` already contains invite modal and memberships shell markup, but the current JS file mainly wires list/detail/discovery/sync/task-center flows and does not yet issue invite or membership-action HTTP calls. [VERIFIED: codebase grep]

### Pattern 1: Payment Is A DB-Backed State Machine, Not A Queue Runtime

**What:** Keep payment planning centered on `bind_card_tasks` persistence and the existing status transitions instead of trying to force payment into the Phase 2 jobs/batch/websocket model. [VERIFIED: codebase grep]

**When to use:** For `generate-link`, bind-card task creation/opening, third-party/local auto-bind execution, user-confirmed verification, and subscription sync. [VERIFIED: codebase grep]

**Key facts to preserve:**

- `BindCardTask` rows carry `plan_type`, `workspace_name`, `price_interval`, `seat_quantity`, `country`, `currency`, `checkout_url`, `checkout_session_id`, `publishable_key`, `client_secret`, `checkout_source`, `bind_mode`, `status`, `last_error`, and audit timestamps. [VERIFIED: codebase grep]
- The effective status vocabulary is `link_ready`, `opened`, `waiting_user_action`, `verifying`, `paid_pending_sync`, `completed`, and `failed`. [VERIFIED: codebase grep]
- Payment sync only clears an account back to free on high-confidence `free`; low-confidence `free` preserves existing paid state and usually keeps the task in `paid_pending_sync` or `waiting_user_action`. [VERIFIED: codebase grep]
- Local auto-bind and session bootstrap depend on session token, cookies, `oai-did` / device ID, access token refresh behavior, and possibly a browser automation step. [VERIFIED: codebase grep]

**Example: current payment task serialization**

```python
# Source: src/web/routes/payment.py
def _serialize_bind_card_task(task: BindCardTask) -> dict:
    return {
        "id": task.id,
        "account_id": task.account_id,
        "account_email": task.account.email if task.account else None,
        "plan_type": task.plan_type,
        "checkout_url": task.checkout_url,
        "checkout_source": task.checkout_source,
        "bind_mode": task.bind_mode or "semi_auto",
        "status": task.status,
        "last_error": task.last_error,
    }
```

### Pattern 2: Team Splits Into Read Models, Synchronous Membership Actions, And Accepted Async Tasks

**What:** Treat Team migration as three sub-models, not one package-sized blob. [VERIFIED: codebase grep]

**When to use:** 
- Use repositories/read handlers for `teams`, `team_memberships`, task list/detail, and aggregates. [VERIFIED: codebase grep]
- Use synchronous action handlers for `revoke`, `remove`, and `bind-local-account`. [VERIFIED: codebase grep]
- Use async accepted-task semantics for discovery, sync, and any eventual invite task execution. [VERIFIED: codebase grep]

**Key facts to preserve:**

- `teams`, `team_memberships`, `team_tasks`, and `team_task_items` are live compatibility tables and currently have no Go schema coverage at all. [VERIFIED: codebase grep]
- Python Team accepted responses include `success`, `task_uuid`, `task_type`, `status`, `ws_channel`, and scope fields derived from `team_id` or `owner_account_id`. [VERIFIED: codebase grep]
- `team_tasks.active_scope_key` enforces one active write task per `team` or `owner` scope. [VERIFIED: codebase grep]
- Discovery/sync tasks are scheduled today, but invite task types are only enqueued and guard-logged; they are not actually scheduled, and `run_team_task()` does not implement `invite_accounts` or `invite_emails`. [VERIFIED: codebase grep]

**Example: accepted-response payload shape**

```python
# Source: src/web/task_manager.py
payload = {
    "success": True,
    "task_uuid": task_uuid,
    "task_type": task_type,
    "status": status,
    "ws_channel": f"/api/ws/task/{task_uuid}",
}
```

### Pattern 3: Reuse Go Slice Wiring Instead Of Reopening Earlier Phase Boundaries

**What:** Build Payment and Team as additive Go slices mounted through `cmd/api` and `internal/http/router.go`, following the same package + handler + bootstrap pattern used in Phase 3. [VERIFIED: codebase grep]

**When to use:** For any new Payment/Team handler that should become Go-owned without rewriting the frontend. [VERIFIED: codebase grep]

**Example: existing route wiring pattern**

```go
// Source: backend-go/internal/http/router.go
if accountsService != nil {
    accountshttp.NewHandler(accountsService).RegisterRoutes(r)
}
if registrationService != nil && jobService != nil {
    registrationhttp.NewHandler(...).RegisterRoutes(r)
    r.Get("/api/ws/task/{task_uuid}", taskSocketHandler.HandleTaskSocket)
}
```

### Existing Go Foundations To Extend

- Extend `backend-go/internal/accounts` for account/session/bootstrap/subscription writeback instead of creating a second source of truth for account cookies, session tokens, device IDs, or `subscription_type`. [VERIFIED: codebase grep]
- Extend `backend-go/internal/jobs` and `backend-go/internal/registration/ws` for Team task status/log projection if Team async work moves to Go jobs. [VERIFIED: codebase grep]
- Extend `backend-go/internal/uploader` and the already-wired `UploadAccountStore` pattern for any side effects that need account-scoped writeback or TM/Sub2API transport reuse. [VERIFIED: codebase grep]
- Extend `backend-go/internal/http/router.go` and `backend-go/cmd/api/main.go` for bootstrap/mount ownership instead of bypassing the existing entrypoints. [VERIFIED: codebase grep]

### Missing Slices That Still Need To Be Created

- A new Go payment domain slice does not exist yet. [VERIFIED: codebase grep]
- A new Go team domain slice does not exist yet. [VERIFIED: codebase grep]
- Postgres migrations for `bind_card_tasks`, `teams`, `team_memberships`, `team_tasks`, and `team_task_items` do not exist in `backend-go/db/migrations`. [VERIFIED: codebase grep]
- The current Go router still returns 404 for `/api/payment/...` and `/api/team/...` in boundary tests. [VERIFIED: codebase grep]
- There is no Go equivalent yet for Python payment browser automation (`auto_bind_checkout_with_playwright`) or the Python Team upstream client/parser contract. [VERIFIED: codebase grep]

### Anti-Patterns to Avoid

- **Do not force Payment onto the jobs/websocket stack first:** Payment parity currently depends on DB task rows and HTTP polling/list refresh, not queue semantics. [VERIFIED: codebase grep]
- **Do not assume Team invite support is already complete because endpoints exist:** the invite task types are only half-wired today. [VERIFIED: codebase grep]
- **Do not clear paid subscription state on every `free` readback:** Python only clears it on high-confidence `free`. [VERIFIED: codebase grep]
- **Do not mount Phase 4 routes incidentally while touching unrelated packages:** current tests intentionally keep them 404 until this phase. [VERIFIED: codebase grep]

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Team async control plane | A second websocket/task protocol just for Team | Reuse Go `jobs.Service` + `registration/ws` projection or preserve the existing accepted payload contract exactly | The repo already has durable status/log storage and task websocket projection; duplicating it creates contract drift. [VERIFIED: codebase grep] |
| Payment account/session parsing | Another session-token/device-id/cookie parser | Reuse and extend existing account/session helpers already present in Python payment code and Go accounts service | These fields already exist in the shared `accounts` contract and are easy to get subtly wrong. [VERIFIED: codebase grep] |
| Service-config transport code | New TM/Sub2API/CPA sender implementations | Reuse `backend-go/internal/uploader` repositories, senders, and account-store wiring | Phase 3 already migrated this part and it is not a Phase 4 problem anymore. [VERIFIED: codebase grep] |
| Team table merge logic | Ad-hoc membership counting/status collapse in handlers | Preserve current Team sync utilities for email normalization, status precedence, and seat counting | The Python sync layer already encodes status precedence (`joined` > `already_member` > `invited` > removed/revoked/failed) and seat aggregation behavior. [VERIFIED: codebase grep] |

**Key insight:** The dangerous work in Phase 4 is compatibility projection, not raw CRUD scaffolding. [VERIFIED: codebase grep]

## Common Pitfalls

### Pitfall 1: Planning Payment As If It Were Just “Another Job Type”

**What goes wrong:** The plan introduces queue workers or websocket streams before preserving the existing bind-card task table semantics. [VERIFIED: codebase grep]

**Why it happens:** Phase 2 established a strong jobs/worker pattern, but payment never adopted that runtime model in Python. [VERIFIED: codebase grep]

**How to avoid:** First lock the `BindCardTask` row contract, status transitions, polling/list behavior, and subscription writeback rules; only then decide whether any sub-step actually benefits from a background worker. [VERIFIED: codebase grep]

**Warning signs:** Plans that mention `/api/ws/payment/*`, a new in-memory payment runtime, or skipping the `bind_card_tasks` migration. [VERIFIED: codebase grep]

### Pitfall 2: Missing The Team Invite Execution Gap

**What goes wrong:** The planner assumes invite flows are already equivalent to discovery/sync because the routes exist. [VERIFIED: codebase grep]

**Why it happens:** `team.py` returns accepted payloads for invite routes, but the runner does not implement invite task types and the frontend currently does not call them. [VERIFIED: codebase grep]

**How to avoid:** Treat invite tasks as an explicit missing slice with its own plan items: persistence, executor behavior, UI/consumer parity, and verification. [VERIFIED: codebase grep]

**Warning signs:** Any plan that says “Team async tasks already cover invite” without referencing `run_team_task()` limitations. [VERIFIED: codebase grep]

### Pitfall 3: Treating Auto-Team Frontend Coverage As Complete

**What goes wrong:** Planning ignores invite/membership actions because the current `auto_team.js` shell mostly renders overview/detail/task-center flows. [VERIFIED: codebase grep]

**Why it happens:** The template already contains invite modal markup, which makes the domain look more complete than the current JS wiring actually is. [VERIFIED: codebase grep]

**How to avoid:** Separate “latent backend contract that must remain compatible” from “actively exercised UI path” and test both. [VERIFIED: codebase grep]

**Warning signs:** Only testing `/api/team/teams` and `/api/team/tasks` while skipping invite and membership endpoints. [VERIFIED: codebase grep]

### Pitfall 4: Clearing Paid State Too Aggressively During Subscription Sync

**What goes wrong:** A transient or low-confidence `free` result wipes out real `plus` or `team` state. [VERIFIED: codebase grep]

**Why it happens:** Payment sync includes retry, proxy fallback, and confidence-based write rules that are easy to simplify incorrectly. [VERIFIED: codebase grep]

**How to avoid:** Keep the current “only clear on high-confidence free” rule as an explicit requirement in tests and repository write logic. [VERIFIED: codebase grep]

**Warning signs:** Any implementation that always overwrites `subscription_type` from the latest remote check result. [VERIFIED: codebase grep]

## Verification Strategy

- Keep Python behavior as the parity oracle through Phase 4 and treat Go parity as proven only when route, payload, status, and side-effect compatibility evidence exists. [VERIFIED: codebase grep]
- Update boundary tests intentionally: current router/e2e tests assert payment/team stay 404, and Phase 4 should replace those with mounted-slice tests only when the new handlers are wired. [VERIFIED: codebase grep]
- For payment, add unit/integration coverage around `bind_card_tasks` state transitions, session bootstrap outcomes, third-party/local auto-bind response mapping, and subscription confidence writeback rules before any staging run. [VERIFIED: codebase grep]
- For Team, add coverage around `teams` / `team_memberships` / `team_tasks` repository semantics, accepted payload shape, active-scope conflict behavior, invite execution, membership actions, and websocket/task projection compatibility. [VERIFIED: codebase grep]
- Reuse fake repositories and e2e-style compatibility tests, following the Phase 3 pattern, before attempting real OpenAI/ChatGPT Team API or browser-automation validation. [VERIFIED: codebase grep]
- End Phase 4 as “parity verified, human validation still needed”, not as “Python can be cut over now”; final cutover remains explicitly deferred to Phase 5. [VERIFIED: codebase grep]

## Code Examples

Verified patterns from the current codebase:

### Current Team Accepted Payload Pattern

```python
# Source: src/services/team/tasks.py + src/web/task_manager.py
task_manager.update_status(task_uuid, "pending")
return task_manager.build_accepted_response_payload(
    task_uuid,
    task_type=task_type,
    status="pending",
    scope_type=scope_type,
    scope_id=scope_id,
    team_id=team_id,
    owner_account_id=owner_account_id,
)
```

This is the contract planner must preserve for Team async endpoints that return `202 accepted`-style behavior. [VERIFIED: codebase grep]

### Current Go Route-Mount Pattern

```go
// Source: backend-go/cmd/api/main.go
accountsRepository := accounts.NewPostgresRepository(deps.Postgres)
accountsService := accounts.NewService(accountsRepository)
uploaderService := newAPIUploaderService(
	uploader.NewPostgresConfigRepository(deps.Postgres),
	accountsRepository,
)
handler := internalhttp.NewRouter(jobService, accountsService, uploaderService, ...)
```

Phase 4 should follow this bootstrap pattern instead of side-loading handlers outside `cmd/api` and `internal/http/router.go`. [VERIFIED: codebase grep]

### Current Payment Sync Writeback Rule

```python
# Source: src/web/routes/payment.py
if status in ("plus", "team"):
    account.subscription_type = status
    account.subscription_at = now
elif status == "free":
    if str(detail.get("confidence") or "").lower() == "high":
        account.subscription_type = None
        account.subscription_at = None
```

This confidence gate is a planning-critical invariant for payment migration. [VERIFIED: codebase grep]

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Python-owned backend monolith for payment/team routes | Go slice-per-domain migration with additive router mounts and compatibility tests | Established by completed Phase 2 and Phase 3 work documented on 2026-04-05 | Phase 4 should extend existing Go package boundaries rather than enlarge the Python route monolith. [VERIFIED: codebase grep] |
| Phase 3 router boundary kept payment/team unmounted | Phase 4 is the first roadmap phase allowed to mount `/api/payment*` and `/api/team*` | `backend-go/internal/http/router_test.go` and `backend-go/tests/e2e/management_flow_test.go` on 2026-04-05 | Route-boundary assertions must be deliberately rewritten as part of Phase 4, not ignored as collateral changes. [VERIFIED: codebase grep] |
| Team async runtime stored in Python `team_tasks` + `task_manager` only | Recommended next step is to project the same accepted/task/ws contract through Go-owned repositories and, where useful, the existing Go jobs/ws infrastructure | This is the migration delta surfaced by the current repo shape | Planning must decide the compatibility projection explicitly; it is not already solved by existing Go code. [VERIFIED: codebase grep] |

**Deprecated/outdated:**

- Treating Payment and Team as “future work with no active compatibility burden” is outdated as of the current roadmap; they are the only remaining Python-only backend domains in v1 scope. [VERIFIED: codebase grep]

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Phase 4 can keep a bounded Python or sidecar automation adapter behind Go-owned payment APIs for browser-heavy local auto-bind/session bootstrap work, as long as the public API ownership and operator contract move to Go first. | Architecture Patterns / Verification Strategy | If this is not acceptable, Phase 4 scope expands materially because the browser automation port must be completed before parity can be claimed. |
| A2 | Team async endpoints should eventually project onto the existing Go `/api/ws/task/{task_uuid}` channel instead of preserving a separate Python-only websocket implementation. | Standard Stack / Architecture Patterns | If this proves incompatible, the planner will need a dedicated Team websocket adapter and extra parity work. |

## Open Questions

1. **Should local auto-bind/session-bootstrap browser automation be fully ported to Go in Phase 4, or isolated behind a bounded transition adapter?**
   What we know: the current implementation is Python-only and depends on Playwright/browser/session bootstrap helpers. [VERIFIED: codebase grep]
   What's unclear: whether Phase 4 considers a bounded adapter acceptable or requires a native Go replacement immediately. [ASSUMED]
   Recommendation: decide this in 04-01 before decomposing implementation tasks, because it changes scope and verification cost materially.

2. **Should Team async persistence reuse Go `jobs` directly, dual-write into `team_tasks`, or keep `team_tasks` as the compatibility read model?**
   What we know: Go jobs are already durable and websocket-friendly, while Python Team compatibility currently depends on `team_tasks`, `team_task_items`, and accepted payload semantics. [VERIFIED: codebase grep]
   What's unclear: which storage model gives the lowest migration risk while preserving current operator behavior. [ASSUMED]
   Recommendation: answer this at plan time and keep the choice explicit; do not let storage shape drift implicitly from handler convenience.

3. **Are invite and membership UI flows expected to become fully interactive in Phase 4, or is backend/API parity enough for latent routes?**
   What we know: the template exposes invite modal markup, but the current JS wiring mainly exercises discovery/sync/list/detail/task-center flows. [VERIFIED: codebase grep]
   What's unclear: whether Phase 4 must finish the currently latent UI event wiring or only preserve backend compatibility for existing/manual consumers. [ASSUMED]
   Recommendation: confirm expected operator surface before assigning effort to frontend smoke coverage.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | Build/test new Go payment/team slices | ✓ (wrong version) | `go1.24.3` locally vs `go 1.25.0` in `backend-go/go.mod` | Upgrade toolchain before any phase gate that requires full Go test confidence |
| Python runtime | Python oracle reads, transition adapters, and existing payment/team behavior verification | ✓ | `Python 3.11.9` | — |
| `uv` | Python env bootstrap | ✓ | `0.9.13` | `pip`-based install path remains possible |
| `npm` | Existing frontend/static tooling sanity only | ✓ | `10.8.2` | none needed for backend-only contract tests |
| Python `playwright` module | Real local-auto bind-card/browser validation | ✗ | — | Keep contract/unit tests only and defer real browser validation to staging/manual checks |
| PostgreSQL CLI/tools | Real migration smoke and manual data inspection | ✗ | — | Use fake repos/unit/e2e tests until a real Postgres environment is provided |
| Redis CLI/tools | Real queue/worker smoke for any Team-on-jobs runtime | ✗ | — | Use in-memory/fake repo tests until a real Redis environment is provided |
| Docker | Container/browser parity and bundled runtime checks | ✗ | — | Skip local container parity; rely on code-level verification plus later staging |
| Python `curl_cffi` | Existing payment/team oracle behavior | ✓ | importable | — |

**Missing dependencies with no fallback:**

- None for planning-only research. [VERIFIED: command probe]

**Missing dependencies with fallback:**

- `playwright`, PostgreSQL tooling, Redis tooling, and Docker are missing locally, so Phase 4 should plan contract/unit/e2e parity work first and keep real environment validation as explicit human-needed tasks. [VERIFIED: command probe]

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes | Preserve owner account token/session handling through existing `accounts` contract and Team owner `access_token` checks before upstream actions. [VERIFIED: codebase grep] |
| V3 Session Management | yes | Preserve current payment session bootstrap rules around `session_token`, cookie merge, `oai-did`, and low-confidence subscription handling. [VERIFIED: codebase grep] |
| V4 Access Control | yes | Preserve team-owner scoping (`owner_account_id`, `membership_id`, `upstream_user_id`) and do not allow membership mutations without owner/token checks. [VERIFIED: codebase grep] |
| V5 Input Validation | yes | Keep Pydantic request validation on the compatibility side and mirror it in Go request structs/handler validation. [VERIFIED: codebase grep] |
| V6 Cryptography | yes | Treat access tokens, refresh tokens, session tokens, API keys, and card data as opaque secrets; keep using existing libraries and redaction helpers instead of hand-rolled crypto or logging shortcuts. [VERIFIED: codebase grep] |

### Known Threat Patterns for This Stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Token / cookie / API key leakage in logs or task payloads | Information Disclosure | Preserve secret masking (`_mask_secret`) and card masking (`_mask_card_number`) semantics; never emit raw `session_token`, card PAN/CVC, access token, or third-party API key in logs or responses. [VERIFIED: codebase grep] |
| User-supplied proxy / third-party API URL abuse | Tampering / SSRF | Keep this surface operator-only, validate/normalize URLs strictly, and do not broaden server-side fetch behavior beyond the current compatibility contract. [VERIFIED: codebase grep] |
| Incorrect Team membership mutation against the wrong owner or stale upstream user | Elevation of Privilege | Require `owner_account_id`, owner access token, `team_id`, and `upstream_user_id` checks exactly as current membership actions do. [VERIFIED: codebase grep] |
| Active-task collision on the same Team or owner scope | Denial of Service / Integrity | Preserve `active_scope_key` uniqueness or its equivalent so concurrent Team write tasks cannot race silently. [VERIFIED: codebase grep] |
| Subscription downgrade on uncertain sync result | Tampering | Preserve the current “only clear on high-confidence free” rule in both code and tests. [VERIFIED: codebase grep] |

## Sources

### Primary (HIGH confidence)

- `.planning/phases/04-payment-and-team-domains/04-CONTEXT.md` - locked decisions, scope, canonical refs, and reuse boundary. [VERIFIED: codebase grep]
- `.planning/REQUIREMENTS.md`, `.planning/STATE.md`, `.planning/ROADMAP.md` - phase requirements, prior-phase handoff, and cutover boundary. [VERIFIED: codebase grep]
- `.planning/phases/01-compatibility-baseline/01-route-client-parity-matrix.md` - current frontend/API compatibility consumers and ownership matrix. [VERIFIED: codebase grep]
- `.planning/phases/01-compatibility-baseline/01-shared-schema-contract.md` - shared table coverage gaps and Phase 4 storage obligations. [VERIFIED: codebase grep]
- `.planning/phases/02-native-registration-runtime/02-VERIFICATION.md` and `.planning/phases/03-management-apis/03-VERIFICATION.md` - reusable Go runtime/management foundations and current router boundary assumptions. [VERIFIED: codebase grep]
- `src/web/routes/payment.py`, `src/web/routes/team.py`, `src/web/routes/team_tasks.py`, `src/web/task_manager.py`, `src/database/models.py`, `src/database/team_models.py`, `src/database/team_crud.py`, `src/services/team/*` - current Python oracle for payment and team behavior. [VERIFIED: codebase grep]
- `static/js/payment.js`, `static/js/auto_team.js`, `static/js/accounts.js`, `templates/payment.html`, `templates/auto_team.html` - current frontend consumers and operator paths. [VERIFIED: codebase grep]
- `backend-go/internal/accounts/*`, `backend-go/internal/jobs/*`, `backend-go/internal/registration/*`, `backend-go/internal/registration/ws/*`, `backend-go/internal/uploader/*`, `backend-go/internal/http/*`, `backend-go/cmd/api/main.go`, `backend-go/go.mod`, `backend-go/db/migrations/*` - current Go foundations, package boundaries, and missing payment/team storage coverage. [VERIFIED: codebase grep]
- Local environment command probes (`go version`, `python3 --version`, `uv --version`, `npm --version`, Python import checks) - environment availability findings. [VERIFIED: command probe]

### Secondary (MEDIUM confidence)

- None.

### Tertiary (LOW confidence)

- None.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - almost all stack conclusions come from the current repo layout, `go.mod`, and completed Phase 2/3 verification artifacts. [VERIFIED: codebase grep]
- Architecture: MEDIUM - the current runtime deltas are well evidenced, but the exact best compatibility projection for Team async persistence and payment browser automation still depends on Phase 4 planning choices. [VERIFIED: codebase grep]
- Pitfalls: HIGH - the major regression risks are directly visible in current Python route logic, Team runner gaps, frontend consumers, and router boundary tests. [VERIFIED: codebase grep]

**Research date:** 2026-04-05
**Valid until:** 2026-05-05
