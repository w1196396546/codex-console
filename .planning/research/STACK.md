# Stack Research: v1.1 Go Admin Frontend Refactor

**Project:** Codex Console Go Migration  
**Milestone:** v1.1 Go Admin Frontend Refactor  
**Researched:** 2026-04-06  
**Confidence:** HIGH

## Existing Baseline

- The current operator UI is server-rendered HTML in `templates/` with shared CSS in `static/css/style.css` and page-specific vanilla JS in `static/js/*.js`.
- Python `src/web/app.py` currently mounts `/static`, renders all HTML pages, injects `static_version`, and exposes the login/page shell.
- `backend-go/` already owns the API and worker runtime, but does not yet expose a dedicated admin UI asset tree or shared page rendering layer.
- The current frontend includes shared public/project-promo content through `templates/partials/site_notice.html` and repeated "OpenAI 注册系统" framing in page headers.

## Recommended Stack Direction

### Keep the current delivery model for parity

Do not introduce a SPA framework in this milestone. The safest path is to keep server-rendered pages plus page-specific vanilla JS so the copied frontend can preserve the current DOM structure and API calls while the shell/layout evolves.

### Add a Go-owned frontend asset surface

- Create a dedicated Go-owned frontend directory for copied templates, CSS, and JS.
- Mount that surface through Go-owned routes instead of extending the Python `templates/` / `static/` directories.
- Keep API routers and frontend routers separate so the admin shell can evolve without risking `/api/*` ownership drift.

### Use thin Go template helpers only where needed

Most current pages are static HTML shells with JS-driven behavior. Re-implement only the minimum shared server-rendered concerns in Go:

- login/page entry rendering
- shared layout/shell helpers
- asset version/cache-busting helpers
- page metadata/title wiring

### Chi routing guidance

From chi docs via Context7:

- Prefer mounted subrouters for clear separation between `/api` and admin UI routes.
- Serve static assets with explicit file-server mounts instead of mixing them into business handlers.
- Avoid middleware combinations that conflict with `http.FileServer`; chi docs specifically note `RedirectSlashes` is not compatible with file-server behavior.

## Recommended Additions

- Go-owned admin UI router mounted separately from `/api`
- Dedicated static asset mount for copied CSS/JS/icons
- Shared Go template helpers for page shell, layout, and asset versioning
- Optional embedded/static filesystem packaging only if it does not slow page-parity work

## What Not To Add

- No SPA rewrite
- No new frontend framework adoption for this milestone
- No in-place edits to the legacy Python frontend asset tree
- No backend contract redesign to suit new UI tastes

## Integration Implications

- Page-specific JS can stay close to current behavior, but shared shell concerns should move into reusable Go-owned layout primitives.
- Asset versioning must remain explicit so copied assets do not suffer from stale browser caches during rollout.
- Route organization should preserve rollback: legacy Python frontend remains available while Go-owned pages are introduced.

---
*Research completed: 2026-04-06*
