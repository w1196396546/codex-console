# Phase 2: Native Registration Runtime - Research

**Researched:** 2026-04-05. [VERIFIED: current session date]
**Domain:** Native registration runtime parity for the Python-to-Go migration. [VERIFIED: .planning/phases/02-native-registration-runtime/02-CONTEXT.md]
**Confidence:** MEDIUM. [VERIFIED: codebase analysis + local environment audit]

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
[VERIFIED: .planning/phases/02-native-registration-runtime/02-CONTEXT.md]
- **D-01:** Existing Python registration behavior remains the compatibility oracle for request/response semantics until the Go path proves parity.
- **D-02:** The Python runner bridge may be used only as a bounded transition aid during implementation; the completed Phase 2 critical path must not require it for normal registration execution.
- **D-03:** Task, batch, Outlook batch, websocket, pause/resume/cancel, and log offset semantics are part of the runtime contract and must remain compatible.
- **D-04:** Account persistence plus CPA/Sub2API/TM auto-upload side effects must remain compatible with current workflow timing and data shape.
- **D-05:** Treat existing Go registration/task/websocket compatibility surfaces as baseline foundations to extend, not to rewrite from scratch.
- **D-06:** Do not pull broader account-management, settings, payment, or team domain migration into this phase.

### Claude's Discretion
[VERIFIED: .planning/phases/02-native-registration-runtime/02-CONTEXT.md]
The agent may choose the exact internal decomposition for native registration execution, compatibility fixtures, and persistence adapters as long as the external runtime contract remains compatible and the Python bridge is removed from the completed critical path.

### Deferred Ideas (OUT OF SCOPE)
[VERIFIED: .planning/phases/02-native-registration-runtime/02-CONTEXT.md]
- Broader account-management endpoints remain Phase 3 scope.
- Payment/bind-card runtime migration remains Phase 4 scope.
- Team runtime migration remains Phase 4 scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|---|---|---|
| RUN-01 | Registration start, batch, and Outlook batch flows execute on Go-owned runtime logic without requiring the Python runner bridge on the critical path. | Extend the existing `internal/registration` HTTP/batch/outlook/ws shell plus the worker’s default native runner; close the remaining API/status/payload gaps instead of building a new runtime. [VERIFIED: .planning/REQUIREMENTS.md][VERIFIED: backend-go/cmd/worker/main.go][VERIFIED: backend-go/internal/registration/http/handlers.go][VERIFIED: backend-go/internal/registration/batch_service.go][VERIFIED: backend-go/internal/registration/outlook_service.go] |
| RUN-02 | Existing clients can list, inspect, pause, resume, cancel, and clean up registration tasks and batches through Go with current behavior. | Plan explicit parity work for `/api/registration/tasks` list, `DELETE /api/registration/tasks/{task_uuid}`, `cancelling` semantics, and websocket/polling offset behavior. [VERIFIED: .planning/REQUIREMENTS.md][VERIFIED: src/web/routes/registration.py][VERIFIED: src/web/routes/websocket.py][VERIFIED: backend-go/internal/registration/http/handlers.go][VERIFIED: backend-go/internal/registration/ws/task_socket.go][VERIFIED: backend-go/internal/registration/ws/batch_socket.go] |
| RUN-03 | Registration side effects continue to persist accounts and trigger CPA, Sub2API, and TM uploads with current semantics. | Use the existing executor persistence hook, `accounts.Service`, and `AutoUploadDispatcher`, while preserving Python’s “CPA/Sub2API write flags, TM log only” behavior. [VERIFIED: .planning/REQUIREMENTS.md][VERIFIED: src/core/register.py][VERIFIED: src/web/routes/registration.py][VERIFIED: backend-go/internal/registration/executor.go][VERIFIED: backend-go/internal/registration/auto_upload_dispatcher.go][VERIFIED: backend-go/internal/accounts/service.go] |
| COMP-03 | Existing polling and websocket consumers receive compatible task and batch progress semantics from Go-owned backend flows. | Preserve `status`, `paused`, `cancelled`, `finished`, `log_offset`, and `log_next_offset` semantics because current frontend JS uses them directly for control state, polling recovery, and websocket fallback. [VERIFIED: .planning/REQUIREMENTS.md][VERIFIED: static/js/app.js][VERIFIED: src/web/routes/registration.py][VERIFIED: src/web/routes/websocket.py][VERIFIED: backend-go/internal/registration/http/integration_test.go][VERIFIED: backend-go/internal/registration/ws/task_socket_test.go][VERIFIED: backend-go/internal/registration/ws/batch_socket_test.go] |
</phase_requirements>

## Summary

Python is still the compatibility oracle for registration execution, task lifecycle, batch and Outlook batch control, websocket behavior, and upload side effects, and the Phase 2 scope is explicitly to remove Python from the registration critical path without changing those observable semantics. [VERIFIED: .planning/phases/02-native-registration-runtime/02-CONTEXT.md][VERIFIED: src/web/routes/registration.py][VERIFIED: src/web/routes/websocket.py][VERIFIED: src/web/task_manager.py]

Go already owns the correct architectural skeleton: the worker boots a native runner by default, the registration HTTP/batch/outlook/websocket surfaces already exist, the executor already supports preparation, persistence, and auto-upload hooks, and the native runner already covers the mail-provider matrix plus password and passwordless token-completion flows. [VERIFIED: backend-go/cmd/worker/main.go][VERIFIED: backend-go/internal/registration/http/handlers.go][VERIFIED: backend-go/internal/registration/executor.go][VERIFIED: backend-go/internal/nativerunner/runner.go][VERIFIED: backend-go/internal/nativerunner/prepare_signup_flow.go][VERIFIED: backend-go/internal/nativerunner/mail/provider.go]

The planning problem is therefore contract closure, not greenfield runtime design: Go still drifts from Python on HTTP status codes and response shape for start/batch entrypoints, omits the task list and delete routes, differs on `cancelling` versus `cancelled` transitions, and leaves proxy selection plus Outlook reservation wiring incomplete in worker preparation. [VERIFIED: src/web/routes/registration.py][VERIFIED: backend-go/internal/registration/http/handlers.go][VERIFIED: backend-go/internal/registration/batch_service.go][VERIFIED: backend-go/cmd/worker/main.go]

