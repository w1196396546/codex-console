# Phase 5: Cutover and Decommission - Research

**Researched:** 2026-04-06
**Domain:** Final Go backend cutover, compatibility evidence, and Python backend decommission planning
**Confidence:** MEDIUM

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Phase 5 must stage cutover before decommission.
- **D-02:** Deferred live/operator validation from earlier phases becomes mandatory evidence here.
- **D-03:** Preserve the existing frontend/UI contract; no frontend rewrite.
- **D-04:** Go API and worker are the target production backend entrypoints.
- **D-05:** Python may remain only if it is no longer on the production backend critical path.
- **D-06:** Residual Python bridges must be removed from the production path or explicitly isolated and documented.
- **D-07:** Autonomous defaults; avoid extra confirmation churn.
- **D-08:** Runbooks, startup scripts, compose/manifests, and verification harnesses are phase-critical deliverables.
- **D-09:** Reuse Phase 1-4 artifacts instead of reopening domain-level migration scope.

### Deferred Ideas (OUT OF SCOPE)
- Full frontend replacement/removal unless required for backend critical-path cutover.
- Auth-boundary redesign unless cutover requires it.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| CUT-02 | Production can route backend traffic to Go for all in-scope flows with rollback instructions documented and tested; Python backend responsibilities are removed from the production critical path or explicitly isolated; compatibility evidence exists before final migration sign-off. | The findings below identify the current Python-first deployment topology, the Go runtime entrypoints already available, the residual Python runtime helpers that still exist, and the exact evidence/runbook surfaces Phase 5 must own to close the milestone. |

</phase_requirements>

## Summary

Phase 5 is not another domain migration; it is the runtime ownership phase. The repo already has Go-owned APIs and worker execution for the migrated domains, but the visible startup path, local deployment story, and operator documentation are still Python-first. `webui.py`, `scripts/docker/start-webui.sh`, `docker-compose.yml`, and the top-level `README.md` still describe or launch the Python FastAPI/Jinja application as the primary service, while `backend-go/cmd/api/main.go` and `backend-go/cmd/worker/main.go` remain standalone entrypoints that are not yet presented as the default production topology.

The remaining risk is therefore not just code parity. It is operational ambiguity: the repo can claim Go ownership at the domain level while still deploying, documenting, or falling back to Python in ways that keep Python on the production backend critical path. Phase 5 must close that ambiguity with explicit topology docs, rollback instructions, environment-gated verification, and a clear isolation/removal strategy for residual Python backend helpers.

**Primary recommendation:** split Phase 5 into two plans. `05-01` should establish the cutover topology, rollback path, and compatibility evidence checklist. `05-02` should remove or isolate residual Python backend responsibilities from the production path and refresh the published deployment/docs surface so operators cannot accidentally keep using the old topology.

## Current Runtime Ownership Snapshot

### Python still owns or anchors
- `webui.py` is still the top-level application entrypoint and initializes storage, settings, logging, and Uvicorn startup.
- `src/web/app.py` still mounts HTML pages, login cookie flow, `/api`, and `/api/ws`.
- `scripts/docker/start-webui.sh` and `docker-compose.yml` still start the Python web UI as the visible containerized runtime.
- Top-level `README.md` still publishes Python and Docker-compose usage centered on `python webui.py`.

### Go already owns or exposes
- `backend-go/cmd/api/main.go` mounts the migrated domain services, including accounts/settings/logs/payment/team, behind the Go router.
- `backend-go/cmd/worker/main.go` runs the queue worker and native registration executor.
- Go runtime preconditions are explicit: `DATABASE_URL` and `REDIS_ADDR` are required for the Go-owned production path.
- Phase 4 verification already confirms payment/team runtime ownership through Go APIs; earlier phase verifications cover registration and management domains.

### Residual Python backend risks
- `backend-go/internal/registration/python_runner.go` and `python_runner_script.go` still exist as transition aid code and must be audited against the final production path.
- Python-side payment bridge helpers still exist in `src/web/routes/payment.py`; Phase 4 closed the Go contract, but Phase 5 must ensure production routing no longer depends on those Python route implementations.
- Documentation drift exists: `backend-go/README.md` still says payment and bind-card flows are not yet migrated.

## Recommended Phase 5 Slices

### 05-01: Execute staged cutover, rollback, and parity verification
- Publish the target production topology: which process serves backend APIs, which process runs queued work, what env vars/services are required, and how rollback works.
- Add an environment-gated cutover verification harness that proves the Go-owned path for API, worker, websocket/task, and the live operator workflows deferred in Phases 2-4.
- Update operator-facing docs/runbooks so they no longer describe a Python-first backend ownership model.

### 05-02: Retire Python backend responsibilities from the production path
- Rewire startup/deployment assets so production no longer defaults to Python-backed backend ownership.
- Audit residual Python runtime bridges/helpers and either remove them from the production path or isolate them behind an explicit compatibility boundary with owner/follow-up.
- Refresh final docs to make the Go-owned backend path the only supported production baseline, with any remaining Python shell clearly marked as non-critical or legacy.

## Evidence Checklist

Phase 5 should not claim success without explicit evidence for all of the following:

- Go API + worker production topology documented with required env/services and startup order.
- Rollback steps documented and exercised at least in a bounded, environment-gated validation path.
- Compatibility evidence captured for the deferred live checks from Phases 2-4:
  - registration/runtime/operator checks
  - management/admin checks
  - payment/team operator checks
- Published docs (`README.md`, `backend-go/README.md`, runbooks) updated so they no longer misstate ownership.
- Residual Python backend dependencies inventoried with a disposition for each: removed, isolated, or accepted as non-critical shell/oracle.

## Open Risks

- The current workspace does not itself prove access to a live cutover environment, so the plan must be explicit about environment-gated validation and should not fabricate final sign-off.
- Python still mounts `/api` and `/api/ws` in the existing UI application; if the final deployment keeps that process, the plan must clearly define whether it proxies to Go, becomes presentation-only, or is removed from the production path.
- Removing Python too aggressively could accidentally re-scope the project into a frontend rewrite; the phase must keep its boundary on backend ownership and operational cutover.

## Anti-Patterns to Avoid

- **Do not treat docs/runbooks as optional.** In this phase they are part of the runtime truth.
- **Do not delete Python entrypoints first and figure out rollback later.** Rollback must exist before destructive decommission steps.
- **Do not claim Phase 5 complete from unit tests alone.** This phase needs live, environment-gated compatibility evidence.
- **Do not equate “Python code still exists” with “Python is still on the critical path.”** The real question is whether production backend ownership still depends on it.

## Planning Recommendations

1. Make `05-01` the evidence and topology plan, not a vague “operator validation” bucket.
2. Make `05-02` explicitly about production-path ownership cleanup, not blanket deletion of all Python code.
3. Keep verification commands split into always-on local checks and environment-gated cutover checks.
4. Treat stale docs (`backend-go/README.md`, top-level startup instructions, compose/entrypoint scripts) as blockers for final cutover readiness.

---
*Phase: 05-cutover-and-decommission*
*Research completed: 2026-04-06*
