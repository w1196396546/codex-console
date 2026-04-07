# Phase 5: Cutover and Decommission - Context

**Gathered:** 2026-04-06
**Status:** Ready for planning
**Mode:** Auto-generated (user requested autonomous defaults)

<domain>
## Phase Boundary

Cut production backend ownership over to Go, verify compatibility end to end, and retire Python backend responsibilities from the production critical path. This phase covers cutover topology, rollback, compatibility evidence, deployment/runtime ownership, and residual Python isolation or removal. It does not redesign the frontend and does not change the existing API, data, or operator-facing workflow contracts.

</domain>

<decisions>
## Implementation Decisions

### Scope and sequencing
- **D-01:** Phase 5 must sequence work as staged cutover first, decommission second; do not delete or bypass rollback paths before Go-owned runtime evidence exists.
- **D-02:** The deferred live/operator validation from Phases 2-4 becomes mandatory evidence in Phase 5, not an optional follow-up.
- **D-03:** Preserve the existing frontend/UI contract; Phase 5 is a backend ownership and deployment cutover phase, not a frontend rewrite phase.

### Runtime ownership defaults
- **D-04:** `backend-go/cmd/api/main.go` and `backend-go/cmd/worker/main.go` are the target production backend entrypoints; PostgreSQL and Redis remain mandatory prerequisites for the Go-owned path.
- **D-05:** Python may remain temporarily only as a bounded presentation shell or compatibility oracle if it no longer owns production-critical backend behavior.
- **D-06:** Residual Python backend helpers and bridges, including registration Python runner paths and Python-side payment bridge helpers, must either be removed from the production path or explicitly isolated with documented owner, rollback, and follow-up handling.

### Planning constraints
- **D-07:** The user requested autonomous defaults; do not stop for menu-style confirmation unless the action is destructive, external to the repo, or otherwise high-risk.
- **D-08:** Deployment docs, runbooks, startup scripts, compose/manifests, and verification harnesses are first-class deliverables in this phase because the cutover truth lives there as much as it does in code.
- **D-09:** Keep the migration brownfield-safe: reuse the completed Phase 1-4 artifacts as truth sources instead of reopening domain-level compatibility design.

### the agent's Discretion
The agent may choose the exact decomposition of cutover runbooks, verification scripts, deployment topology docs, and isolation strategy for residual Python code as long as rollback remains explicit, the compatibility contract stays intact, and Python is no longer on the production backend critical path by the end of the phase.

</decisions>

<specifics>
## Specific Ideas

- The repo still starts production-like local runtime through `webui.py`, `scripts/docker/start-webui.sh`, and `docker-compose.yml`; Phase 5 must make the Go-owned topology explicit instead of assuming it.
- `backend-go/README.md` is stale and still describes payment/bind-card as not yet migrated; Phase 5 should treat operator-facing docs as part of the cutover surface.
- Frontend templates and `static/js` should continue working without a redesign; the critical change is backend ownership and deployment topology, not page behavior.

</specifics>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project and milestone state
- `.planning/PROJECT.md`
- `.planning/REQUIREMENTS.md`
- `.planning/ROADMAP.md`
- `.planning/STATE.md`

### Compatibility and cutover constraints
- `.planning/phases/01-compatibility-baseline/01-route-client-parity-matrix.md`
- `.planning/phases/01-compatibility-baseline/01-runtime-boundary-contract.md`
- `.planning/phases/01-compatibility-baseline/01-ops-compat-runbook.md`
- `.planning/phases/02-native-registration-runtime/02-VERIFICATION.md`
- `.planning/phases/03-management-apis/03-VERIFICATION.md`
- `.planning/phases/04-payment-and-team-domains/04-VERIFICATION.md`

### Current runtime entrypoints and deployment surface
- `webui.py`
- `src/web/app.py`
- `scripts/docker/start-webui.sh`
- `docker-compose.yml`
- `README.md`
- `backend-go/README.md`
- `backend-go/docs/phase1-runbook.md`

### Current Go production-target runtime
- `backend-go/cmd/api/main.go`
- `backend-go/cmd/worker/main.go`
- `backend-go/internal/http/router.go`
- `backend-go/internal/registration/python_runner.go`
- `backend-go/internal/registration/python_runner_script.go`

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Phase 1 already froze the runtime boundary, cutover preconditions, and operator runbook expectations.
- Phases 2-4 already produced verification artifacts that can seed the Phase 5 parity checklist instead of rediscovering domain-level contracts.
- Go API and worker entrypoints are already separated and production-shaped; Phase 5 mainly needs ownership, topology, and validation work around them.

### Established Patterns
- Domain migrations have been additive and compatibility-first; Phase 5 should keep that pattern and avoid big-bang replacement.
- Environment-gated verification is already used in the repo (`BACKEND_GO_BASE_URL`, migration/test guards); Phase 5 should reuse that style for cutover checks.
- The repo still publishes Python-first startup instructions, so documentation drift is a real deployment risk.

### Integration Points
- `docker-compose.yml` and `scripts/docker/start-webui.sh` currently anchor the visible deployment path.
- `README.md` and `backend-go/README.md` are operator-facing entrypoints that must stop describing stale ownership.
- `backend-go/cmd/api/main.go` and `backend-go/cmd/worker/main.go` are the concrete runtime seams where Phase 5 cutover evidence points.

</code_context>

<deferred>
## Deferred Ideas

- Full frontend replacement or removal of the Python/Jinja UI shell, if still desired after backend cutover, is out of scope unless required to remove Python from the backend critical path.
- Auth hardening or page-level security redesign remains out of scope unless it is required to preserve the existing contract during cutover.

</deferred>

---
*Phase: 05-cutover-and-decommission*
*Context gathered: 2026-04-06*