**Primary recommendation:** Plan Phase 2 as a compatibility-closure wave over the existing Go `registration` + `nativerunner` + `jobs` + `accounts` + `uploader` modules, using Python only as an oracle and fixture source until Go matches route, status, log, and side-effect behavior. [VERIFIED: .planning/phases/02-native-registration-runtime/02-CONTEXT.md][VERIFIED: backend-go/internal/registration/executor.go][VERIFIED: backend-go/internal/nativerunner/prepare_signup_flow.go]

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---|---|---|---|
| `backend-go/internal/registration` (`Service`, `BatchService`, `OutlookService`, `Executor`) | repo current [VERIFIED: backend-go/internal/registration/service.go][VERIFIED: backend-go/internal/registration/batch_service.go][VERIFIED: backend-go/internal/registration/outlook_service.go][VERIFIED: backend-go/internal/registration/executor.go] | Own the registration API shell, batch state projection, Outlook batch projection, and execution orchestration. [VERIFIED: backend-go/internal/registration/http/handlers.go] | Already mounted in the router, already used by the worker path, and already covered by compatibility-style unit and integration tests. [VERIFIED: backend-go/internal/registration/http/handlers.go][VERIFIED: backend-go/internal/registration/http/handlers_test.go][VERIFIED: backend-go/internal/registration/http/integration_test.go] |
| `backend-go/internal/nativerunner` (`Runner`, `PrepareSignupFlow`, token completion, mail providers) | repo current [VERIFIED: backend-go/internal/nativerunner/runner.go][VERIFIED: backend-go/internal/nativerunner/prepare_signup_flow.go] | Execute native signup, OTP verification, create-account continuation, and existing-account token completion. [VERIFIED: backend-go/internal/nativerunner/prepare_signup_flow.go] | The worker already instantiates it as the default registration runner, so Phase 2 should extend this path instead of reintroducing Python-shape orchestration. [VERIFIED: backend-go/cmd/worker/main.go][VERIFIED: backend-go/cmd/worker/main_test.go] |
| `backend-go/internal/jobs` | repo current [VERIFIED: backend-go/internal/registration/executor.go][VERIFIED: backend-go/internal/registration/http/handlers.go] | Provide durable task state, logs, queue control, and pause/resume/cancel control points. [VERIFIED: backend-go/internal/registration/executor.go][VERIFIED: backend-go/internal/registration/http/handlers.go] | Go websocket and task APIs already derive state from jobs instead of inventing a second runtime store. [VERIFIED: backend-go/internal/registration/ws/task_socket.go][VERIFIED: backend-go/internal/registration/ws/batch_socket.go] |
| `backend-go/internal/accounts` | repo current [VERIFIED: backend-go/internal/accounts/service.go][VERIFIED: backend-go/internal/accounts/types.go] | Persist registration output, merge existing account state, and hold token-completion runtime metadata in `extra_data`. [VERIFIED: backend-go/internal/accounts/service.go][VERIFIED: backend-go/cmd/worker/main.go] | It already exposes the exact merge/writeback seam the executor and token-completion runtime need. [VERIFIED: backend-go/internal/registration/executor.go][VERIFIED: backend-go/internal/accounts/repository_postgres.go] |

### Supporting
| Library | Version | Purpose | When to Use |
|---|---|---|---|
| `backend-go/internal/uploader` + `AutoUploadDispatcher` | repo current [VERIFIED: backend-go/internal/uploader/sender.go][VERIFIED: backend-go/internal/registration/auto_upload_dispatcher.go] | Normalize CPA, Sub2API, and TM service reads, payload building, HTTP sending, and account writeback. [VERIFIED: backend-go/internal/uploader/builder.go][VERIFIED: backend-go/internal/registration/auto_upload_dispatcher.go] | Use for all registration-triggered uploads; do not duplicate upload code inside the runner or handlers. [VERIFIED: backend-go/internal/registration/executor.go] |
| `backend-go/internal/registration/ws` | repo current [VERIFIED: backend-go/internal/registration/ws/task_socket.go][VERIFIED: backend-go/internal/registration/ws/batch_socket.go] | Provide websocket snapshot + poll-loop compatibility for task and batch channels. [VERIFIED: backend-go/internal/registration/ws/task_socket.go][VERIFIED: backend-go/internal/registration/ws/batch_socket.go] | Use to preserve current frontend websocket behavior while the underlying runtime stays job-backed. [VERIFIED: static/js/app.js] |
| `backend-go/internal/registration/python_runner.go` | repo current [VERIFIED: backend-go/internal/registration/python_runner.go] | Keep a bounded bridge available during implementation and parity debugging. [VERIFIED: .planning/phases/02-native-registration-runtime/02-CONTEXT.md] | Use only as a temporary fallback or fixture aid; the completed Phase 2 critical path must not depend on it. [VERIFIED: .planning/phases/02-native-registration-runtime/02-CONTEXT.md] |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|---|---|---|
| Rebuilding registration/task/websocket flow from scratch in Go | Extend the existing `internal/registration` HTTP, batch, outlook, websocket, and executor layers. [VERIFIED: backend-go/internal/registration/http/handlers.go][VERIFIED: backend-go/internal/registration/executor.go] | Reuse preserves the current routing and test surface, but requires explicit compatibility-gap work instead of a “clean slate” rewrite. [VERIFIED: backend-go/internal/registration/http/handlers_test.go] |
| Keeping Python persistence and optional uploads inside the critical path | Keep runner output pure and persist/upload through `Executor` hooks plus `accounts.Service` and `AutoUploadDispatcher`. [VERIFIED: backend-go/internal/registration/executor.go][VERIFIED: backend-go/internal/registration/auto_upload_dispatcher.go] | This is the correct final path, but planners must add parity checks for persistence fields and upload log/writeback semantics before removing the bridge. [VERIFIED: backend-go/internal/registration/executor_persistence_test.go][VERIFIED: backend-go/internal/registration/python_runner_persistence_test.go] |

**Installation:** No new package installation is recommended for Phase 2 planning; extend the repo-local Go registration stack already present. [VERIFIED: backend-go/cmd/worker/main.go][VERIFIED: backend-go/internal/registration/http/handlers.go]

## Python Oracle Behaviors

- `POST /api/registration/start` creates a `registration_tasks` row immediately, schedules `run_registration_task` as a FastAPI background task, and returns the full `RegistrationTaskResponse` model with Python-default HTTP 200 semantics rather than a minimal accepted payload. [VERIFIED: src/web/routes/registration.py]
- `POST /api/registration/batch` and `POST /api/registration/outlook-batch` also return HTTP 200 with rich bodies, initialize process-local batch state, and then schedule asynchronous batch execution. [VERIFIED: src/web/routes/registration.py]
- Python task log reads always treat `log_next_offset` as the authoritative incremental cursor and prefer in-memory runtime logs over persisted `task.logs` when runtime state exists. [VERIFIED: src/web/routes/registration.py][VERIFIED: src/web/task_manager.py]
- Python batch and Outlook batch status reads use the same `log_offset` / `log_next_offset` contract and preserve a `log_base_index` when the in-memory batch log window has trimmed older messages. [VERIFIED: src/web/routes/registration.py][VERIFIED: src/web/task_manager.py]
- Python single-task pause updates the database row to `paused`, updates `task_manager`, and expects resume to restore either `pending` or `running` based on prior runtime state rather than forcing `running` unconditionally. [VERIFIED: src/web/routes/registration.py][VERIFIED: src/web/task_manager.py]
- Python single-task cancel is a two-step contract: the route and websocket handlers first set `cancelling`, and the worker thread eventually finalizes `cancelled` when the engine notices the cancel flag. [VERIFIED: src/web/routes/registration.py][VERIFIED: src/web/routes/websocket.py][VERIFIED: src/core/register.py]
- Python websocket channels send an initial snapshot, accept `ping/pause/resume/cancel`, and keep the connection alive with heartbeat-style `ping` frames when idle. [VERIFIED: src/web/routes/websocket.py]
- Python Outlook execution state intentionally ignores `account.status`; it classifies only by account existence and refresh-token completeness, and the frontend selector uses the same rule. [VERIFIED: src/web/routes/registration.py][VERIFIED: tests/test_registration_routes.py][VERIFIED: static/js/outlook_account_selector.js]
- Python backend start for Outlook batch is permissive even for “registered complete” accounts if the client explicitly sends those IDs, even though the frontend defaults to selecting only executable accounts. [VERIFIED: src/web/routes/registration.py][VERIFIED: tests/test_registration_routes.py][VERIFIED: static/js/outlook_account_selector.js]
- Python account persistence writes `email`, `password`, `client_id`, `session_token`, `cookies`, `email_service`, `email_service_id`, `account_id`, `workspace_id`, `access_token`, `refresh_token`, `id_token`, `proxy_used`, `extra_data`, `status`, and `source`, with status derived from refresh-token completeness and login-versus-register context. [VERIFIED: src/core/register.py]
- Python auto-upload side effects are asymmetric: CPA success writes `cpa_uploaded` and `cpa_uploaded_at`, Sub2API success writes `sub2api_uploaded` and `sub2api_uploaded_at`, and TM only emits logs without any success-flag writeback. [VERIFIED: src/web/routes/registration.py][VERIFIED: backend-go/internal/registration/python_runner_script.go][VERIFIED: tests/test_sub2api_upload.py]

