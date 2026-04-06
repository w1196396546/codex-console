# 06-03 Summary

## Outcome

Phase 6 baseline verification is in place for both frontends:

- Go admin frontend now mounts `/go-admin/login`, protected `/go-admin/*` pages, and `/go-admin/static/*` assets through the Go API process.
- Legacy Python frontend still boots and serves its existing login/redirect/static baseline unchanged.

## Evidence

- `cd backend-go && go test ./internal/adminui ./internal/http ./cmd/api ./tests/e2e -run 'Test(AdminUIBaselineCopiedAssetsExist|Router.*AdminUI.*|APIHandlerMountsAdminUI|AdminFrontendBaselineRoutes)'`
- `python` smoke check against `src.web.app:create_app()` confirmed:
  - `/login` returns `200`
  - `/` redirects to `/login?next=/`
  - `/accounts` redirects to `/login?next=/accounts`
  - `/static/css/style.css` returns `200`

## Notes

- The current environment does not expose a `pytest` executable, so the legacy smoke was verified with an inline Python command instead of `pytest tests/test_legacy_webui_smoke.py`.
- Existing unrelated Go baseline failures under `internal/nativerunner/auth` and `db/migrations_test` remain outside this phase and were not part of the targeted verification set.
