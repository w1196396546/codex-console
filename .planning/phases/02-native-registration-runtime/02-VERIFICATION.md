---
phase: 02-native-registration-runtime
verified: 2026-04-05T11:02:05Z
status: human_needed
score: 3/3 must-haves verified
human_verification:
  - test: "真实 native 注册联调（single/batch/outlook batch）"
    expected: "通过 Go worker 完成注册主路径，任务完成后无需 Python runner bridge，且结果/状态与现有客户端契约一致"
    why_human: "需要真实 OpenAI/Auth、邮箱 provider、PostgreSQL/Redis 与运行中的 worker 联调；当前自动化以 stub/in-memory/e2e 兼容测试为主"
  - test: "真实任务控制与 websocket 观察"
    expected: "对运行中任务执行 pause/resume/cancel 时，现有前端或 WS client 能看到兼容的 status、cancelling、中间日志游标与 heartbeat 行为"
    why_human: "真实并发、网络抖动、长任务时序和 reconnect 体验无法仅靠当前仓库内单元/e2e stub 充分覆盖"
  - test: "真实 CPA/Sub2API/TM 自动上传副作用"
    expected: "注册成功后仅 CPA/Sub2API 写回成功标记和时间，TM 仍保持仅日志副作用"
    why_human: "需要命中真实或 staging 上传端点验证外部响应与写回顺序；当前自动化主要验证 Go 侧调度与写回逻辑"
---

# Phase 2: Native Registration Runtime Verification Report

