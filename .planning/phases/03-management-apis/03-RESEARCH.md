# Phase 3: Management APIs - Research

**Researched:** 2026-04-05
**Domain:** Go migration of management/admin APIs for accounts, settings, email services, upload-service configs, proxies, and logs
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Current Python management APIs remain the compatibility oracle until their Go replacements prove parity.
- **D-02:** Existing templates and `static/js` pages must keep working without a frontend rewrite; backend contract preservation comes first.
- **D-03:** Phase 3 covers accounts management, settings, email services, upload-service configs, proxies, and logs.
- **D-04:** Payment/bind-card flows remain Phase 4 scope even when currently called from accounts or management pages.
- **D-05:** Team discovery/sync/invite/task flows remain Phase 4 scope even when current pages link to them.
- **D-06:** Extend existing Go `accounts`, registration-adjacent, and config/uploader persistence foundations instead of reintroducing Python-side orchestration into normal management paths.
- **D-07:** Preserve current field names, filtering semantics, export/import behavior, and operator-visible status strings unless a later phase explicitly changes them.

### Claude's Discretion
The agent may choose the exact decomposition across Go handlers/services/repositories and UI contract fixtures as long as the existing management pages and scripts remain backend-compatible and scope does not bleed into payment or team domains.

### Deferred Ideas (OUT OF SCOPE)
- Payment/bind-card API migration remains Phase 4 scope.
- Team domain API migration remains Phase 4 scope.
- Final production cutover and external-environment verification remain Phase 5 scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| MGMT-01 | Operators can manage accounts through Go-owned APIs with current CRUD, import/export, refresh, validate, and upload workflows. [VERIFIED: .planning/REQUIREMENTS.md] | Extend `backend-go/internal/accounts` beyond the current list-only HTTP route, preserve the query/body shapes used by `static/js/accounts.js`, `static/js/accounts_overview.js`, and `static/js/accounts_state_actions.js`, and reuse existing uploader senders for CPA/Sub2API/TM writebacks. [VERIFIED: backend-go/internal/accounts/http/handlers.go; static/js/accounts.js; static/js/accounts_overview.js; static/js/accounts_state_actions.js; backend-go/internal/uploader/sender.go] |
| MGMT-02 | Operators can manage settings, email services, upload-service configs, proxies, and logs through Go-owned APIs with current behavior. [VERIFIED: .planning/REQUIREMENTS.md] | New Go slices are required for settings, email-services, proxies, and logs; upload-config CRUD should extend the existing uploader config persistence rather than replace it. [VERIFIED: backend-go/internal/http/router.go; backend-go/internal/uploader/repository_postgres.go; src/web/routes/settings.py; src/web/routes/email.py; src/web/routes/logs.py] |
| CUT-01 | Current templates and static JavaScript can target the Go backend for migrated domains without requiring a UI rewrite. [VERIFIED: .planning/REQUIREMENTS.md] | Preserve current `/api/*` paths and mixed page ownership: the management pages already hard-code management routes plus a small number of Phase 4 payment/team calls that must remain untouched for now. [VERIFIED: .planning/phases/01-compatibility-baseline/01-route-client-parity-matrix.md; static/js/accounts.js] |
</phase_requirements>

## Summary

Phase 3 is a contract-preservation phase, not a redesign phase: the existing Jinja pages and page-scoped JavaScript already hard-code management endpoints across accounts, settings, email services, upload-service configs, proxies, and logs. [VERIFIED: .planning/phases/01-compatibility-baseline/01-route-client-parity-matrix.md; static/js/accounts.js; static/js/accounts_overview.js; static/js/settings.js; static/js/email_services.js; static/js/logs.js]

The current Go baseline is materially narrower than the Python management surface. Go already owns registration/jobs infrastructure plus a thin `/api/accounts` list route, account persistence/upsert logic, uploader config readers/senders, and registration-side readers for `settings`, `email_services`, and proxy selection; it does not yet own management HTTP routes for settings, email-services, upload-config CRUD, proxies, or logs. [VERIFIED: backend-go/internal/http/router.go; backend-go/internal/accounts/http/handlers.go; backend-go/internal/accounts/service.go; backend-go/internal/uploader/repository_postgres.go; backend-go/internal/uploader/sender.go; backend-go/internal/registration/available_services_postgres.go; backend-go/internal/registration/outlook_repository_postgres.go; backend-go/internal/registration/proxy_selector.go]

The planning risk is not “missing business logic in general”; it is contract drift at the handler edge. The current Go `/api/accounts` response is not UI-compatible yet because its query layer ignores filters and omits fields the current pages render, while several settings/email/upload/proxy/log routes have no Go owner at all. [VERIFIED: backend-go/internal/accounts/repository_postgres.go; src/web/routes/accounts.py; static/js/accounts.js; static/js/accounts_overview.js; src/web/routes/settings.py; src/web/routes/email.py; src/web/routes/logs.py]

**Primary recommendation:** Plan Phase 3 as three slices: extend `internal/accounts` to full account-management parity, extend `internal/uploader` plus registration-side readers into config-admin APIs, and add new Go slices for `settings`, `email-services`, `proxies`, and `logs`, while leaving payment/team endpoints on their current Python ownership. [VERIFIED: .planning/phases/03-management-apis/03-CONTEXT.md; backend-go/internal/accounts/; backend-go/internal/uploader/; static/js/accounts.js]

## Python-vs-Go Delta

