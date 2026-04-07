#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

require_cmd go

gated_live_checks=0
ran_live_checks=0

echo "==> Phase 5 cutover: local migration suite"
(cd backend-go && go test ./db/migrations -v)

echo "==> Phase 5 cutover: local runtime wiring suite"
(
  cd backend-go
  go test ./cmd/api -run 'TestAPI(Payment|Team)Runtime.*' -v
  go test ./internal/http -run 'Test(Router|PaymentTeam|PhaseFour).*' -v
)

echo "==> Phase 5 cutover: local registration compatibility suite"
(
  cd backend-go
  go test ./tests/e2e -run 'TestRegistration(CompatibilityFlow|WebSocketCompatibility|BatchCompatibilityFlow|BatchWebSocketCompatibility|OutlookBatchCompatibility|StatsCompatibilityEndpoint)' -v
)

echo "==> Phase 5 cutover: local management compatibility suite"
(
  cd backend-go
  go test ./tests/e2e -run 'Test(RecentAccountsCompatibilityEndpoint|ManagementAccountsCompatibilityEndpoints|ManagementAccountsOverviewRefreshCompatibility|ManagementSettingsCompatibilityEndpoints|ManagementEmailServicesCompatibilityEndpoints|ManagementUploaderCompatibilityEndpoints|ManagementLogsCompatibilityEndpoints)' -v
)

echo "==> Phase 5 cutover: local payment and team compatibility suite"
(
  cd backend-go
  go test ./tests/e2e -run 'TestPaymentPhaseFourCompatibilityRoutes|TestTeamPhaseFourAcceptedTaskLiveFlow' -v
)

if [[ -n "${BACKEND_GO_BASE_URL:-}" ]]; then
  echo "==> Phase 5 cutover: live backend health check"
  (cd backend-go && BACKEND_GO_BASE_URL="$BACKEND_GO_BASE_URL" go test ./tests/e2e -run 'TestHealthzEndpoint' -v)
  ran_live_checks=$((ran_live_checks + 1))
else
  echo "SKIP/GATED live checks: BACKEND_GO_BASE_URL is not set"
  gated_live_checks=$((gated_live_checks + 1))
fi

if [[ -n "${MIGRATION_TEST_DATABASE_URL:-${DATABASE_URL:-}}" ]]; then
  echo "==> Phase 5 cutover: live PostgreSQL migration check"
  (
    cd backend-go
    MIGRATION_TEST_DATABASE_URL="${MIGRATION_TEST_DATABASE_URL:-$DATABASE_URL}" \
      go test ./db/migrations -run 'TestGooseMigratesLegacySchemaWithRealPostgres' -v
  )
  ran_live_checks=$((ran_live_checks + 1))
else
  echo "SKIP/GATED live checks: MIGRATION_TEST_DATABASE_URL or DATABASE_URL is not set"
  gated_live_checks=$((gated_live_checks + 1))
fi

echo "==> Phase 5 cutover summary"
echo "LOCAL checks: PASS"

if (( gated_live_checks > 0 )); then
  echo "LIVE checks: SKIP/GATED (${gated_live_checks} skipped, ${ran_live_checks} executed)"
  echo "Final Phase 5 sign-off still requires running the gated live checks in a real environment."
else
  echo "LIVE checks: PASS (${ran_live_checks} executed)"
fi
