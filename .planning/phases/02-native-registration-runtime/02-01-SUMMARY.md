---
phase: 02-native-registration-runtime
plan: "01"
subsystem: auth
tags: [registration, nativerunner, worker, postgres, outlook]
requires:
  - phase: 01-compatibility-baseline
    provides: registration runtime boundary contract, Python oracle semantics, and compatibility fixtures
provides:
  - explicit worker wiring for proxy selection and Outlook reservation during native registration preparation
  - native signup continuation coverage for password login, workspace fallback, and add-phone boundaries
  - executable evidence that normal registration execution stays on the Go native runner path
affects: [02-02-task-runtime-semantics, 02-03-account-persistence, 03-management-apis]
tech-stack:
  added: []
  patterns: [jobs-payload-backed outlook reservation, worker-injected preparation adapters, native auth continuation without python bridge]
key-files:
  created:
    - backend-go/internal/registration/proxy_selector.go
    - backend-go/internal/registration/outlook_reservation_store.go
    - backend-go/internal/registration/executor_preparation_test.go
    - backend-go/internal/nativerunner/auth/login_password.go
    - backend-go/internal/nativerunner/auth/login_password_test.go
    - backend-go/internal/nativerunner/auth/workspace_resolution.go
  modified:
    - backend-go/cmd/worker/main.go
    - backend-go/cmd/worker/main_test.go
    - backend-go/internal/registration/orchestrator.go
    - backend-go/internal/registration/orchestrator_test.go
    - backend-go/internal/nativerunner/prepare_signup_flow.go
    - backend-go/internal/nativerunner/prepare_signup_flow_test.go
    - backend-go/internal/nativerunner/auth/post_signup.go
    - backend-go/internal/nativerunner/auth/post_signup_test.go
key-decisions:
  - "Worker preparation now injects explicit Postgres-backed proxy selection and Outlook reservation adapters instead of relying on implicit no-op preparation state."
  - "Outlook reservation state is stored in registration job payloads so concurrent child jobs do not introduce a second runtime store before Phase 02-02."
  - "Password login, workspace continuation, and add-phone boundary recovery stay inside native auth helpers so the Python bridge is not reintroduced on the normal path."
patterns-established:
  - "Preparation adapters: worker bootstrap owns concrete settings/catalog/outlook/proxy/reservation dependencies and passes them into registration.WithPreparationDependencies."
  - "Native auth continuation: PrepareSignupFlow may branch into password login or token completion, but always returns Go-owned persistence payloads."
requirements-completed: [RUN-01]
duration: 10min
completed: 2026-04-05
---

# Phase 2 Plan 01: Native Registration Critical Path Summary

**Worker-native registration now resolves proxies and Outlook reservations in Go while native signup/token-continuation branches complete without requiring the Python runner on the normal path**

## Performance

- **Duration:** 10 min
- **Started:** 2026-04-05T10:25:00Z
- **Completed:** 2026-04-05T10:34:58Z
- **Tasks:** 2
- **Files modified:** 14

## Accomplishments
- Worker bootstrap now injects concrete proxy-selection and Outlook-reservation dependencies into native registration preparation.
- Orchestrator and executor preparation coverage now prove batch-child and Outlook-child jobs receive prepared service, proxy, and reservation plans.
- Native runner coverage now includes password login continuation, workspace fallback, and add-phone boundary handling without routing normal success paths back through Python.

## Task Commits

Each task was committed atomically:

1. **Task 1: 补齐 worker 准备依赖与原生执行入口接线** - `0b4678c` (feat)
2. **Task 2: 对齐 native prepare/signup 关键分支与 Python oracle** - `1f47c28` (feat)

**Plan metadata:** pending

## Files Created/Modified
- `backend-go/cmd/worker/main.go` - wires Postgres-backed preparation dependencies into the native registration executor.
- `backend-go/cmd/worker/main_test.go` - verifies worker preparation uses concrete proxy/reservation adapters.
- `backend-go/internal/registration/proxy_selector.go` - adds request, proxy-pool, dynamic-proxy, and static-proxy selection for native preparation.
- `backend-go/internal/registration/outlook_reservation_store.go` - persists Outlook service reservations in registration job payloads.
- `backend-go/internal/registration/orchestrator.go` - prepares proxy-aware email-service plans and reservation-aware Outlook execution plans.
- `backend-go/internal/nativerunner/prepare_signup_flow.go` - completes native signup, token completion, and existing-account continuation branches.
- `backend-go/internal/nativerunner/auth/login_password.go` - handles historical-password login and OAuth token exchange in Go.
- `backend-go/internal/nativerunner/auth/post_signup.go` - follows callback, workspace, organization, and add-phone continuation branches in native auth.
- `backend-go/internal/nativerunner/auth/workspace_resolution.go` - resolves workspace IDs from auth cookies and consent HTML fallback.

## Decisions Made

- Used existing registration job payloads as the Outlook reservation source of truth to satisfy D-05 and avoid inventing a second runtime store before 02-02.
- Kept proxy selection inside a narrow runtime adapter that reads current settings and legacy proxy rows without pulling proxy-management APIs into Phase 3 scope.
- Preserved the native-runner boundary by extending auth continuation helpers instead of sending successful signup recovery back through `python_runner.go`.

## Deviations from Plan

None - plan scope and write set were preserved.

## Issues Encountered

- The official Task 1 verification command `cd backend-go && go test ./cmd/worker ./internal/registration -run 'TestNewRegistrationPreparationDependenciesUsesPostgresRepositories|TestOrchestrator.*|TestExecutor.*Preparation.*' -v` was attempted, but the current workspace contains out-of-scope dirty tests in `backend-go/internal/registration/batch_service_test.go` and `backend-go/internal/registration/outlook_service_test.go` that now expect 02-02 progress fields (`Skipped`, `CurrentIndex`, `LogBaseIndex`). Per the user constraint, those files were not modified. In-scope fallback verification used `go test ./cmd/worker -run TestNewRegistrationPreparationDependenciesUsesPostgresRepositories -v` and `go build ./internal/registration`, while Task 2 official tests passed unchanged.

## User Setup Required

None - no external service configuration required.

## Threat Flags

| Flag | File | Description |
|------|------|-------------|
| threat_flag: outbound-dynamic-proxy | backend-go/internal/registration/proxy_selector.go | Worker preparation can call a configured dynamic proxy API before native registration starts when no request or DB proxy is available. |

## Next Phase Readiness

- 02-02 can build on explicit proxy/outlook preparation data already attached to worker execution plans.
- Native auth continuation now has executable coverage for password login, workspace fallback, and add-phone boundaries, which reduces the remaining Python-bridge surface for runtime semantics work.
- Package-wide `./internal/registration` verification still needs the out-of-scope 02-02 progress-field WIP to be isolated or completed before it can serve as a clean gate again.

---
*Phase: 02-native-registration-runtime*
*Completed: 2026-04-05*

## Self-Check: PASSED