## Go Foundations To Extend

- The Go worker already constructs a native registration runner by default through `newWorkerRegistrationRunner()` and wraps it with `registration.NewNativeRunner(...)`; this is already the baseline critical-path implementation to extend. [VERIFIED: backend-go/cmd/worker/main.go][VERIFIED: backend-go/cmd/worker/main_test.go]
- `registration.Executor` already gives Phase 2 the right seam order: decode request, normalize and prepare config, enforce batch admission, run the runner, persist the account, then dispatch auto-upload side effects. [VERIFIED: backend-go/internal/registration/executor.go]
- `registration.BatchService` already models batch state as an aggregation over child jobs plus incremental log offsets, which is the correct durable alternative to Python’s process-local `batch_tasks`. [VERIFIED: backend-go/internal/registration/batch_service.go]
- `registration.OutlookService` already mirrors the current Outlook account shape (`has_oauth`, `is_registered`, `has_refresh_token`, `needs_token_refresh`, `is_registration_complete`, `registered_account_id`) and already builds Outlook batch requests on top of the shared batch service. [VERIFIED: backend-go/internal/registration/outlook_service.go][VERIFIED: backend-go/internal/registration/outlook_service_test.go]
- `registration/ws/task_socket.go` and `registration/ws/batch_socket.go` already implement the required websocket pattern: send current snapshot first, then poll for changed status or new logs, while still honoring client `ping/pause/resume/cancel` commands. [VERIFIED: backend-go/internal/registration/ws/task_socket.go][VERIFIED: backend-go/internal/registration/ws/batch_socket.go]
- `nativerunner/mail/provider.go` already covers `tempmail`, `yyds_mail`, `freemail`, `duck_mail`, `luckmail`, `imap_mail`, `outlook`, and `moe_mail`; Phase 2 should not invent a second provider abstraction. [VERIFIED: backend-go/internal/nativerunner/mail/provider.go][VERIFIED: backend-go/internal/nativerunner/mail/provider_test.go]
- `nativerunner.PrepareSignupFlow` already covers both the straight registration path and the “existing account” token-completion path, including passwordless and password strategies, runtime cooldown/spacing, and Redis-backed lease coordination when wired by the worker. [VERIFIED: backend-go/internal/nativerunner/prepare_signup_flow.go][VERIFIED: backend-go/internal/nativerunner/prepare_signup_flow_test.go][VERIFIED: backend-go/internal/nativerunner/token_completion.go][VERIFIED: backend-go/cmd/worker/main.go]
- `accounts.Service.UpsertAccount()` already merges rather than blindly overwriting persisted account fields, and it intentionally preserves a stronger existing status when a refresh-token-bearing account receives a partial `token_pending` or `login_incomplete` update. [VERIFIED: backend-go/internal/accounts/service.go][VERIFIED: backend-go/internal/accounts/service_test.go]
- `AutoUploadDispatcher` already preserves the Python writeback asymmetry by marking only CPA and Sub2API success flags while leaving TM as log-only. [VERIFIED: backend-go/internal/registration/auto_upload_dispatcher.go][VERIFIED: backend-go/internal/registration/auto_upload_dispatcher_test.go]

## Architecture Patterns

### Recommended Project Structure
```text
backend-go/
├── cmd/worker/                    # Native registration worker bootstrap and dependency wiring
├── internal/registration/         # API compatibility shell, batch/outlook services, executor
├── internal/registration/ws/      # Task and batch websocket compatibility layer
├── internal/nativerunner/         # Native signup + token completion + mail provider runtime
├── internal/accounts/             # Persisted account merge/writeback boundary
├── internal/uploader/             # CPA/Sub2API/TM payloads and HTTP senders
└── internal/jobs/                 # Durable task state, logs, pause/resume/cancel control
```

### Pattern 1: Prepare -> Run -> Persist -> Upload
**What:** Keep request normalization and provider/config lookup in `orchestrator`, keep native execution in `nativerunner`, and keep account persistence plus auto-upload side effects in the executor hook chain. [VERIFIED: backend-go/internal/registration/orchestrator.go][VERIFIED: backend-go/internal/registration/executor.go]
**When to use:** Use this for every single registration, batch child task, and Outlook batch child task. [VERIFIED: backend-go/internal/registration/executor.go][VERIFIED: backend-go/internal/registration/batch_service.go][VERIFIED: backend-go/internal/registration/outlook_service.go]
**Example:**
```go
// Source: backend-go/internal/registration/executor.go
prepared, err := e.preparer.Prepare(ctx, job.JobID, req)
output, err := e.runner.Run(ctx, registration.RunnerRequest{
    TaskUUID:             job.JobID,
    StartRequest:         prepared.Request,
    Plan:                 prepared.Plan,
    GoPersistenceEnabled: e.accountPersistence != nil,
    control:              e.runnerControl(job.JobID),
}, logf)
```

### Pattern 2: Jobs Are the Go Task Source of Truth
**What:** Read status and logs from `jobs.Service`, and project task/batch compatibility responses from that data instead of creating a second Go-local task manager. [VERIFIED: backend-go/internal/registration/http/handlers.go][VERIFIED: backend-go/internal/registration/batch_service.go][VERIFIED: backend-go/internal/registration/ws/task_socket.go]
**When to use:** Use for task detail, task logs, batch status, websocket snapshots, and resume/pause/cancel behavior. [VERIFIED: backend-go/internal/registration/http/handlers.go][VERIFIED: backend-go/internal/registration/ws/task_socket.go][VERIFIED: backend-go/internal/registration/ws/batch_socket.go]
**Example:**
```go
// Source: backend-go/internal/registration/batch_service.go
jobLogs, err := s.jobs.ListJobLogs(ctx, taskUUID)
start := record.logOffsets[taskUUID]
for _, item := range jobLogs[start:] {
    record.logs = append(record.logs, item.Message)
}
record.logOffsets[taskUUID] = len(jobLogs)
```

### Pattern 3: WebSocket Snapshot + Poll Loop Compatibility
**What:** Send a full current snapshot first, then stream new logs and state changes by polling the durable task source. [VERIFIED: backend-go/internal/registration/ws/task_socket.go][VERIFIED: backend-go/internal/registration/ws/batch_socket.go]
**When to use:** Use for both `/api/ws/task/{task_uuid}` and `/api/ws/batch/{batch_id}` because the current frontend reconnects, hydrates, and falls back to polling around this contract. [VERIFIED: static/js/app.js][VERIFIED: src/web/routes/websocket.py]
**Example:**
```go
// Source: backend-go/internal/registration/ws/task_socket.go
if err := h.sendSnapshot(ctx, socket, state, taskUUID); err != nil {
    return
}
go func() { errs <- h.readLoop(ctx, socket, state, taskUUID) }()
go func() { errs <- h.pollLoop(ctx, socket, state, taskUUID) }()
```

### Compatibility Hotspots To Plan Explicitly