| Domain | Python Today | Go Today | Planning Impact |
|--------|--------------|----------|-----------------|
| Accounts | Python owns full account CRUD, import/export, overview cards, token refresh/validate, upload triggers, current-account switching, inbox-code lookup, and team-relation projections. [VERIFIED: src/web/routes/accounts.py] | Go exposes only `GET /api/accounts`; the service/repo also support `UpsertAccount` for runtime persistence, but no management HTTP routes are wired beyond list. [VERIFIED: backend-go/internal/accounts/http/handlers.go; backend-go/internal/accounts/service.go; backend-go/internal/http/router.go] | Extend the existing accounts slice instead of creating a second account API surface. Preserve current request filters and response adapters before reusing the current Go list handler. |
| Settings | Python owns `/api/settings`, registration/email-code/outlook/dynamic-proxy updates, proxy CRUD, SQLite database info/backup/import/cleanup, and a legacy `/settings/logs` endpoint. [VERIFIED: src/web/routes/settings.py] | Go has no settings/proxy HTTP slice, but registration code already reads `settings` keys and static/dynamic proxy settings from Postgres. [VERIFIED: backend-go/internal/http/router.go; backend-go/internal/registration/available_services_postgres.go; backend-go/internal/registration/proxy_selector.go] | Reuse the existing settings read semantics and db-key naming, but add a dedicated Go admin slice for writes and compatibility envelopes. Treat `/settings/database/*` as a separate sub-problem because the current Python behavior is SQLite-local while Go runtime is Postgres-only. |
| Email Services | Python owns stats, type catalog, CRUD, enable/disable/test, `/full`, and Outlook batch import. List/detail responses expose `last_used`, filtered config booleans, and Outlook registration hints. [VERIFIED: src/web/routes/email.py] | Go only reads enabled email services for registration availability and Outlook account matching; it has no email-service management HTTP/API layer. [VERIFIED: backend-go/internal/registration/available_services_postgres.go; backend-go/internal/registration/outlook_repository_postgres.go; backend-go/internal/http/router.go] | Add a new Go email-service admin slice; preserve the filtered-vs-full response split and Outlook registration projection. |
| Upload Configs | Python owns CRUD/test for CPA/Sub2API/TM service configs, and account pages use those configs for upload actions. [VERIFIED: src/web/routes/upload/cpa_services.py; src/web/routes/upload/sub2api_services.py; src/web/routes/upload/tm_services.py; static/js/accounts.js; static/js/settings.js] | Go already has Postgres readers for CPA/Sub2API/TM config rows plus payload builders and HTTP senders for the three target systems. [VERIFIED: backend-go/internal/uploader/repository_postgres.go; backend-go/internal/uploader/builder.go; backend-go/internal/uploader/sender.go] | Extend `internal/uploader` into admin CRUD/services/handlers; do not reimplement target-specific payload formats in a new package. |
| Proxies | Python owns proxy CRUD, default selection, enable/disable/test-all, and dynamic proxy config testing. [VERIFIED: src/web/routes/settings.py] | Go registration already knows how to read static/dynamic proxy settings and will opportunistically read a legacy proxy pool if a compatible Postgres table/column exists; there is no proxy management API. [VERIFIED: backend-go/internal/registration/proxy_selector.go] | Add a dedicated proxy admin slice and align it with existing Go proxy-selection semantics. Preserve Python’s `is_default` and `last_used` behavior. |
| Logs | Python owns `app_logs` browsing, stats, cleanup, and destructive clear endpoints. [VERIFIED: src/web/routes/logs.py] | Go owns `job_logs` for jobs/registration tasks, not `app_logs`, and there is no admin log browser endpoint. [VERIFIED: backend-go/internal/jobs/http/handlers.go; backend-go/internal/jobs/repository_runtime.go; .planning/phases/01-compatibility-baseline/01-shared-schema-contract.md] | Build a new Go logs slice for `app_logs`; do not try to alias `/api/logs` to `job_logs`. |

## Frontend Consumers

### Current Pages and Route Dependencies

| Page / Consumer | Hard Dependencies | Contract Notes |
|-----------------|-------------------|----------------|
| `templates/accounts.html` + `static/js/accounts.js` | `/accounts/stats/summary`, paginated `/accounts`, `/accounts/{id}`, `/accounts/{id}/tokens`, `/accounts/{id}/refresh`, `/accounts/{id}/inbox-code`, `/accounts/{id}` `PATCH/DELETE`, `/accounts/batch-*`, `/accounts/export/*`, `/accounts/{id}/upload-*`, `/cpa-services?enabled=true`, `/sub2api-services?enabled=true`, `/tm-services?enabled=true`. [VERIFIED: static/js/accounts.js; templates/accounts.html] | The account table renders `email_service`, `status`, `cpa_uploaded`, `sub2api_uploaded`, `subscription_type`, and `last_refresh`; the detail modal expects `client_id`, `account_id`, `workspace_id`, cookies, token bundle fields, `device_id`, and `session_token_source`. [VERIFIED: static/js/accounts.js] |
| `templates/accounts_overview.html` + `static/js/accounts_overview.js` | `/accounts/stats/overview`, `/accounts/overview/cards`, `/accounts/overview/refresh`, `/accounts/overview/cards/remove`, `/accounts/overview/cards/selectable`, `/accounts/overview/cards/{id}/attach`, `/accounts`, `/accounts/import`, `/accounts` `POST`, `/accounts/export/json`. [VERIFIED: static/js/accounts_overview.js; templates/accounts_overview.html] | Card data expects `plan_type`, `current`, `hourly_quota`, `weekly_quota`, `code_review_quota`, `overview_fetched_at`, `overview_stale`, and `overview_error`; legacy summary tiles expect `token_stats`, `by_status`, `by_email_service`, `by_source`, `by_subscription`, and `recent_accounts`. [VERIFIED: static/js/accounts_overview.js; src/web/routes/accounts.py] |
| `templates/settings.html` + `static/js/settings.js` | `/settings`, `/settings/database*`, `/settings/registration`, `/settings/email-code`, `/settings/outlook`, `/settings/proxies*`, `/settings/proxy/dynamic*`, `/email-services`, `/email-services/types`, `/tm-services*`, `/cpa-services*`, `/sub2api-services*`. [VERIFIED: static/js/settings.js; templates/settings.html] | The page expects mixed response envelopes: `/settings` returns a nested object, `/settings/proxies` returns `{proxies,total}`, `/email-services` returns `{services,total}`, while `/tm-services`, `/cpa-services`, and `/sub2api-services` return plain arrays. [VERIFIED: static/js/settings.js; src/web/routes/settings.py; src/web/routes/email.py; src/web/routes/upload/cpa_services.py; src/web/routes/upload/sub2api_services.py; src/web/routes/upload/tm_services.py] |
| `templates/email_services.html` + `static/js/email_services.js` | `/email-services/stats`, filtered `/email-services?service_type=...`, `/email-services/outlook/batch-import`, `/email-services/{id}/full`, `/settings`, `/settings/tempmail`, `/email-services/test-tempmail`, `/email-services/{id}` `PATCH/DELETE`, `/email-services/{id}/test`. [VERIFIED: static/js/email_services.js; templates/email_services.html] | Outlook rows depend on `config.has_oauth`, `registration_status`, `registered_account_id`, and `last_used`; custom rows depend on service-type-specific config fields preserved under `config`. [VERIFIED: static/js/email_services.js; src/web/routes/email.py] |
| `templates/logs.html` + `static/js/logs.js` | `/logs?page=&page_size=&level=&logger_name=&keyword=&since_minutes=`, `/logs/stats`, `/logs/cleanup`, `DELETE /logs?confirm=true`. [VERIFIED: static/js/logs.js; templates/logs.html] | The page expects paginated `logs[]` items with `created_at`, `level`, `logger`, `message`, `exception`, plus stats fields `total`, `latest_at`, and `levels.{INFO,WARNING,ERROR,CRITICAL}`. [VERIFIED: static/js/logs.js; src/web/routes/logs.py] |

