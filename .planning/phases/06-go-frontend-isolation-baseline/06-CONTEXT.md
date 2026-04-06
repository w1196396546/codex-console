# Phase 6: Go Frontend Isolation Baseline - Context

**Gathered:** 2026-04-06
**Status:** Ready for planning
**Source:** Milestone v1.1 kickoff + user instruction

<domain>
## Phase Boundary

Phase 6 establishes the technical isolation baseline for the new Go-owned admin frontend.
It does not redesign all pages yet. It creates the copied frontend workspace, the Go-side
route/static mount points, and the minimum shared infrastructure needed so later phases can
refactor the copied frontend independently while leaving the legacy Python frontend untouched.

</domain>

<decisions>
## Implementation Decisions

### Locked Decisions

- The existing Python frontend under `templates/` and `static/` must remain untouched.
- The new admin frontend must be built from a dedicated Go-owned copy of frontend assets.
- The Go runtime must serve the new admin frontend without putting Python back on the critical path.
- Phase 6 focuses on isolation, bootstrapping, and verification, not full visual redesign.
- Legacy frontend fallback must remain available during the rollout.
- Backend API paths, response shapes, and websocket behavior must remain compatible.

### The Agent's Discretion

- Exact directory names for the copied Go frontend asset tree
- Exact Go package/file layout for template rendering and static mounting
- Whether to render through `html/template`, embedded filesystems, or plain disk-backed files first
- Exact baseline verification approach, as long as it proves legacy untouched + new Go frontend bootable

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Existing frontend surface
- `templates/index.html` — current registration/home page shell
- `templates/accounts.html` — current management page shell and nav pattern
- `templates/settings.html` — current tab-heavy admin page shape
- `templates/logs.html` — current monitoring/admin page shape
- `templates/partials/site_notice.html` — shared project notice content that must not carry into new shell
- `static/css/style.css` — current shared design tokens and layout primitives
- `static/js/` — current page-specific JavaScript behavior that later phases must preserve

### Current runtime ownership
- `src/web/app.py` — legacy Python page routing and static/template mounts
- `backend-go/internal/http/router.go` — Go router composition entry
- `backend-go/cmd/api/main.go` — Go API bootstrap

### Planning artifacts
- `.planning/PROJECT.md`
- `.planning/REQUIREMENTS.md`
- `.planning/ROADMAP.md`
- `.planning/research/SUMMARY.md`

</canonical_refs>

<specifics>
## Specific Ideas

- Prefer a dedicated Go-side admin UI subtree rather than reusing Python template/static directories.
- Add explicit static asset mounts and page routes in Go so later phases can migrate page-by-page.
- Keep enough shared shell infrastructure in Phase 6 to support login/layout/versioning reuse.
- Record pre-existing Go test failures separately; do not let unrelated baseline failures block frontend isolation work.

</specifics>

<deferred>
## Deferred Ideas

- Shared admin shell visual redesign belongs to Phase 7.
- High-traffic page migration belongs to Phase 8.
- Payment/team/card-pool/login polish and rollout readiness belong to Phase 9.

</deferred>

---

*Phase: 06-go-frontend-isolation-baseline*
*Context gathered: 2026-04-06 via milestone kickoff*
