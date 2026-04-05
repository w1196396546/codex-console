# Architecture Research

**Domain:** Brownfield Python-to-Go backend migration for Codex Console
**Researched:** 2026-04-05
**Confidence:** HIGH

## Standard Architecture

### System Overview

```text
┌─────────────────────────────────────────────────────────────┐
│                Existing Templates / Static JS               │
├─────────────────────────────────────────────────────────────┤
│  /accounts  /settings  /payment  /team  /logs  /register   │
└──────────────────────────────┬──────────────────────────────┘
                               │
┌─────────────────────────────────────────────────────────────┐
│                 Go Compatibility HTTP Layer                 │
├─────────────────────────────────────────────────────────────┤
│  registration  accounts  admin/config  payment  team       │
└──────────────────────────────┬──────────────────────────────┘
                               │
┌─────────────────────────────────────────────────────────────┐
│                  Go Domain / Orchestration                  │
├─────────────────────────────────────────────────────────────┤
│  job service  native runner  upload dispatcher  adapters   │
└──────────────────────────────┬──────────────────────────────┘
                               │
┌─────────────────────────────────────────────────────────────┐
│                 Persistence and External Boundaries         │
├─────────────────────────────────────────────────────────────┤
│ PostgreSQL │ Redis/Asynq │ OpenAI │ Mail │ CPA/Sub2/TM │ UI │
└─────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

| Component | Responsibility | Typical Implementation |
|-----------|----------------|------------------------|
| Compatibility HTTP layer | Preserve current route paths, methods, payloads, and status semantics | `chi` handlers with compatibility DTOs and contract tests |
| Domain services | Own each migrated business area | `backend-go/internal/<domain>/` packages with service + repository split |
| Background execution | Run long-lived registration/payment/team tasks safely | `Asynq` workers backed by PostgreSQL/Redis |
| Persistence contract | Preserve current data shapes while consolidating on Go-owned stores | PostgreSQL migrations, `sqlc`, and compatibility reads/writes |
| Legacy transition boundary | Isolate any short-lived Python fallback | Explicit bridge adapters with a retirement phase, never hidden dependencies |

## Recommended Project Structure

```text
backend-go/
├── cmd/
│   ├── api/                # HTTP entrypoint
│   └── worker/             # Background worker entrypoint
├── internal/
│   ├── registration/       # Registration APIs, task lifecycle, Outlook batches
│   ├── accounts/           # Accounts CRUD, export/import, refresh, validate
│   ├── admin/              # Settings, proxies, email services, logs, upload configs
│   ├── payment/            # Payment and bind-card orchestration
│   ├── team/               # Team discovery/sync/invite/membership/task flows
│   ├── jobs/               # Shared job service and runtime logs
│   ├── uploader/           # CPA / Sub2API / TM side effects
│   ├── nativerunner/       # Native registration runtime
│   └── platform/           # PostgreSQL / Redis / shared infrastructure
└── db/
    ├── migrations/         # Explicit schema evolution
    └── query/              # sqlc query definitions
```

### Structure Rationale

- **`internal/<domain>/`** keeps migration work scoped by business boundary so Python and Go ownership can be compared one domain at a time.
- **`jobs/`** remains a shared substrate for every long-running flow, avoiding per-domain reinvention of pause/resume/cancel/log patterns.
- **`admin/`, `payment/`, and `team/`** should be added as first-class Go packages rather than extending `registration/` into another monolith.

## Architectural Patterns

### Pattern 1: Compatibility Facade

**What:** Freeze the current Python route contract at the Go handler boundary, then evolve internals behind it.
**When to use:** Whenever a Python-only endpoint is migrated but the client surface must stay unchanged.
**Trade-offs:** Slight duplication at the DTO layer, but much safer cutover behavior.

### Pattern 2: Domain Service + Repository

**What:** Keep orchestration logic in services and storage concerns in repositories.
**When to use:** Every migrated business domain that currently mixes routing, side effects, and persistence in Python.
**Trade-offs:** More files, but far better testability and parity auditing.

### Pattern 3: Explicit Transition Adapter

**What:** If Python fallback is still temporarily needed, isolate it behind a named adapter with a clear retirement milestone.
**When to use:** Only for narrow transition gaps that cannot be removed in the current phase.
**Trade-offs:** Safe short-term bridge, but dangerous if it becomes invisible or permanent.

## Data Flow

### Request Flow

```text
[Existing UI/client]
    ↓
[Go compatibility handler]
    ↓
[Domain service]
    ↓
[Repository / job service / side-effect adapter]
    ↓
[PostgreSQL / Redis / external provider]
```

### State Management

```text
[PostgreSQL]
    ↑ durable state
[Go services] ←→ [Redis/Asynq leases + work queue]
    ↓ status/log snapshots
[HTTP + WebSocket handlers]
    ↓
[Existing templates/static JS]
```

### Key Data Flows

1. **Registration flow:** client request -> Go compatibility handler -> job creation -> worker -> native runner -> account persistence -> optional uploads -> status/log streaming.
2. **Admin flow:** client request -> Go handler -> repository read/write -> compatible JSON response with unchanged field names and semantics.
3. **Domain cutover flow:** Python route contract inventory -> Go ownership -> client switch -> Python retirement.

## Scaling Considerations

| Scale | Architecture Adjustments |
|-------|--------------------------|
| Current operator scale | Monolithic Go API + worker is fine if parity is complete |
| Higher batch volume | Increase Redis/Asynq worker concurrency and remove any remaining Python bridge bottlenecks |
| Multi-instance deployment | Ensure every runtime state path is durable and not tied to process-local memory |

### Scaling Priorities

1. **First bottleneck:** lingering Python-only runtime state - move it into durable Go job/state infrastructure.
2. **Second bottleneck:** contract drift during incremental migration - prevent it with compatibility tests before each cutover.

## Anti-Patterns

### Anti-Pattern 1: Recreate the Python monolith inside Go

**What people do:** Move all remaining Python logic into one large Go package because the domain boundaries feel messy.
**Why it's wrong:** It reproduces the same maintainability problem in a new language.
**Do this instead:** Split by domain boundary and keep shared concerns in reusable infrastructure packages.

### Anti-Pattern 2: Cut clients over before parity is observable

**What people do:** Point current clients at Go once the "main path" seems to work.
**Why it's wrong:** Hidden route, payload, and task-state mismatches only show up in production.
**Do this instead:** Make parity explicit with route/data/workflow regression checks before every domain cutover.

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| OpenAI / Auth / Sentinel | Adapter boundary behind registration runtime | Avoid leaking provider quirks into HTTP handlers |
| Email providers | Prepared service config + provider adapters | Preserve current config keys and fallback semantics |
| CPA / Sub2API / TM | Side-effect dispatcher after account persistence | Keep idempotency and payload compatibility visible |
| Browser/payment automation | Isolated payment-domain orchestration | Do not mix these flows back into generic registration packages |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| handler <-> service | direct method calls with compatibility DTOs | Keep all payload translation at the edge |
| service <-> repository | explicit interfaces | Allows parity tests without live infrastructure |
| service <-> worker/runtime | jobs + runner interfaces | Prevents hidden Python fallback from leaking through the stack |

## Sources

- `.planning/codebase/ARCHITECTURE.md`
- `.planning/codebase/STRUCTURE.md`
- `backend-go/internal/http/router.go`
- `backend-go/cmd/api/main.go`
- `backend-go/cmd/worker/main.go`

---
*Architecture research for: Brownfield Python-to-Go backend migration for Codex Console*
*Researched: 2026-04-05*