### Cross-Scope Calls That Must Stay Out of Phase 3

- `static/js/accounts.js` still calls `/payment/accounts/{id}/session-bootstrap`, `/payment/accounts/{id}/mark-subscription`, and `/payment/accounts/batch-check-subscription`; these are explicit Phase 4 dependencies and should remain Python-owned during Phase 3. [VERIFIED: static/js/accounts.js; .planning/phases/03-management-apis/03-CONTEXT.md]
- `static/js/accounts.js` also computes `/auto-team?owner_account_id=...` links from `team_relation_summary` and `subscription_type`; Phase 3 must preserve the read-only team relation fields without migrating team APIs. [VERIFIED: static/js/accounts.js; src/web/routes/accounts.py; .planning/phases/03-management-apis/03-CONTEXT.md]

### Public Routes With No Current Page-Scoped Consumer

- `/api/accounts/overview/cards/addable` and `/api/accounts/overview/cards/{account_id}/restore` exist in Python but have no current `templates/` or `static/js/` caller in repository search. [VERIFIED: codebase grep]
- `/api/accounts/current`, `/api/accounts/{account_id}/cookies`, `/api/settings/logs`, `/api/email-services/reorder`, and `/api/sub2api-services/upload` likewise have no current page-scoped caller in repository search. [VERIFIED: codebase grep]

## Existing Go Foundations

### Extend These

| Package / Asset | Current Capability | Why It Should Be Extended |
|-----------------|--------------------|---------------------------|
| `backend-go/internal/accounts` | Repository/service already own `ListAccounts`, `GetAccountByEmail`, `UpsertAccount`, and token-completion CAS logic; worker/registration already depend on it for persistence. [VERIFIED: backend-go/internal/accounts/service.go; backend-go/internal/accounts/repository_postgres.go; backend-go/cmd/worker/main.go] | This is the right home for account CRUD/detail/stats/export/import/upload orchestration because it already owns account persistence and merge semantics. |
| `backend-go/internal/uploader` | Repository reads CPA/Sub2API/TM configs; builders and senders already encode target payloads and headers for all three upload targets. [VERIFIED: backend-go/internal/uploader/repository_postgres.go; backend-go/internal/uploader/builder.go; backend-go/internal/uploader/sender.go] | Reusing it avoids duplicating external upload contract logic and keeps account-upload writebacks aligned with existing runner-side upload behavior. |
| `backend-go/internal/registration/available_services_postgres.go` | Reads `settings` key/value rows and enabled `email_services` rows from Postgres. [VERIFIED: backend-go/internal/registration/available_services_postgres.go] | These readers are the seed for a Go settings/email-service admin slice because they already understand current db-key names and enabled-service ordering. |
| `backend-go/internal/registration/outlook_repository_postgres.go` | Reads enabled Outlook services and matches registered accounts by email. [VERIFIED: backend-go/internal/registration/outlook_repository_postgres.go] | This can be reused to compute `registration_status` / `registered_account_id` in Go email-service list responses without reaching back into Python. |
| `backend-go/internal/registration/proxy_selector.go` | Reads static/dynamic proxy settings and gracefully tolerates a missing legacy proxy pool/column. [VERIFIED: backend-go/internal/registration/proxy_selector.go] | This is the authoritative Go interpretation of current proxy-selection behavior and should anchor proxy-management parity decisions. |

### New Slices Still Required

| New Slice | Why It Does Not Already Exist |
|-----------|-------------------------------|
| Settings admin slice | No Go package currently owns `/api/settings*` write APIs or the Python metadata contract for `settings.description/category/updated_at`. [VERIFIED: backend-go/internal/http/router.go; .planning/phases/01-compatibility-baseline/01-shared-schema-contract.md] |
| Email-services admin slice | Go only has registration-time email-service readers; CRUD/stats/type-catalog/full-vs-filtered response handling are still Python-only. [VERIFIED: backend-go/internal/registration/available_services_postgres.go; src/web/routes/email.py] |
| Proxy admin slice | Go can select proxies for registration, but it cannot list/create/update/test/default proxies for the settings page. [VERIFIED: backend-go/internal/registration/proxy_selector.go; src/web/routes/settings.py] |
| Logs admin slice | Go `job_logs` are job/task logs, not `app_logs`, and there is no `/api/logs` handler. [VERIFIED: backend-go/internal/jobs/http/handlers.go; src/web/routes/logs.py; .planning/phases/01-compatibility-baseline/01-shared-schema-contract.md] |

