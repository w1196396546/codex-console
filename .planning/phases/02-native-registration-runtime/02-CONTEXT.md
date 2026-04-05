# Phase 2: Native Registration Runtime - Context

**Gathered:** 2026-04-05
**Status:** Ready for planning
**Mode:** Auto-generated (discuss skipped via workflow.skip_discuss)

<domain>
## Phase Boundary

Complete Go ownership of registration execution, task lifecycle, and upload side effects so Python is no longer required on the registration critical path. This phase covers native registration preparation/execution, task and batch progress semantics, and Go-side account persistence plus automatic uploader side effects. It does not yet migrate the broader management APIs, payment domain, or team domain.

</domain>

<decisions>
## Implementation Decisions

### Runtime ownership
- **D-01:** Existing Python registration behavior remains the compatibility oracle for request/response semantics until the Go path proves parity.
- **D-02:** The Python runner bridge may be used only as a bounded transition aid during implementation; the completed Phase 2 critical path must not require it for normal registration execution.

### Compatibility obligations
- **D-03:** Task, batch, Outlook batch, websocket, pause/resume/cancel, and log offset semantics are part of the runtime contract and must remain compatible.
- **D-04:** Account persistence plus CPA/Sub2API/TM auto-upload side effects must remain compatible with current workflow timing and data shape.

### Scope boundary
- **D-05:** Treat existing Go registration/task/websocket compatibility surfaces as baseline foundations to extend, not to rewrite from scratch.
- **D-06:** Do not pull broader account-management, settings, payment, or team domain migration into this phase.

### the agent's Discretion
The agent may choose the exact internal decomposition for native registration execution, compatibility fixtures, and persistence adapters as long as the external runtime contract remains compatible and the Python bridge is removed from the completed critical path.

</decisions>

<specifics>
## Specific Ideas

No additional product preferences beyond the roadmap and Phase 1 contracts. Follow the frozen route/client, schema, and runtime contracts from Phase 1 when deciding where native Go ownership starts and what compatibility evidence is required before cutting the Python runner out of the path.

</specifics>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project and phase scope
- `.planning/PROJECT.md` — Migration framing, validated baseline, and compatibility constraints.
- `.planning/REQUIREMENTS.md` — Phase requirement IDs `RUN-01`, `RUN-02`, `RUN-03`, `COMP-03`.
- `.planning/ROADMAP.md` — Phase 2 goal, success criteria, and later-phase boundaries.

### Phase 1 contracts
- `.planning/phases/01-compatibility-baseline/01-route-parity-inventory.json` — Current registration route ownership and client consumer map.
- `.planning/phases/01-compatibility-baseline/01-route-client-parity-matrix.md` — Human-reviewable parity matrix for registration/task/websocket surfaces.
- `.planning/phases/01-compatibility-baseline/01-shared-schema-contract.md` — Shared storage contract and Python-only table boundaries.
- `.planning/phases/01-compatibility-baseline/01-runtime-boundary-contract.md` — Auth/runtime/task semantics that Phase 2 must preserve.
- `.planning/phases/01-compatibility-baseline/01-compat-fixture-manifest.md` — Existing automated coverage and coverage gaps relevant to registration compatibility.
- `.planning/phases/01-compatibility-baseline/01-ops-compat-runbook.md` — Stop-ship and verification expectations established in Phase 1.

### Current registration/runtime implementation
- `src/web/routes/registration.py` — Python registration/task/orchestrator API behavior.
- `src/web/routes/websocket.py` — Python websocket task and batch semantics.
- `src/web/task_manager.py` — Python process-local runtime state and log behavior.
- `src/core/register.py` — Python registration engine critical-path behavior.
- `backend-go/internal/registration/` — Current Go registration compatibility and execution baseline.
- `backend-go/internal/nativerunner/` — Native Go registration runner foundation.
- `backend-go/cmd/worker/main.go` — Go worker bootstrap and registration runner wiring.
- `backend-go/internal/accounts/` — Go account persistence baseline.
- `backend-go/internal/uploader/` — Go uploader config/payload baseline.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `backend-go/internal/registration/http/handlers.go`, `service.go`, `batch_service.go`, `outlook_service.go`, and `ws/*.go` already provide the compatibility shell that Phase 2 should extend.
- `backend-go/internal/nativerunner/` already contains auth, mail, signup preparation, and token-completion foundations.
- Phase 1 compatibility artifacts already isolate which registration/task/websocket surfaces are baseline versus still contract-critical.

### Established Patterns
- Go registration work already follows service + handler + worker separation; keep extending that instead of copying Python monolith structure.
- Python route/task behavior is still the oracle for observable semantics, especially around logs, incremental offsets, and control operations.
- Existing Go-side tests under `backend-go/tests/e2e/` and `backend-go/internal/registration/*_test.go` should be extended before introducing new runtime behavior.

### Integration Points
- Native registration execution plugs into `backend-go/cmd/worker/main.go` and `backend-go/internal/registration/executor.go`.
- Task/batch compatibility semantics cross `backend-go/internal/registration/http/`, `ws/`, `jobs/`, and `accounts/`.
- Upload side effects connect through `backend-go/internal/uploader/` and any Go-side auto-upload dispatcher paths.

</code_context>

<deferred>
## Deferred Ideas

- Broader account-management endpoints remain Phase 3 scope.
- Payment/bind-card runtime migration remains Phase 4 scope.
- Team runtime migration remains Phase 4 scope.

</deferred>

---
*Phase: 02-native-registration-runtime*
*Context gathered: 2026-04-05*