**Phase Goal:** Complete Go ownership of registration execution, task lifecycle, and upload side effects so Python is no longer required on the registration critical path.
**Verified:** 2026-04-05T11:02:05Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | Registration start, batch, and Outlook batch flows complete through Go-owned execution without requiring the Python runner bridge on the critical path. | ✓ VERIFIED | `backend-go/cmd/worker/main.go` wires `newWorkerRegistrationRunner()` into `registration.NewNativeRunner(...)` and injects preparation/account/uploader dependencies; no worker bootstrap path constructs `PythonRunner` ([`backend-go/cmd/worker/main.go:41`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/cmd/worker/main.go#L41), [`backend-go/cmd/worker/main.go:112`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/cmd/worker/main.go#L112), [`backend-go/cmd/worker/main.go:139`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/cmd/worker/main.go#L139)). Native provider coverage and signup/token-completion branches exist in `nativerunner` ([`backend-go/internal/nativerunner/mail/provider.go:10`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/nativerunner/mail/provider.go#L10), [`backend-go/internal/nativerunner/prepare_signup_flow.go:168`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/nativerunner/prepare_signup_flow.go#L168), [`backend-go/internal/nativerunner/prepare_signup_flow.go:287`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/nativerunner/prepare_signup_flow.go#L287)). Current workspace evidence includes the passed worker/registration tests supplied by the user. |
| 2 | Existing clients can inspect, pause, resume, cancel, and observe task and batch progress through Go with compatible status and log behavior. | ✓ VERIFIED | HTTP registration routes expose start/batch/outlook-batch, task list/detail/log/delete, and task/batch controls with Python-compatible 200 responses and log-offset handling ([`backend-go/internal/registration/http/handlers.go:78`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/http/handlers.go#L78), [`backend-go/internal/registration/http/handlers.go:162`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/http/handlers.go#L162), [`backend-go/internal/registration/http/handlers.go:261`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/http/handlers.go#L261)). `BatchService` projects `cancelling`, `current_index`, `log_offset`, and `log_next_offset` from jobs/logs ([`backend-go/internal/registration/batch_service.go:233`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/batch_service.go#L233), [`backend-go/internal/registration/batch_service.go:313`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/batch_service.go#L313), [`backend-go/internal/registration/batch_service.go:352`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/batch_service.go#L352)). WS handlers preserve snapshot-first, ping/pong, monotonic log cursors, and cancelling projection ([`backend-go/internal/registration/ws/task_socket.go:67`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/ws/task_socket.go#L67), [`backend-go/internal/registration/ws/task_socket.go:184`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/ws/task_socket.go#L184), [`backend-go/internal/registration/ws/batch_socket.go:53`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/ws/batch_socket.go#L53), [`backend-go/internal/registration/ws/batch_socket.go:138`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/ws/batch_socket.go#L138)). E2E tests exercise compatibility flows end to end ([`backend-go/tests/e2e/jobs_flow_test.go:61`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/tests/e2e/jobs_flow_test.go#L61), [`backend-go/tests/e2e/jobs_flow_test.go:160`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/tests/e2e/jobs_flow_test.go#L160), [`backend-go/tests/e2e/jobs_flow_test.go:226`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/tests/e2e/jobs_flow_test.go#L226), [`backend-go/tests/e2e/jobs_flow_test.go:322`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/tests/e2e/jobs_flow_test.go#L322), [`backend-go/tests/e2e/jobs_flow_test.go:491`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/tests/e2e/jobs_flow_test.go#L491)). |
| 3 | Account persistence and CPA/Sub2API/TM upload side effects remain compatible with current workflow semantics. | ✓ VERIFIED | `Executor.Execute()` performs `prepare -> runner -> account upsert -> auto upload -> writeback` and persists typed runner errors with account payloads ([`backend-go/internal/registration/executor.go:178`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/executor.go#L178), [`backend-go/internal/registration/executor.go:190`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/executor.go#L190), [`backend-go/internal/registration/executor.go:204`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/executor.go#L204)). `accounts.Service` preserves stronger existing state while merging runtime updates/writebacks ([`backend-go/internal/accounts/service.go:47`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/accounts/service.go#L47), [`backend-go/internal/accounts/service.go:77`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/accounts/service.go#L77), [`backend-go/internal/accounts/service.go:147`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/accounts/service.go#L147)). `AutoUploadDispatcher` only writes success markers for CPA/Sub2API, not TM ([`backend-go/internal/registration/auto_upload_dispatcher.go:58`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/auto_upload_dispatcher.go#L58), [`backend-go/internal/registration/auto_upload_dispatcher.go:187`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/auto_upload_dispatcher.go#L187), [`backend-go/internal/registration/auto_upload_dispatcher.go:205`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/auto_upload_dispatcher.go#L205)). Persistence/writeback tests cover field semantics, partial failures, and upload ordering ([`backend-go/internal/registration/executor_persistence_test.go:14`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/executor_persistence_test.go#L14), [`backend-go/internal/registration/executor_persistence_test.go:112`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/executor_persistence_test.go#L112), [`backend-go/internal/registration/executor_persistence_test.go:187`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/executor_persistence_test.go#L187), [`backend-go/internal/registration/executor_persistence_test.go:326`](/Users/weihaiqiu/IdeaProjects/codex-console/backend-go/internal/registration/executor_persistence_test.go#L326)). |

**Score:** 3/3 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `backend-go/cmd/worker/main.go` | Worker bootstrap wires native registration runner and dependencies | ✓ VERIFIED | Native runner, preparation deps, account persistence, and auto-upload dispatcher are all injected. |
| `backend-go/internal/registration/orchestrator.go` | Preparation, proxy selection, Outlook reservation, and execution plan building | ✓ VERIFIED | Resolves settings/catalog/outlook data and builds native execution plans with proxy/outlook metadata. |
| `backend-go/internal/nativerunner/prepare_signup_flow.go` | Native signup + token-completion critical path | ✓ VERIFIED | Covers OTP signup, existing-account token completion dispatch, workspace/account resolution, and persistence payload creation. |
| `backend-go/internal/registration/http/handlers.go` | Python-compatible registration HTTP surface | ✓ VERIFIED | Exposes start/batch/outlook, task list/detail/log/delete, and control endpoints. |
| `backend-go/internal/registration/batch_service.go` | Durable batch state projection and cancelling/log cursors | ✓ VERIFIED | Aggregates jobs/logs into batch-compatible counters, status, and cursor fields. |
| `backend-go/internal/registration/executor.go` | Ordered prepare/run/persist/upload chain | ✓ VERIFIED | Uses typed runner output, persists on typed errors, and dispatches uploads only after successful upsert. |
| `backend-go/internal/registration/auto_upload_dispatcher.go` | Upload dispatch and writeback asymmetry | ✓ VERIFIED | Reads configured services, sends uploads, and only marks CPA/Sub2API success. |
| `backend-go/internal/accounts/service.go` | Account merge semantics | ✓ VERIFIED | Preserves stronger state and merges runtime/writeback metadata. |
| `backend-go/internal/registration/ws/task_socket.go` | Task websocket semantics | ✓ VERIFIED | Snapshot-first, ping/pong, cancelling projection, monotonic log offsets. |
| `backend-go/internal/registration/ws/batch_socket.go` | Batch websocket semantics | ✓ VERIFIED | Snapshot-first, cancelling projection, progress metadata, monotonic log offsets. |
| `backend-go/tests/e2e/jobs_flow_test.go` | End-to-end compatibility assertions | ✓ VERIFIED | Exercises registration/task/batch/outlook HTTP and WS flows through router + jobs worker. |

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| `backend-go/cmd/worker/main.go` | `backend-go/internal/registration/orchestrator.go` | `newRegistrationPreparationDependencies + registration.WithPreparationDependencies` | ✓ WIRED | gsd-tools key-link verification passed; explicit code path in worker bootstrap. |
| `backend-go/internal/registration/orchestrator.go` | `backend-go/internal/nativerunner/prepare_signup_flow.go` | `RunnerRequest.Plan email_service/proxy/outlook preparation` | ✓ WIRED | Execution plan carries prepared email/proxy/outlook data into native runner path. |
| `backend-go/internal/registration/http/handlers.go` | `backend-go/internal/registration/batch_service.go` | `StartBatch/GetBatch/PauseBatch/ResumeBatch/CancelBatch` | ✓ WIRED | Handler delegates batch lifecycle to shared batch service. |
| `backend-go/internal/registration/native_runner.go` | `backend-go/internal/registration/executor.go` | `RunnerOutput.AccountPersistence` | ✓ WIRED | Native runner adapter returns typed `RunnerOutput` consumed by executor. |
| `backend-go/internal/registration/executor.go` | `backend-go/internal/accounts/service.go` | `WithAccountPersistence + UpsertAccount` | ✓ WIRED | Executor persists native output and typed runner errors through account service. |
| `backend-go/internal/registration/executor.go` | `backend-go/internal/registration/auto_upload_dispatcher.go` | `AutoUploadDispatchRequest after persisted account save` | ✓ WIRED | Auto-upload dispatch uses persisted account returned from the first upsert. |
| `backend-go/internal/registration/ws/task_socket.go` | `backend-go/tests/e2e/jobs_flow_test.go` | `task websocket frame shape and control semantics` | ✓ WIRED | Task websocket semantics are asserted in unit + e2e coverage. |
| `backend-go/internal/registration/ws/batch_socket.go` | `backend-go/tests/e2e/jobs_flow_test.go` | `batch websocket frame shape and control semantics` | ✓ WIRED | Batch/outlook websocket semantics are asserted in unit + e2e coverage. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| --- | --- | --- | --- | --- |
| `backend-go/internal/registration/http/handlers.go` | `TaskResponse`, `logs`, `BatchStatusResponse` | `jobs.Service` + `BatchService` | Yes | ✓ FLOWING |
| `backend-go/internal/registration/ws/task_socket.go` | `job.Status`, `[]JobLog`, `taskMetadata` | `jobs.Service.GetJob/ListJobLogs` | Yes | ✓ FLOWING |
| `backend-go/internal/registration/ws/batch_socket.go` | `BatchStatusResponse`, `Logs` | `BatchService.GetBatch` | Yes | ✓ FLOWING |
| `backend-go/internal/registration/executor.go` | `RunnerOutput.AccountPersistence` | Native runner output, then `accounts.Service.UpsertAccount` | Yes | ✓ FLOWING |
| `backend-go/internal/registration/auto_upload_dispatcher.go` | `ServiceConfig`, `UploadResult`, `AccountUpdate` | `uploader.PostgresConfigRepository` + sender results | Yes | ✓ FLOWING |

### Behavioral Spot-Checks

These were provided by the user as already passed in the current workspace and are consistent with the inspected code paths.

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| Worker/native preparation dependencies | `cd backend-go && go test ./cmd/worker ./internal/registration -run 'TestNewRegistrationPreparationDependenciesUsesPostgresRepositories\|TestOrchestrator.*\|TestExecutor.*Preparation.*' -v` | Passed in current workspace | ✓ PASS |
| HTTP/polling compatibility | `cd backend-go && go test ./internal/registration/http ./internal/registration ./internal/jobs ./tests/e2e -run 'TestRegistrationCompatibleEndpoints\|TestBatchEndpoints\|TestOutlookBatchEndpoints\|TestRegistrationStartAndTaskReadback\|TestRegistrationCompatibilityFlow\|TestRegistrationBatchCompatibilityFlow\|TestRegistrationOutlookBatchCompatibility' -v` | Passed in current workspace | ✓ PASS |
| Persistence and typed-error semantics | `cd backend-go && go test ./internal/registration ./internal/accounts -run 'TestExecutor.*Persist.*\|TestExecutor.*TypedError.*\|TestExecutor.*AutoUpload.*\|Test.*NativeRunner.*Boundary.*\|Test.*PythonRunner.*Persistence.*\|TestService.*Upsert.*\|Test.*Repository.*Account.*' -v` | Passed in current workspace | ✓ PASS |
| Auto-upload sequencing and writeback asymmetry | `cd backend-go && go test ./internal/registration ./internal/accounts -run 'TestAutoUploadDispatcher.*\|TestExecutor.*AutoUpload.*\|TestService.*Upsert.*' -v` | Passed in current workspace | ✓ PASS |
| Websocket compatibility | `cd backend-go && go test ./internal/registration/ws ./tests/e2e -run 'TestTaskSocketSendsCurrentStatusAndLogs\|TestTaskSocketCompletedStatusIncludesEmailFromJobResult\|TestRegistrationWebSocketCompatibility\|TestBatchSocketSendsCurrentStatusAndLogs\|TestRegistrationBatchWebSocketCompatibility\|TestRegistrationOutlookBatchCompatibility' -v` | Passed in current workspace | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| --- | --- | --- | --- | --- |
| `RUN-01` | `02-01`, `02-03` | Registration start, batch, and Outlook batch flows execute on Go-owned runtime logic without requiring the Python runner bridge on the critical path. | ✓ SATISFIED | Worker bootstrap uses `NewNativeRunner`; orchestrator/native flow/provider coverage exists; worker/registration preparation tests passed. |
| `RUN-02` | `02-02`, `02-04` | Existing clients can list, inspect, pause, resume, cancel, and clean up registration tasks and batches through Go with current behavior. | ✓ SATISFIED | HTTP task list/detail/log/delete + batch control endpoints are wired; e2e and websocket compatibility tests passed. |
| `RUN-03` | `02-03` | Registration side effects continue to persist accounts and trigger CPA, Sub2API, and TM uploads with current semantics. | ✓ SATISFIED | Executor persist-then-upload ordering and CPA/Sub2API-only writeback are implemented and covered by tests. |
| `COMP-03` | `02-02`, `02-04` | Existing polling and websocket consumers receive compatible task and batch progress semantics from Go-owned backend flows. | ✓ SATISFIED | Task/batch HTTP and WS responses preserve status/log cursor semantics; compatibility/e2e tests passed. |

### Anti-Patterns Found

No blocker anti-patterns were found in the scanned Phase 2 implementation files. The grep scan only surfaced benign test-only skips and normal empty-literal test fixtures, not production placeholders or hollow runtime branches.

### Human Verification Required

### 1. Real Native Registration

**Test:** Against staging dependencies, start one single registration, one batch registration, and one Outlook batch registration through the Go API/worker path.
**Expected:** Each flow completes on the Go worker path, task status transitions match current contract, and no Python runner bridge is needed for the normal path.
**Why human:** Requires live Auth/OpenAI plus mailbox/provider integration and a real worker environment.

### 2. Real-Time Task Control

**Test:** While a real registration task is running, use the existing client or a websocket client to send `pause`, `resume`, and `cancel` and observe `/api/ws/task/{task_uuid}` and `/api/ws/batch/{batch_id}`.
**Expected:** The client sees compatible status frames, visible `cancelling`, heartbeat/ping behavior, and monotonic `log_offset`/`log_next_offset`.
**Why human:** Real-time concurrency, timing, reconnect, and long-running task behavior are not fully represented by the in-memory e2e fixtures.

### 3. Live Auto-Upload Side Effects

**Test:** Run a successful registration with CPA, Sub2API, and TM auto-upload enabled against staging uploader endpoints.
**Expected:** CPA/Sub2API mark success timestamps back onto the account; TM only logs and does not persist a success flag.
**Why human:** Requires live external uploader responses and end-to-end side-effect observation in the real persistence environment.

### Gaps Summary

No code or wiring gaps were found against the Phase 2 roadmap success criteria or requirement IDs. The remaining work is manual staging validation of real external integrations and live runtime timing, so the phase is classified as `human_needed` rather than `gaps_found`.

---

_Verified: 2026-04-05T11:02:05Z_
_Verifier: Claude (gsd-verifier)_