## Standard Stack

### Core

| Library / Asset | Version | Purpose | Why Standard |
|-----------------|---------|---------|--------------|
| Go toolchain | `1.25.0` repo target; local `1.24.3` installed | Phase 3 Go implementation/runtime baseline | `backend-go/go.mod` pins `go 1.25.0`, so Phase 3 should stay on the existing backend-go toolchain rather than introduce a second runtime. [VERIFIED: backend-go/go.mod; local `go version`] |
| `github.com/go-chi/chi/v5` | `v5.2.3` | HTTP routing | The current Go API router is already built on `chi`; management APIs should be added there, not under a parallel framework. [VERIFIED: backend-go/go.mod; backend-go/internal/http/router.go] |
| `github.com/jackc/pgx/v5` | `v5.9.1` | Postgres repositories | Existing accounts, registration, and uploader code already use pgx-style repositories. [VERIFIED: backend-go/go.mod; backend-go/internal/accounts/repository_postgres.go; backend-go/internal/uploader/repository_postgres.go] |
| Python route files | current repo state | Compatibility oracle | Python remains the source of truth for management API behavior until Go parity is proven. [VERIFIED: .planning/phases/03-management-apis/03-CONTEXT.md] |

### Supporting

| Asset | Purpose | When to Use |
|-------|---------|-------------|
| `backend-go/internal/accounts` | Account persistence, merge rules, and current `/api/accounts` baseline | Use as the base for account-management parity, not just runner persistence. [VERIFIED: backend-go/internal/accounts/service.go] |
| `backend-go/internal/uploader` | Upload config reads plus CPA/Sub2API/TM payload/send logic | Use for upload-config CRUD extension and account upload endpoints. [VERIFIED: backend-go/internal/uploader/repository_postgres.go; backend-go/internal/uploader/sender.go] |
| `src/config/settings.py` definitions | Typed db-key map and secret-field semantics | Use as the compatibility source for Go settings keys and defaults. [VERIFIED: src/config/settings.py] |
| `static/js/accounts_state_actions.js` | Canonical account filter/query/body serialization | Use as the contract source for Go query/body adapters. [VERIFIED: static/js/accounts_state_actions.js] |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Extending the existing `/api/accounts` route family | Introducing `/api/v2/management/*` | This would immediately violate `CUT-01` because the current pages are hard-coded to the existing `/api/*` paths. [VERIFIED: .planning/REQUIREMENTS.md; static/js/accounts.js; static/js/accounts_overview.js] |
| Extending `internal/uploader` for config-admin work | Building separate per-target upload helper packages | That would duplicate the already-correct CPA/Sub2API/TM payload and header logic. [VERIFIED: backend-go/internal/uploader/builder.go; backend-go/internal/uploader/sender.go] |
| Reusing current settings db-key names | Rebinding management APIs directly to env vars | Python settings are database-backed and Go registration already reads those same keys from Postgres. [VERIFIED: src/config/settings.py; backend-go/internal/registration/available_services_postgres.go; backend-go/internal/registration/proxy_selector.go] |

**Installation:** No new third-party packages are recommended for Phase 3 planning; stay on the repo-pinned Go/Python stack and extend existing internal packages. [VERIFIED: backend-go/go.mod; pyproject.toml]

**Version verification:** Phase 3 does not require a new third-party library decision. The relevant implementation stack is already pinned in `backend-go/go.mod` and `pyproject.toml`; the only local mismatch found is that the machine currently has Go `1.24.3` while the repo target is `1.25.0`. [VERIFIED: backend-go/go.mod; pyproject.toml; local `go version`]

## Architecture Patterns

### Recommended Project Structure

```text
backend-go/internal/
├── accounts/              # extend existing account CRUD/detail/stats/export/import/upload orchestration
├── uploader/              # extend config CRUD services/repos around existing builders/senders
├── settings/              # new typed settings reader/writer using current db_key map
├── emailservices/         # new admin CRUD/stats/type-catalog/full-vs-filtered responses
├── proxies/               # new proxy CRUD/test/default admin slice aligned with selector semantics
└── logs/                  # new app_logs compatibility slice
```

### Pattern 1: Contract Adapter at the HTTP Edge
**What:** Parse requests and emit responses in the exact shape the existing JS expects, even if the internal Go service uses cleaner types. [VERIFIED: static/js/accounts_state_actions.js; static/js/logs.js]
**When to use:** Every migrated management route. The frontend already encodes field names, envelopes, and mixed array-vs-object response shapes. [VERIFIED: static/js/accounts.js; static/js/settings.js; static/js/email_services.js]
**Example:**
```javascript
// Source: static/js/accounts_state_actions.js
params.set('page', String(page));
params.set('page_size', String(pageSize));
if (normalized.status) params.set('status', normalized.status);
if (normalized.email_service) params.set('email_service', normalized.email_service);
if (normalized.refresh_token_state) params.set('refresh_token_state', normalized.refresh_token_state);
if (normalized.search) params.set('search', normalized.search);
```