- Go `StartRegistration`, `StartBatch`, and `StartOutlookBatch` currently return HTTP 202 with minimal accepted payloads, while Python currently returns HTTP 200 with richer response models; this is a visible compatibility drift and should be treated as a stop-ship item for Phase 2 planning. [VERIFIED: backend-go/internal/registration/http/handlers.go][VERIFIED: backend-go/internal/registration/http/handlers_test.go][VERIFIED: src/web/routes/registration.py]
- Go currently exposes task detail, task logs, and task controls, but it still omits `/api/registration/tasks` listing and `DELETE /api/registration/tasks/{task_uuid}`, both of which are present in Python and consumed during page restore and operator cleanup workflows. [VERIFIED: backend-go/internal/registration/http/handlers.go][VERIFIED: src/web/routes/registration.py][VERIFIED: static/js/app.js]
- Go single-task cancel currently surfaces `jobs.StatusCancelled` immediately through the task control and task websocket paths, while Python explicitly shows `cancelling` first and finalizes `cancelled` only after the worker exits. [VERIFIED: backend-go/internal/registration/http/handlers.go][VERIFIED: backend-go/internal/registration/ws/task_socket_test.go][VERIFIED: src/web/routes/registration.py][VERIFIED: src/web/routes/websocket.py]
- Go batch cancel currently returns `status: cancelled` from the HTTP control endpoint but forces a websocket-side intermediate `cancelling` snapshot before the final cancelled state; Python HTTP returns only `{success,message}` while runtime state transitions through `cancelling`. [VERIFIED: backend-go/internal/registration/batch_service.go][VERIFIED: backend-go/internal/registration/http/handlers.go][VERIFIED: backend-go/internal/registration/ws/batch_socket.go][VERIFIED: src/web/routes/registration.py][VERIFIED: src/web/routes/websocket.py]
- Go task websocket messages do not currently include the Python `timestamp` field, even though the Python websocket producers attach timestamps to both log and status frames. [VERIFIED: backend-go/internal/registration/ws/task_socket.go][VERIFIED: backend-go/internal/registration/ws/batch_socket.go][VERIFIED: src/web/task_manager.py][VERIFIED: src/web/routes/websocket.py]
- The worker’s preparation dependencies currently wire settings, email-service catalog, and Outlook readers, but not a `ProxySelector` or `OutlookReservationStore`, so Python’s current “pick a proxy when absent” and “reserve one Outlook service across concurrent tasks” behavior is not fully mirrored in the Go worker path yet. [VERIFIED: backend-go/cmd/worker/main.go][VERIFIED: backend-go/internal/registration/orchestrator.go][VERIFIED: src/web/routes/registration.py]
- Python still contains `native` and `abcard` entry-flow branches plus add-phone/workspace/session bridge fallback behavior; Go native runner already covers post-signup continuation and token completion, but final parity still depends on fixture-level proof that Go handles the same existing-account and post-signup boundary cases the Python engine already heals around. [VERIFIED: src/core/register.py][VERIFIED: backend-go/internal/nativerunner/prepare_signup_flow.go][VERIFIED: backend-go/internal/nativerunner/prepare_signup_flow_test.go][VERIFIED: backend-go/internal/nativerunner/token_completion_passwordless_test.go]
- Python and the bridge script both preserve the current upload writeback asymmetry of “CPA/Sub2API set flags, TM logs only”; planners should treat any TM flag writeback as a behavior change, not a cleanup. [VERIFIED: src/web/routes/registration.py][VERIFIED: backend-go/internal/registration/python_runner_script.go][VERIFIED: backend-go/internal/registration/auto_upload_dispatcher.go]

### Anti-Patterns to Avoid

- **Do not recreate `task_manager` in Go:** Python’s in-memory task manager is the oracle for observable behavior, not the model to copy into the Go runtime. [VERIFIED: src/web/task_manager.py][VERIFIED: backend-go/internal/registration/batch_service.go]
- **Do not move persistence or uploads back into the runner:** Go already has explicit executor hooks for persistence and side effects, and collapsing them back into runner code would make parity harder to prove. [VERIFIED: backend-go/internal/registration/executor.go]
- **Do not “fix” Outlook skip behavior during Phase 2:** Current backend semantics are permissive, while the frontend selector is where executable filtering happens by default. [VERIFIED: src/web/routes/registration.py][VERIFIED: static/js/outlook_account_selector.js][VERIFIED: tests/test_registration_routes.py]
- **Do not normalize status vocabulary or omit fields the frontend already reads:** The current page logic directly branches on `paused`, `cancelling`, `cancelled`, `finished`, `log_offset`, and `log_next_offset`. [VERIFIED: static/js/app.js]

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---|---|---|---|
| Registration orchestration | A new monolithic “Go version of `web/routes/registration.py`” task engine. [VERIFIED: src/web/routes/registration.py] | `registration.Executor` + `orchestrator` + `nativerunner.Runner`. [VERIFIED: backend-go/internal/registration/executor.go][VERIFIED: backend-go/internal/registration/orchestrator.go][VERIFIED: backend-go/internal/nativerunner/runner.go] | The seams already exist and already have targeted tests for preparation, control, admission, persistence, and native-runner boundaries. [VERIFIED: backend-go/internal/registration/executor_test.go][VERIFIED: backend-go/internal/registration/executor_control_test.go][VERIFIED: backend-go/internal/registration/native_runner_boundary_test.go] |
| Batch coordination | A new scheduler-local batch state store. [VERIFIED: src/web/routes/registration.py] | `registration.BatchService` plus job-derived aggregation and `registration/ws/batch_socket.go`. [VERIFIED: backend-go/internal/registration/batch_service.go][VERIFIED: backend-go/internal/registration/ws/batch_socket.go] | Existing Go batch logic already understands concurrency, interval gating, incremental logs, and pause/resume/cancel. [VERIFIED: backend-go/internal/registration/executor_admission_test.go][VERIFIED: backend-go/internal/registration/batch_service_test.go] |
| Mail-provider clients | Ad hoc HTTP/IMAP clients inside the worker. [VERIFIED: backend-go/internal/nativerunner/runner.go] | `nativerunner/mail.NewProvider()` and existing provider implementations. [VERIFIED: backend-go/internal/nativerunner/mail/provider.go] | The provider matrix is already implemented and tested, including OTP freshness and IMAP fallback behavior. [VERIFIED: backend-go/internal/nativerunner/mail/provider_test.go][VERIFIED: backend-go/internal/nativerunner/mail/outlook_test.go][VERIFIED: backend-go/internal/nativerunner/mail/imap_test.go] |
| Upload dispatch | Per-route or per-runner CPA/Sub2API/TM HTTP code. [VERIFIED: src/web/routes/registration.py] | `internal/uploader` senders plus `AutoUploadDispatcher`. [VERIFIED: backend-go/internal/uploader/sender.go][VERIFIED: backend-go/internal/registration/auto_upload_dispatcher.go] | This avoids duplicating service lookup, payload building, and writeback asymmetry rules. [VERIFIED: backend-go/internal/registration/auto_upload_dispatcher_test.go] |
| Token completion concurrency control | New goroutine-local mutexes or per-worker memory fences. [VERIFIED: backend-go/cmd/worker/main.go] | `TokenCompletionCoordinator` with runtime store + Redis lease store. [VERIFIED: backend-go/internal/nativerunner/token_completion.go][VERIFIED: backend-go/cmd/worker/main.go] | Existing code already models spacing, cooldown, retryable failure backoff, stale-lease detection, and heartbeat renewal. [VERIFIED: backend-go/internal/nativerunner/token_completion_test.go] |