### Pattern 2: Service/Repository Layering Over Existing Persistence
**What:** Keep handlers thin, put compatibility/business rules in services, and keep SQL in repositories. [VERIFIED: backend-go/internal/accounts/http/handlers.go; backend-go/internal/accounts/service.go; backend-go/internal/accounts/repository_postgres.go]
**When to use:** All new Go management slices, especially where response shaping depends on computed fields like subscription/team hints or filtered config output. [VERIFIED: src/web/routes/accounts.py; src/web/routes/email.py]
**Example:**
```go
// Source: backend-go/internal/accounts/service.go
func (s *Service) ListAccounts(ctx context.Context, req ListAccountsRequest) (AccountListResponse, error) {
	normalized := req.Normalized()
	accounts, total, err := s.repository.ListAccounts(ctx, normalized)
	if err != nil {
		return AccountListResponse{}, err
	}
	return AccountListResponse{Page: normalized.Page, PageSize: normalized.PageSize, Total: total, Accounts: accounts}, nil
}
```

### Pattern 3: Reuse Existing Upload Builders/Senders
**What:** Treat upload-config CRUD and account upload actions as two layers: config management over service rows, and outbound transport over the existing sender/builder code. [VERIFIED: backend-go/internal/uploader/repository_postgres.go; backend-go/internal/uploader/builder.go; backend-go/internal/uploader/sender.go]
**When to use:** CPA/Sub2API/TM admin endpoints and account upload routes. [VERIFIED: src/web/routes/accounts.py; src/web/routes/upload/cpa_services.py; src/web/routes/upload/sub2api_services.py; src/web/routes/upload/tm_services.py]
**Example:**
```go
// Source: backend-go/internal/uploader/sender.go
httpReq, err := newJSONRequest(ctx, http.MethodPost, joinURLPath(service.BaseURL, "/api/v1/admin/accounts/data"), payload, map[string]string{
	"x-api-key":       service.Credential,
	"Idempotency-Key": "import-" + payload.Data.ExportedAt,
})
```

### Anti-Patterns to Avoid
- **Treating current Go `/api/accounts` as already page-compatible:** the query only supports pagination and the SQL omits fields the pages render, including `email_service` and `subscription_type`. [VERIFIED: backend-go/internal/accounts/http/handlers.go; backend-go/internal/accounts/repository_postgres.go; static/js/accounts.js]
- **Collapsing `app_logs` onto `job_logs`:** the Python logs page is not a job-task log viewer, and the schema contract explicitly calls out that `job_logs` is not a drop-in replacement. [VERIFIED: .planning/phases/01-compatibility-baseline/01-shared-schema-contract.md; backend-go/internal/jobs/http/handlers.go]
- **Pulling payment or team logic into Phase 3 because the pages call it:** the page may stay mixed-owned; only the Phase 3 management routes should move now. [VERIFIED: .planning/phases/03-management-apis/03-CONTEXT.md; static/js/accounts.js]

## Don’t Hand-Roll

| Problem | Don’t Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| CPA/Sub2API/TM payload formatting | New ad hoc JSON/multipart payload builders in handlers | `backend-go/internal/uploader/builder.go` + `sender.go` | The target-specific payloads, headers, and fallback rules already exist and are tested. [VERIFIED: backend-go/internal/uploader/builder_test.go; backend-go/internal/uploader/sender_test.go] |
| Settings key naming | Fresh Go-only key strings | Current Python `SETTING_DEFINITIONS` / `DB_SETTING_KEYS` map | Registration already depends on these db keys, and changing them would drift from Python semantics. [VERIFIED: src/config/settings.py; backend-go/internal/registration/available_services_postgres.go; backend-go/internal/registration/proxy_selector.go] |
| Account filter/query serialization | New query/body semantics | `static/js/accounts_state_actions.js` as the frontend contract source | The page already serializes `status`, `email_service`, `refresh_token_state`, and `search` in a fixed shape. [VERIFIED: static/js/accounts_state_actions.js] |
| Proxy selection semantics | Another selection algorithm | Existing `registration.PostgresProxySelector` semantics | Registration already falls back from request proxy → legacy pool → dynamic/static settings; admin APIs should not invent a different truth. [VERIFIED: backend-go/internal/registration/proxy_selector.go] |
| Logs replacement | Reusing `/api/jobs/{jobID}/logs` for the admin log page | A dedicated `app_logs` compatibility slice | The UI expects app-wide log browsing, filters, cleanup, and destructive clear against `app_logs`. [VERIFIED: src/web/routes/logs.py; static/js/logs.js] |