**Key insight:** Most of Phase 2 should be planned as “close contract gaps around existing Go primitives,” not “invent new primitives.” [VERIFIED: backend-go/internal/registration/executor.go][VERIFIED: backend-go/internal/nativerunner/prepare_signup_flow.go]

## Runtime State Inventory

| Category | Items Found | Action Required |
|---|---|---|
| Stored data | Python still writes `registration_tasks` rows plus persisted `logs/result/error_message`, while Go task state lives in `jobs`/`job_logs`; Go token-completion runtime also persists attempt and cooldown metadata inside `accounts.extra_data`. [VERIFIED: src/web/routes/registration.py][VERIFIED: src/core/register.py][VERIFIED: backend-go/internal/registration/http/handlers.go][VERIFIED: backend-go/internal/nativerunner/token_completion.go][VERIFIED: backend-go/internal/accounts/repository_postgres.go] | **Code edit + parity verification.** No one-off data reshape is evident in the repo, but planners must explicitly choose the Go durable source of truth and verify that task/result/account fields still satisfy Python-era consumers before removing the bridge. [VERIFIED: backend-go/internal/registration/executor.go][VERIFIED: src/core/register.py] |
| Live service config | Email-service configs, Outlook credentials, and CPA/Sub2API/TM service configs are read from database-backed repositories in both the Python path and the Go path. [VERIFIED: src/web/routes/registration.py][VERIFIED: backend-go/internal/registration/available_services_postgres.go][VERIFIED: backend-go/internal/registration/outlook_repository_postgres.go][VERIFIED: backend-go/internal/uploader/repository_postgres.go] | **Code edit only.** Preserve current table shapes and lookup rules; no separate SaaS UI config was found in repo-backed research for this phase. [VERIFIED: backend-go/internal/registration/outlook_service.go][VERIFIED: backend-go/internal/registration/auto_upload_dispatcher.go][ASSUMED] |
| OS-registered state | None were verified from the repository itself, and this session did not audit launchd/systemd/task-scheduler registrations on the host. [VERIFIED: repository inspection][ASSUMED] | **Manual validation.** Add an ops check in the cutover plan to confirm there is no host-level service wrapper still forcing the Python runner or Python backend path. [ASSUMED] |
| Secrets / env vars | Go critical-path execution requires `DATABASE_URL` and `REDIS_ADDR`; Python bridge execution additionally requires a valid Python executable and repo root; native auth defaults to `https://chatgpt.com` and worker token-completion leases use Redis when configured. [VERIFIED: .planning/phases/01-compatibility-baseline/01-runtime-boundary-contract.md][VERIFIED: backend-go/cmd/worker/main.go][VERIFIED: backend-go/internal/registration/python_runner.go][VERIFIED: backend-go/internal/nativerunner/default_runner.go] | **Code edit + deployment validation.** No env-key rename is needed, but planners must treat PostgreSQL and Redis readiness as cutover prerequisites. [VERIFIED: backend-go/cmd/worker/main.go][VERIFIED: .planning/phases/01-compatibility-baseline/01-runtime-boundary-contract.md] |
| Build artifacts | The repo target for Go is `1.25.0`, but the local machine currently has `go1.24.3`; Python `3.11.9` is present; Docker is absent; local PostgreSQL and Redis ports are closed; `redis-cli` and `pg_isready` are absent. [VERIFIED: AGENTS.md stack block][VERIFIED: local commands `go version`, `python3 --version`, `nc -z localhost 5432`, `nc -z localhost 6379`, `command -v redis-cli`, `command -v pg_isready`, `command -v docker`] | **Environment bring-up required.** This is not a data migration, but it is a blocking execution prerequisite for end-to-end native-runner cutover verification. [VERIFIED: local environment audit] |

## Common Pitfalls

### Pitfall 1: Mistaking “worker already uses native runner” for “Phase 2 is already done”
**What goes wrong:** Planning skips contract-closure work because the worker path is already native by default. [VERIFIED: backend-go/cmd/worker/main.go]
**Why it happens:** The runtime core is native, but route/status/payload/task-control compatibility still drifts from Python. [VERIFIED: src/web/routes/registration.py][VERIFIED: backend-go/internal/registration/http/handlers.go]
**How to avoid:** Treat API payload parity, route coverage parity, `cancelling` semantics, and upload side-effect parity as first-class work items. [VERIFIED: .planning/REQUIREMENTS.md][VERIFIED: static/js/app.js]
**Warning signs:** Start or batch endpoints still return 202/minimal payloads, `/api/registration/tasks` list is still missing, or cancel moves directly to `cancelled`. [VERIFIED: backend-go/internal/registration/http/handlers.go][VERIFIED: backend-go/internal/registration/ws/task_socket_test.go]

### Pitfall 2: Breaking incremental log semantics while “optimizing” websockets or polling
**What goes wrong:** The UI drops logs, duplicates logs, or replays old logs after reconnect. [VERIFIED: static/js/app.js][VERIFIED: static/js/registration_log_buffer.js]
**Why it happens:** The frontend increments offsets on websocket log frames and relies on server-returned `log_next_offset` for hydration and polling fallback. [VERIFIED: static/js/app.js][VERIFIED: src/web/routes/registration.py][VERIFIED: backend-go/internal/registration/http/integration_test.go]
**How to avoid:** Preserve the current “snapshot first, live log frames later, monotonic `log_next_offset` always” contract for both task and batch channels. [VERIFIED: src/web/routes/websocket.py][VERIFIED: backend-go/internal/registration/ws/task_socket.go][VERIFIED: backend-go/internal/registration/ws/batch_socket.go]
**Warning signs:** `log_next_offset` decreases, websocket snapshot includes history already consumed by hydration, or log dedupe in `registration_log_buffer.js` starts masking missing server events. [VERIFIED: static/js/registration_log_buffer.js][VERIFIED: src/web/task_manager.py]

### Pitfall 3: Recreating Python’s process-local runtime state inside Go
**What goes wrong:** Multiple workers disagree on task or batch state, and resume/cancel behavior stops being durable. [VERIFIED: src/web/task_manager.py][VERIFIED: backend-go/internal/registration/batch_service.go]
**Why it happens:** Python uses `task_manager` and `batch_tasks` because it is process-local; Go already has a jobs-backed model and should not regress to in-memory control state. [VERIFIED: src/web/task_manager.py][VERIFIED: backend-go/internal/registration/executor.go][VERIFIED: backend-go/internal/registration/batch_service.go]
**How to avoid:** Keep job state durable and use batch aggregation plus websocket polling as the projection layer. [VERIFIED: backend-go/internal/registration/batch_service.go][VERIFIED: backend-go/internal/registration/ws/batch_socket.go]
**Warning signs:** A restart loses batch status, pause state exists only in memory, or different workers produce different websocket snapshots. [VERIFIED: backend-go/internal/registration/batch_service.go][ASSUMED]

### Pitfall 4: Forgetting proxy and Outlook reservation parity
**What goes wrong:** Concurrent Outlook tasks can contend for the same mailbox, or proxy usage stops matching current Python behavior. [VERIFIED: src/web/routes/registration.py][VERIFIED: backend-go/internal/registration/orchestrator.go]
**Why it happens:** Python resolves a proxy when absent and reserves Outlook services across in-flight tasks, but the worker currently does not wire `ProxySelector` or `OutlookReservationStore`. [VERIFIED: src/web/routes/registration.py][VERIFIED: backend-go/cmd/worker/main.go][VERIFIED: backend-go/internal/registration/orchestrator.go]
**How to avoid:** Add explicit planning work for proxy-selection parity and Outlook reservation persistence before high-concurrency native cutover. [VERIFIED: backend-go/internal/registration/orchestrator_test.go]
**Warning signs:** Repeated mailbox credentials across concurrent tasks or empty `proxy_used` on Go-persisted accounts when Python would have assigned a proxy. [VERIFIED: src/web/routes/registration.py][VERIFIED: backend-go/internal/accounts/types.go][ASSUMED]

### Pitfall 5: Accidentally changing upload writeback semantics
**What goes wrong:** Operator-visible account flags drift, or TM starts mutating account state even though it only logged before. [VERIFIED: src/web/routes/registration.py][VERIFIED: backend-go/internal/registration/auto_upload_dispatcher.go]
**Why it happens:** CPA, Sub2API, and TM look symmetric at the route layer, but Python only persists success markers for CPA and Sub2API. [VERIFIED: src/web/routes/registration.py][VERIFIED: backend-go/internal/registration/python_runner_script.go]
**How to avoid:** Preserve the current per-kind behavior and cover it with regression tests around executor writeback. [VERIFIED: backend-go/internal/registration/executor_persistence_test.go][VERIFIED: backend-go/internal/registration/auto_upload_dispatcher_test.go]
**Warning signs:** `tm` uploads start writing flags in `accounts`, or CPA/Sub2API uploads stop updating timestamps after success. [VERIFIED: backend-go/internal/accounts/types.go][VERIFIED: backend-go/internal/registration/auto_upload_dispatcher.go]

## Code Examples

Verified patterns from the current codebase:

### Executor Hook Order
```go
// Source: backend-go/internal/registration/executor.go
prepared, err := e.preparer.Prepare(ctx, job.JobID, req)
output, err := e.runner.Run(ctx, RunnerRequest{
    TaskUUID:             job.JobID,
    StartRequest:         prepared.Request,
    Plan:                 prepared.Plan,
    GoPersistenceEnabled: e.accountPersistence != nil,
    control:              e.runnerControl(job.JobID),
}, logf)
```

### Python Log Window Contract
```python
# Source: src/web/routes/registration.py
def _resolve_log_window(logs, *, offset=0, base_index=0):
    safe_offset = max(int(offset or 0), int(base_index or 0))
    window_end = base_index + len(logs)
    if safe_offset > window_end:
        safe_offset = window_end
    slice_start = max(0, safe_offset - base_index)
    return logs[slice_start:], safe_offset, window_end
```

### Go WebSocket Snapshot Before Polling
```go
// Source: backend-go/internal/registration/ws/task_socket.go
if err := h.sendSnapshot(ctx, socket, state, taskUUID); err != nil {
    return
}
go func() { errs <- h.readLoop(ctx, socket, state, taskUUID) }()
go func() { errs <- h.pollLoop(ctx, socket, state, taskUUID) }()
```

## Verification Strategy

1. **Freeze Python observable fixtures before changing Go behavior.** Capture HTTP status codes, response bodies, websocket frame sequences, and log-offset transitions for single registration, batch registration, Outlook batch, task pause/resume/cancel, and batch pause/resume/cancel. [VERIFIED: src/web/routes/registration.py][VERIFIED: src/web/routes/websocket.py][VERIFIED: static/js/app.js]
2. **Treat existing Go compatibility tests as the starting baseline, not the finish line.** Extend `http/handlers_test.go`, `http/integration_test.go`, `ws/task_socket_test.go`, `ws/batch_socket_test.go`, `executor_persistence_test.go`, and `prepare_signup_flow_test.go` to encode every remaining Python-vs-Go delta the plan chooses to close. [VERIFIED: backend-go/internal/registration/http/handlers_test.go][VERIFIED: backend-go/internal/registration/http/integration_test.go][VERIFIED: backend-go/internal/registration/ws/task_socket_test.go][VERIFIED: backend-go/internal/registration/ws/batch_socket_test.go][VERIFIED: backend-go/internal/registration/executor_persistence_test.go][VERIFIED: backend-go/internal/nativerunner/prepare_signup_flow_test.go]
3. **Add route-level parity checks for what Go still lacks.** The plan should explicitly add tests for `/api/registration/tasks` list, task delete, Python-style start/batch HTTP status codes, and Python-style cancel transitions. [VERIFIED: src/web/routes/registration.py][VERIFIED: backend-go/internal/registration/http/handlers.go]
4. **Add worker-level parity checks for provider preparation.** Verify proxy selection fallback, Outlook reservation, and prepared config normalization across all supported provider types because the current worker wiring is incomplete there. [VERIFIED: backend-go/cmd/worker/main.go][VERIFIED: backend-go/internal/registration/orchestrator.go][VERIFIED: backend-go/internal/registration/orchestrator_test.go]
5. **Add persistence-and-side-effect proofs before bridge removal.** For each outcome path, verify the exact persisted account fields, `extra_data` merge behavior, `status/source`, CPA/Sub2API writebacks, and TM log-only behavior. [VERIFIED: src/core/register.py][VERIFIED: backend-go/internal/registration/executor_persistence_test.go][VERIFIED: backend-go/internal/registration/auto_upload_dispatcher_test.go][VERIFIED: backend-go/internal/accounts/service_test.go]
6. **Run an environment-gated end-to-end cutover check with PostgreSQL and Redis.** The bridge can only leave the critical path after a worker/API run against real Go dependencies proves registration, logs, pause/resume/cancel, and uploads without Python. [VERIFIED: .planning/phases/01-compatibility-baseline/01-runtime-boundary-contract.md][VERIFIED: backend-go/cmd/worker/main.go][VERIFIED: local environment audit]

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|---|---|---|---|
| Python thread-pool execution + `task_manager` + `batch_tasks` drive runtime state. [VERIFIED: src/web/routes/registration.py][VERIFIED: src/web/task_manager.py] | Go worker uses `jobs.Service` + `registration.Executor` + native runner as the default execution core. [VERIFIED: backend-go/cmd/worker/main.go][VERIFIED: backend-go/internal/registration/executor.go] | Repo current as of 2026-04-05. [VERIFIED: current session date][VERIFIED: backend-go/cmd/worker/main.go] | Phase 2 should close compatibility gaps around this Go baseline instead of rebuilding runtime ownership. [VERIFIED: .planning/phases/02-native-registration-runtime/02-CONTEXT.md] |
| Python runner bridge performed persistence and optional uploads when Go persistence was disabled. [VERIFIED: backend-go/internal/registration/python_runner_script.go] | Native runner returns `AccountPersistence`, and the executor performs Go persistence plus Go-side auto-upload dispatch. [VERIFIED: backend-go/internal/registration/executor.go][VERIFIED: backend-go/internal/registration/native_runner_boundary_test.go] | Repo current as of 2026-04-05. [VERIFIED: current session date] | This is already the intended final shape; the remaining work is proving it matches Python semantics well enough to remove the bridge from normal execution. [VERIFIED: .planning/phases/02-native-registration-runtime/02-CONTEXT.md] |
| Python `save_to_database()` writes account rows directly from `RegistrationEngine`. [VERIFIED: src/core/register.py] | Go `accounts.Service.UpsertAccount()` merges writes and protects stronger existing account state. [VERIFIED: backend-go/internal/accounts/service.go] | Repo current as of 2026-04-05. [VERIFIED: current session date] | Planners should verify merge behavior against current Python expectations instead of bypassing the account service. [VERIFIED: backend-go/internal/accounts/service_test.go][VERIFIED: src/core/register.py] |