**Key insight:** Phase 3 mostly needs adapter work and slice completion, not new algorithms. The biggest risk is contract drift at the HTTP edge, not missing low-level persistence primitives. [VERIFIED: backend-go/internal/accounts/; backend-go/internal/uploader/; static/js/*.js; src/web/routes/*.py]

## Common Pitfalls

### Pitfall 1: Assuming the Existing Go Accounts Route Is “Good Enough”
**What goes wrong:** Planner treats current `GET /api/accounts` as finished and only fills in the missing POST/PATCH endpoints. [VERIFIED: backend-go/internal/accounts/http/handlers.go]
**Why it happens:** The route exists, but its decoder only accepts `page` and `page_size`, and its SQL list query does not fetch several fields the current UI renders. [VERIFIED: backend-go/internal/accounts/http/handlers.go; backend-go/internal/accounts/repository_postgres.go; static/js/accounts.js]
**How to avoid:** Start with a field/query diff against `src/web/routes/accounts.py` and the current JS, then add a compatibility DTO layer before expanding handlers. [VERIFIED: src/web/routes/accounts.py; static/js/accounts.js; static/js/accounts_overview.js]
**Warning signs:** Blank email-service/subscription cells, broken team-management link behavior, or filters silently doing nothing. [VERIFIED: static/js/accounts.js; static/js/accounts_state_actions.js]

### Pitfall 2: Treating Upload Config Management as Already Migrated
**What goes wrong:** Planner sees `internal/uploader` and assumes CPA/Sub2API/TM admin APIs already exist in Go. [VERIFIED: backend-go/internal/uploader/]
**Why it happens:** `internal/uploader` already reads config rows and knows how to send payloads, but it has no CRUD handlers/services. [VERIFIED: backend-go/internal/uploader/repository_postgres.go; backend-go/internal/http/router.go]
**How to avoid:** Split “upload target config CRUD” from “account upload transport” and extend the same package for both. [VERIFIED: src/web/routes/upload/cpa_services.py; src/web/routes/upload/sub2api_services.py; src/web/routes/upload/tm_services.py]
**Warning signs:** Reimplemented payload builders, duplicate HTTP client code, or new config types that do not map back to the current tables. [VERIFIED: backend-go/internal/uploader/builder.go; backend-go/internal/uploader/sender.go]

### Pitfall 3: Losing Python Settings Metadata
**What goes wrong:** Go settings code preserves only `key/value` and silently drops `description`, `category`, or `updated_at`. [VERIFIED: .planning/phases/01-compatibility-baseline/01-shared-schema-contract.md; src/database/models.py]
**Why it happens:** Current Go settings readers only fetch `key, value`, while Python settings storage keeps richer metadata. [VERIFIED: backend-go/internal/registration/available_services_postgres.go; src/database/crud.py; src/database/models.py]
**How to avoid:** Treat `settings` admin work as schema-and-API parity, not just value lookup parity. [VERIFIED: src/web/routes/settings.py; src/config/settings.py]
**Warning signs:** Settings page can save values but loses descriptions/categories, or DB rows stop updating `updated_at`. [VERIFIED: src/database/crud.py]

### Pitfall 4: Accidentally Pulling Payment/Team Scope Forward
**What goes wrong:** Account-page work starts migrating payment/session-bootstrap or team workflows “because the buttons are on the same page.” [VERIFIED: static/js/accounts.js]
**Why it happens:** `accounts.js` mixes management calls with Phase 4 payment/team calls. [VERIFIED: static/js/accounts.js]
**How to avoid:** Keep mixed ownership explicit: move only management routes in Phase 3 and leave payment/team endpoints under their current owner. [VERIFIED: .planning/phases/03-management-apis/03-CONTEXT.md]
**Warning signs:** New Go plans/tasks mention bind-card/session bootstrap, subscription sync, or team invite/discovery handlers. [VERIFIED: .planning/REQUIREMENTS.md; .planning/phases/03-management-apis/03-CONTEXT.md]

### Pitfall 5: Replacing `app_logs` With `job_logs`
**What goes wrong:** `/api/logs` starts returning job/task log output instead of the app-wide admin log table. [VERIFIED: .planning/phases/01-compatibility-baseline/01-shared-schema-contract.md]
**Why it happens:** Go already has `job_logs` and `/api/jobs/{jobID}/logs`, so it is tempting to reuse them. [VERIFIED: backend-go/internal/jobs/http/handlers.go]
**How to avoid:** Preserve `app_logs` as its own storage/handler concern, even if implementation ends up sharing helper code. [VERIFIED: src/web/routes/logs.py; src/database/models.py]
**Warning signs:** Missing `level/logger/module/message/exception` fields or pagination filters no longer matching the logs page. [VERIFIED: src/web/routes/logs.py; static/js/logs.js]

## Code Examples

Verified patterns from the repository:

### Account Filter Serialization
```javascript
// Source: static/js/accounts_state_actions.js
return {
    status: normalizeText(filters.status),
    email_service: normalizeText(filters.email_service),
    refresh_token_state: normalizeText(filters.refresh_token_state),
    search: normalizeText(filters.search),
};
```

### Settings Key Definitions
```python
# Source: src/config/settings.py
"proxy_dynamic_api_url": SettingDefinition(
    db_key="proxy.dynamic_api_url",
    default_value="",
    category=SettingCategory.PROXY,
    description="动态代理 API 地址，返回代理 URL 字符串"
)
```

### Upload Config Query Pattern
```go
// Source: backend-go/internal/uploader/repository_postgres.go
SELECT id, name, api_url, api_key, COALESCE(target_type, ''), enabled, priority
FROM sub2api_services
```

### Logs Pagination/Filter Contract
```javascript
// Source: static/js/logs.js
const params = new URLSearchParams({
    page: String(logsState.page),
    page_size: String(logsState.pageSize),
});
if (logsState.filters.level) params.set("level", logsState.filters.level);
if (logsState.filters.logger_name) params.set("logger_name", logsState.filters.logger_name);
if (logsState.filters.keyword) params.set("keyword", logsState.filters.keyword);
if (logsState.filters.since_minutes) params.set("since_minutes", logsState.filters.since_minutes);
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Python-only registration and management backend | Registration/jobs/runtime baseline already exists in Go, but management APIs are still mostly Python-owned. [VERIFIED: .planning/STATE.md; backend-go/internal/http/router.go] | Phase 2 completed on 2026-04-05. [VERIFIED: .planning/STATE.md] | Phase 3 should extend existing Go foundations rather than restart migration design. |
| Upload side effects fully driven by Python helper code | Go runner-side upload builders/senders already cover CPA/Sub2API/TM payload transport. [VERIFIED: backend-go/internal/uploader/builder.go; backend-go/internal/uploader/sender.go] | Phase 2 baseline. [VERIFIED: .planning/phases/03-management-apis/03-CONTEXT.md] | Account upload endpoints should reuse the same transport code and only add admin-facing orchestration. |
| Proxy selection interpreted only by Python settings/runtime | Go registration already interprets static/dynamic proxy settings and can consume a legacy proxy pool when compatible. [VERIFIED: backend-go/internal/registration/proxy_selector.go] | Phase 2 baseline. [VERIFIED: .planning/STATE.md] | Proxy admin APIs should align with this existing Go behavior instead of inventing a new model. |

**Deprecated/outdated:**
- Treating the current Go `/api/accounts` list route as full Phase 3 coverage is outdated; it is only a baseline compatibility slice. [VERIFIED: .planning/phases/01-compatibility-baseline/01-route-client-parity-matrix.md; backend-go/internal/accounts/http/handlers.go]
- Treating `job_logs` as the future `/api/logs` source is outdated for this phase; the schema contract explicitly keeps `app_logs` separate. [VERIFIED: .planning/phases/01-compatibility-baseline/01-shared-schema-contract.md]

## Open Questions

1. **How should `/api/settings/database/*` behave on the Go production path?**
   - What we know: The current settings page calls `/settings/database`, `/settings/database/backup`, `/settings/database/import`, and `/settings/database/cleanup`; the Python implementation is explicitly SQLite/file oriented, while Go bootstrap requires `DATABASE_URL` and `REDIS_ADDR`. [VERIFIED: static/js/settings.js; src/web/routes/settings.py; backend-go/internal/config/config.go]
   - What’s unclear: Whether Phase 3 should fully reimplement these operations for Postgres, keep a bounded compatibility bridge for SQLite installs, or split them into a legacy-only sub-plan. [VERIFIED: codebase grep]
   - Recommendation: Isolate database-admin endpoints as their own plan item early; do not let them silently hide inside “settings parity.” [VERIFIED: static/js/settings.js; src/web/routes/settings.py]

2. **Should Phase 3 preserve `settings.description/category/updated_at` and `email_services.last_used` as first-class Go API data?**
   - What we know: Python storage and responses include those fields, while current Go shared-schema readers do not surface them. [VERIFIED: src/database/models.py; src/database/crud.py; src/web/routes/email.py; backend-go/internal/registration/available_services_postgres.go; .planning/phases/01-compatibility-baseline/01-shared-schema-contract.md]
   - What’s unclear: Whether the first Go management cut should carry full metadata parity immediately or preserve it only at the database layer and defer exposing it selectively. [VERIFIED: codebase grep]
   - Recommendation: Preserve them now; they are already part of the live row contract and at least one current page renders `last_used`. [VERIFIED: static/js/settings.js; static/js/email_services.js]

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | `backend-go/` build/tests | ✓ but version mismatch | local `1.24.3`, repo target `1.25.0` | Use repository/unit/httptest-level analysis now; upgrade Go before full Phase 3 implementation/test execution. [VERIFIED: backend-go/go.mod; local `go version`] |
| Python | Python oracle tests / current route fixtures | ✓ | `3.11.9` | — [VERIFIED: pyproject.toml; local `python3 --version`] |
| PostgreSQL service | Real Go API bring-up, repository integration, migration tests | ✗ not reachable locally | `—` | Use handler/service unit tests and Python oracle fixtures until a Postgres environment is provided. [VERIFIED: local `nc -z localhost 5432`; local `psql --version`; local `pg_isready`] |
| Redis service | Real `cmd/api` bootstrap and full Go server bring-up | ✗ not reachable locally | `—` | Use router/httptest/service tests that bypass `cmd/api` bootstrap for planning and contract work. [VERIFIED: backend-go/internal/config/config.go; local `nc -z localhost 6379`; local `redis-cli --version`] |
| `uv` | Python dependency/bootstrap path | ✓ | `0.9.13` | Use `pip` if needed, but `uv` is available. [VERIFIED: local `uv --version`; pyproject.toml] |

**Missing dependencies with no fallback:**
- None for planning-only work. Real end-to-end Go server execution is blocked until Postgres and Redis are available. [VERIFIED: backend-go/internal/config/config.go; local port checks]

**Missing dependencies with fallback:**
- PostgreSQL and Redis are absent locally, but contract work can still proceed with Python oracle tests, Go unit tests, and `httptest`-based router tests. [VERIFIED: tests/test_accounts_routes.py; backend-go/internal/accounts/http/handlers_test.go; backend-go/tests/e2e/accounts_flow_test.go]

## Verification Strategy

- Build Python-oracle fixtures first for every migrated route family: capture the exact request/response semantics from `src/web/routes/accounts.py`, `settings.py`, `email.py`, `logs.py`, and the upload-config routes before writing Go handlers. [VERIFIED: src/web/routes/accounts.py; src/web/routes/settings.py; src/web/routes/email.py; src/web/routes/logs.py; src/web/routes/upload/cpa_services.py; src/web/routes/upload/sub2api_services.py; src/web/routes/upload/tm_services.py]
- Keep page-level compatibility tests focused on the current JS entrypoints, not on imagined API clients: `accounts.js`, `accounts_overview.js`, `settings.js`, `email_services.js`, and `logs.js` are the real consumers. [VERIFIED: static/js/accounts.js; static/js/accounts_overview.js; static/js/settings.js; static/js/email_services.js; static/js/logs.js]
- Verify mixed ownership explicitly on the accounts page: management endpoints may move to Go, but `/payment/accounts/*` calls and team navigation must remain unchanged and still function against their current owner. [VERIFIED: static/js/accounts.js; .planning/phases/03-management-apis/03-CONTEXT.md]
- Prefer layered automated checks: Go repository/service tests for parity logic, Go handler/router tests for JSON/path compatibility, Python `TestClient` fixtures as the oracle, and a final browser/API smoke over the unchanged templates/static JS. [VERIFIED: backend-go/internal/accounts/service_test.go; backend-go/internal/accounts/http/handlers_test.go; backend-go/tests/e2e/accounts_flow_test.go; tests/test_accounts_routes.py]
- Do not count existing coverage as sufficient for the whole phase: there is account-route coverage, but there are no comparable Python route tests for email or logs in the current repo, and Go has no management handler tests for those domains yet. [VERIFIED: tests/test_accounts_routes.py; tests/test_settings_routes.py; codebase grep]

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | Phase 3 does not introduce a new login/authentication flow; preserve the existing console auth boundary. [VERIFIED: .planning/phases/03-management-apis/03-CONTEXT.md] |
| V3 Session Management | no | Phase 3 should not change payment/bind-card session handling, which remains Phase 4 scope. [VERIFIED: .planning/phases/03-management-apis/03-CONTEXT.md] |
| V4 Access Control | yes | Keep the admin-management routes behind the same existing console boundary and do not widen access while splitting route ownership. [ASSUMED] |
| V5 Input Validation | yes | Preserve typed request validation and explicit enum/filter checks for status, refresh-token filters, cleanup bounds, and service payloads. [VERIFIED: src/web/routes/accounts.py; src/web/routes/settings.py; src/web/routes/email.py; src/web/routes/logs.py] |
| V6 Cryptography | yes | The phase moves APIs that handle API keys, passwords, refresh tokens, cookies, and other secrets; do not invent a new storage/encryption scheme mid-migration. [VERIFIED: src/config/settings.py; src/web/routes/email.py; src/web/routes/upload/cpa_services.py; src/web/routes/upload/sub2api_services.py; src/web/routes/upload/tm_services.py] |

### Known Threat Patterns for This Phase

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Returning secret-bearing config in list endpoints | Information Disclosure | Preserve the current split between filtered list responses (`has_key`, `has_token`, `has_api_key`) and full edit endpoints (`/full`). [VERIFIED: src/web/routes/email.py; src/web/routes/upload/cpa_services.py; src/web/routes/upload/sub2api_services.py] |
| Batch admin endpoints accepting malformed filters or destructive payloads | Tampering | Keep typed request models plus explicit validation for `status`, `refresh_token_state`, cleanup bounds, and required IDs. [VERIFIED: src/web/routes/accounts.py; src/web/routes/logs.py; src/web/routes/settings.py] |
| Phase bleed into payment/team admin surfaces | Elevation of Privilege / Logic Abuse | Leave payment/team routes on their current owner and preserve only the read-only account fields needed by management pages. [VERIFIED: .planning/phases/03-management-apis/03-CONTEXT.md; static/js/accounts.js] |
| Reusing logs or config data outside their intended domain | Information Disclosure / Tampering | Keep `app_logs`, email-service configs, upload-service configs, and proxy configs in dedicated admin slices with domain-specific DTOs. [VERIFIED: src/database/models.py; src/web/routes/logs.py; src/web/routes/email.py; src/web/routes/settings.py] |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The existing console access boundary should be preserved as the access-control standard for new Go management routes. | Security Domain | Could under-specify access-control work if the current boundary is weaker or implemented differently than assumed. |

## Sources

### Primary (HIGH confidence)
- `.planning/phases/03-management-apis/03-CONTEXT.md` - phase scope, locked decisions, and explicit Phase 4 exclusions
- `.planning/REQUIREMENTS.md` - requirement IDs `MGMT-01`, `MGMT-02`, `CUT-01`
- `.planning/STATE.md` - current milestone status and Phase 2 baseline context
- `.planning/phases/01-compatibility-baseline/01-route-client-parity-matrix.md` - current route/page ownership map
- `.planning/phases/01-compatibility-baseline/01-shared-schema-contract.md` - shared-vs-Python-only schema contract and known gaps
- `src/web/routes/accounts.py` - Python account-management contract
- `src/web/routes/settings.py` - Python settings/proxy/database contract
- `src/web/routes/email.py` - Python email-service contract
- `src/web/routes/logs.py` - Python app-log contract
- `src/web/routes/upload/cpa_services.py` - CPA config-admin contract
- `src/web/routes/upload/sub2api_services.py` - Sub2API config-admin contract
- `src/web/routes/upload/tm_services.py` - TM config-admin contract
- `src/config/settings.py` - authoritative db-key map and settings metadata/secret semantics
- `src/database/models.py`, `src/database/crud.py`, `src/database/session.py` - persisted row shapes and update semantics
- `backend-go/internal/http/router.go` - currently wired Go routes
- `backend-go/internal/accounts/*` - existing Go account service/repository/handler baseline
- `backend-go/internal/uploader/*` - existing Go uploader config and transport baseline
- `backend-go/internal/registration/available_services_postgres.go`, `outlook_repository_postgres.go`, `proxy_selector.go` - current Go read-side foundations for settings/email/proxy domains
- `backend-go/db/migrations/0002_init_accounts_registration.sql`, `0003_extend_registration_service_configs.sql` - current Go-managed schema coverage
- `templates/accounts.html`, `templates/accounts_overview.html`, `templates/settings.html`, `templates/email_services.html`, `templates/logs.html` - current page composition and script ownership
- `static/js/accounts.js`, `accounts_overview.js`, `accounts_state_actions.js`, `settings.js`, `email_services.js`, `logs.js` - real frontend contracts
- `tests/test_accounts_routes.py`, `tests/test_settings_routes.py`, `backend-go/internal/accounts/http/handlers_test.go`, `backend-go/tests/e2e/accounts_flow_test.go` - current automated coverage and gaps

### Secondary (MEDIUM confidence)
- Local environment probes: `go version`, `python3 --version`, `uv --version`, `nc -z localhost 5432`, `nc -z localhost 6379`, `psql --version`, `redis-cli --version`, `pg_isready` - current machine/tool/service availability

### Tertiary (LOW confidence)
- None

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - repo-pinned versions and currently wired packages are explicit in `go.mod`, `pyproject.toml`, and current code. [VERIFIED: backend-go/go.mod; pyproject.toml]
- Architecture: HIGH - the current slice boundaries and missing domains are directly visible in `internal/http/router.go`, Python routes, and current JS consumers. [VERIFIED: backend-go/internal/http/router.go; src/web/routes/*.py; static/js/*.js]
- Pitfalls: HIGH - each listed pitfall is grounded in an observed code/contract mismatch, not a generic migration guess. [VERIFIED: codebase grep]

**Research date:** 2026-04-05
**Valid until:** 2026-05-05