**Deprecated/outdated:**
- Using the Python runner bridge as the normal registration critical path is explicitly out of bounds for completed Phase 2. [VERIFIED: .planning/phases/02-native-registration-runtime/02-CONTEXT.md]
- Treating Python’s process-local runtime store as the target design for Go is outdated because the Go baseline already moved to durable jobs plus websocket projection. [VERIFIED: src/web/task_manager.py][VERIFIED: backend-go/internal/registration/batch_service.go]

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|---|---|---|
| A1 | No host-level OS service registrations still force the Python registration path, because this session only inspected the repository and local ports, not launchd/systemd/task-scheduler state. [ASSUMED] | Runtime State Inventory | A cutover plan could miss an operational dependency and appear correct in code review while still starting Python in production. |
| A2 | No non-repo external configuration beyond the database-backed email/upload settings is required for registration runtime behavior in this phase. [ASSUMED] | Runtime State Inventory | The plan could omit a manual external-config verification step and fail late in staging or production. |

## Open Questions (RESOLVED)

1. **Should Phase 2 fully align Go HTTP status codes and response bodies with Python for `/start`, `/batch`, and `/outlook-batch`?**
   - What we know: Python currently returns HTTP 200 with richer response models, while Go currently returns HTTP 202 with accepted-style payloads. [VERIFIED: src/web/routes/registration.py][VERIFIED: backend-go/internal/registration/http/handlers.go]
   - RESOLVED: Phase 2 must take the conservative option and make Go match the current Python HTTP contract for `/start`, `/batch`, and `/outlook-batch` unless a future milestone explicitly approves drift. [VERIFIED: .planning/phases/02-native-registration-runtime/02-CONTEXT.md][VERIFIED: .planning/ROADMAP.md]

2. **Which Python auth-healing branches must have fixture coverage before the bridge can leave the path?**
   - What we know: Python still contains `native` versus `abcard` entry-flow branches plus add-phone/workspace/session fallback logic, and Go native runner already has tests for straight signup, existing-account token completion, and interactive-step boundaries. [VERIFIED: src/core/register.py][VERIFIED: backend-go/internal/nativerunner/prepare_signup_flow_test.go][VERIFIED: backend-go/internal/nativerunner/token_completion_passwordless_test.go]
   - RESOLVED: Phase 2 must explicitly include fixture coverage for at least one `add-phone` or interactive-boundary path before claiming the Python bridge is off the normal registration critical path. Happy-path signup, passwordless existing-account completion, historical-password continuation, and one add-phone/interactive-boundary path are the minimum critical-path set. [VERIFIED: src/core/register.py][VERIFIED: backend-go/internal/nativerunner/prepare_signup_flow_test.go][VERIFIED: backend-go/internal/nativerunner/auth/post_signup_test.go]

3. **How strict should task-control parity be around intermediate `cancelling` states?**
   - What we know: Python shows `cancelling` immediately for task and batch cancels, while Go task control currently lands on `cancelled` faster and Go batch HTTP control already returns `cancelled` even though websocket still shows `cancelling` first. [VERIFIED: src/web/routes/registration.py][VERIFIED: src/web/routes/websocket.py][VERIFIED: backend-go/internal/registration/http/handlers.go][VERIFIED: backend-go/internal/registration/ws/task_socket_test.go][VERIFIED: backend-go/internal/registration/ws/batch_socket.go]
   - RESOLVED: Phase 2 must preserve the intermediate `cancelling` state consistently across HTTP, polling, and websocket surfaces unless a future explicit decision approves drift. [VERIFIED: .planning/phases/02-native-registration-runtime/02-CONTEXT.md][VERIFIED: .planning/ROADMAP.md]

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|---|---|---|---|---|
| Go toolchain | Build/test `backend-go` and run native worker. [VERIFIED: backend-go/go.mod via AGENTS stack block][VERIFIED: backend-go/cmd/worker/main.go] | `✓` but below repo target. [VERIFIED: local command `go version`] | `go1.24.3`; repo target is `1.25.0`. [VERIFIED: local command `go version`][VERIFIED: AGENTS.md stack block] | Upgrade local Go to `1.25.0` before end-to-end verification. [VERIFIED: AGENTS.md stack block] |
| Python 3 | Python oracle runs and bridge-based comparison tests. [VERIFIED: src/core/register.py][VERIFIED: backend-go/internal/registration/python_runner.go] | `✓`. [VERIFIED: local command `python3 --version`] | `Python 3.11.9`. [VERIFIED: local command `python3 --version`] | None needed for oracle/bridge use. [VERIFIED: local environment audit] |
| PostgreSQL service | Go accounts/jobs/uploader repositories and worker critical path. [VERIFIED: .planning/phases/01-compatibility-baseline/01-runtime-boundary-contract.md][VERIFIED: backend-go/cmd/worker/main.go] | `✗` on default local port in this session. [VERIFIED: local command `nc -z localhost 5432`] | Port `5432` closed; `pg_isready` unavailable. [VERIFIED: local command `nc -z localhost 5432`][VERIFIED: local command `command -v pg_isready`] | No full-runtime fallback; use mocks/fakes only for unit tests. [VERIFIED: backend-go/internal/registration/http/handlers_test.go][VERIFIED: backend-go/internal/registration/executor_test.go] |
| Redis service | Asynq queue and token-completion lease store. [VERIFIED: .planning/phases/01-compatibility-baseline/01-runtime-boundary-contract.md][VERIFIED: backend-go/cmd/worker/main.go] | `✗` on default local port in this session. [VERIFIED: local command `nc -z localhost 6379`] | Port `6379` closed; `redis-cli` unavailable. [VERIFIED: local command `nc -z localhost 6379`][VERIFIED: local command `command -v redis-cli`] | No full-runtime fallback for cutover proof; in-memory fakes only cover unit scope. [VERIFIED: backend-go/internal/nativerunner/token_completion_test.go][VERIFIED: backend-go/internal/registration/http/handlers_test.go] |
| Docker | Possible local service bootstrap shortcut. [VERIFIED: local command `command -v docker`] | `✗`. [VERIFIED: local command `command -v docker`] | `—`. [VERIFIED: local command `command -v docker`] | Start PostgreSQL and Redis manually outside Docker if needed. [ASSUMED] |

**Missing dependencies with no fallback:**
- PostgreSQL and Redis are both required for end-to-end native-runner cutover verification and are not locally available in this session. [VERIFIED: .planning/phases/01-compatibility-baseline/01-runtime-boundary-contract.md][VERIFIED: local environment audit]

**Missing dependencies with fallback:**
- Docker is missing, but manual PostgreSQL/Redis bring-up remains possible. [VERIFIED: local environment audit][ASSUMED]
- Local Go is below the repo target; unit-level analysis can continue, but cutover verification should wait for `1.25.0`. [VERIFIED: local command `go version`][VERIFIED: AGENTS.md stack block]

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---|---|---|
| V2 Authentication | yes. [VERIFIED: src/core/register.py][VERIFIED: backend-go/internal/nativerunner/auth/client.go] | Preserve current auth-boundary behavior while migrating the runtime, because Phase 1 froze `/api` and `/api/ws` semantics as compatibility scope. [VERIFIED: .planning/phases/01-compatibility-baseline/01-runtime-boundary-contract.md] |
| V3 Session Management | yes. [VERIFIED: src/core/register.py][VERIFIED: backend-go/internal/nativerunner/prepare_signup_flow.go] | Keep session-token, cookie, device-id, and refresh-token handling inside the existing registration/native-runner modules and account persistence seam. [VERIFIED: src/core/register.py][VERIFIED: backend-go/internal/nativerunner/prepare_signup_flow.go][VERIFIED: backend-go/internal/accounts/service.go] |
| V4 Access Control | yes. [VERIFIED: .planning/phases/01-compatibility-baseline/01-runtime-boundary-contract.md] | Do not widen scope into auth hardening during Phase 2; preserve the current API access boundary unless the user explicitly changes it later. [VERIFIED: .planning/phases/01-compatibility-baseline/01-runtime-boundary-contract.md] |
| V5 Input Validation | yes. [VERIFIED: src/web/routes/registration.py][VERIFIED: backend-go/internal/registration/http/handlers.go][VERIFIED: backend-go/internal/registration/batch_service.go] | Keep explicit request validation on count/interval/concurrency/mode/service IDs and reject invalid offsets. [VERIFIED: src/web/routes/registration.py][VERIFIED: backend-go/internal/registration/http/handlers.go][VERIFIED: backend-go/internal/registration/batch_service.go] |
| V6 Cryptography | yes. [VERIFIED: src/core/register.py][VERIFIED: backend-go/internal/nativerunner/auth/login_password.go] | Use the existing auth/token completion code paths and never add ad hoc token/cookie/PKCE logic outside the current modules. [VERIFIED: src/core/register.py][VERIFIED: backend-go/internal/nativerunner/auth/login_password.go][VERIFIED: backend-go/internal/nativerunner/token_completion.go] |

### Known Threat Patterns for This Stack

| Pattern | STRIDE | Standard Mitigation |
|---|---|---|
| Duplicate or skipped task logs during reconnect. [VERIFIED: static/js/app.js][VERIFIED: static/js/registration_log_buffer.js] | Tampering / Repudiation | Preserve monotonic `log_next_offset`, send snapshot before live frames, and keep websocket plus polling behavior aligned. [VERIFIED: src/web/routes/registration.py][VERIFIED: backend-go/internal/registration/ws/task_socket.go][VERIFIED: backend-go/internal/registration/ws/batch_socket.go] |
| Concurrent token completion for the same email. [VERIFIED: backend-go/internal/nativerunner/token_completion.go][VERIFIED: backend-go/internal/nativerunner/token_completion_test.go] | Tampering / DoS | Keep `TokenCompletionCoordinator` scheduling, cooldown, and Redis lease ownership in the worker path. [VERIFIED: backend-go/internal/nativerunner/token_completion.go][VERIFIED: backend-go/cmd/worker/main.go] |
| Reusing the same Outlook mailbox in concurrent tasks. [VERIFIED: src/web/routes/registration.py][VERIFIED: backend-go/internal/registration/orchestrator.go] | Tampering | Add persistent Outlook reservation handling in Go instead of relying on Python’s process-local claim lock. [VERIFIED: src/web/routes/registration.py][VERIFIED: backend-go/internal/registration/orchestrator.go] |
| Leaking tokens or session artifacts through logs or frontend-visible payloads. [VERIFIED: src/core/register.py][VERIFIED: backend-go/internal/nativerunner/prepare_signup_flow.go] | Information Disclosure | Keep operator logs free of raw secrets and expose only the fields current clients already consume. [VERIFIED: src/core/register.py][VERIFIED: backend-go/internal/registration/http/handlers.go][ASSUMED] |

## Sources

### Primary (HIGH confidence)
- `.planning/phases/02-native-registration-runtime/02-CONTEXT.md` - phase scope, locked decisions, and canonical references. [VERIFIED: .planning/phases/02-native-registration-runtime/02-CONTEXT.md]
- `.planning/REQUIREMENTS.md` - requirement IDs `RUN-01`, `RUN-02`, `RUN-03`, and `COMP-03`. [VERIFIED: .planning/REQUIREMENTS.md]
- `.planning/phases/01-compatibility-baseline/01-route-client-parity-matrix.md` - frozen route ownership and compatibility consumers. [VERIFIED: .planning/phases/01-compatibility-baseline/01-route-client-parity-matrix.md]
- `.planning/phases/01-compatibility-baseline/01-runtime-boundary-contract.md` - cutover rules and task/websocket compatibility contract. [VERIFIED: .planning/phases/01-compatibility-baseline/01-runtime-boundary-contract.md]
- `src/web/routes/registration.py`, `src/web/routes/websocket.py`, `src/web/task_manager.py`, `src/core/register.py` - Python oracle behavior. [VERIFIED: src/web/routes/registration.py][VERIFIED: src/web/routes/websocket.py][VERIFIED: src/web/task_manager.py][VERIFIED: src/core/register.py]
- `backend-go/internal/registration/*`, `backend-go/internal/registration/ws/*`, `backend-go/internal/nativerunner/*`, `backend-go/cmd/worker/main.go` - current Go baseline and worker wiring. [VERIFIED: backend-go/internal/registration/http/handlers.go][VERIFIED: backend-go/internal/registration/executor.go][VERIFIED: backend-go/internal/registration/batch_service.go][VERIFIED: backend-go/internal/registration/outlook_service.go][VERIFIED: backend-go/internal/registration/ws/task_socket.go][VERIFIED: backend-go/internal/registration/ws/batch_socket.go][VERIFIED: backend-go/internal/nativerunner/prepare_signup_flow.go][VERIFIED: backend-go/cmd/worker/main.go]
- `backend-go/internal/accounts/*`, `backend-go/internal/uploader/*` - persistence and side-effect writeback boundaries. [VERIFIED: backend-go/internal/accounts/service.go][VERIFIED: backend-go/internal/accounts/types.go][VERIFIED: backend-go/internal/uploader/sender.go][VERIFIED: backend-go/internal/uploader/builder.go]
- `backend-go/internal/registration/*_test.go`, `backend-go/internal/nativerunner/*_test.go`, `tests/test_registration_routes.py`, `tests/test_registration_engine.py`, `tests/frontend/registration_log_buffer.test.mjs` - current automated evidence and remaining gaps. [VERIFIED: backend-go/internal/registration/http/handlers_test.go][VERIFIED: backend-go/internal/registration/http/integration_test.go][VERIFIED: backend-go/internal/registration/ws/task_socket_test.go][VERIFIED: backend-go/internal/registration/ws/batch_socket_test.go][VERIFIED: backend-go/internal/nativerunner/prepare_signup_flow_test.go][VERIFIED: tests/test_registration_routes.py][VERIFIED: tests/test_registration_engine.py][VERIFIED: tests/frontend/registration_log_buffer.test.mjs]

### Secondary (MEDIUM confidence)
- None; this research was intentionally codebase-scoped. [VERIFIED: current session tool usage]

### Tertiary (LOW confidence)
- None. [VERIFIED: current session tool usage]

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - the recommended stack is the existing repo-local Go runtime already wired in code and tests. [VERIFIED: backend-go/cmd/worker/main.go][VERIFIED: backend-go/internal/registration/http/handlers_test.go]
- Architecture: MEDIUM - the core seams are clear, but proxy-selection and Outlook-reservation parity still need explicit planning work. [VERIFIED: backend-go/cmd/worker/main.go][VERIFIED: backend-go/internal/registration/orchestrator.go]
- Pitfalls: MEDIUM - the visible API/status/log drifts are directly verifiable, but some production-only operational dependencies remain assumptions until staging validates them. [VERIFIED: backend-go/internal/registration/http/handlers.go][ASSUMED]

**Research date:** 2026-04-05. [VERIFIED: current session date]
**Valid until:** 2026-05-05 for codebase-scoped planning, or earlier if registration/runtime code changes materially. [VERIFIED: current session date][ASSUMED]
