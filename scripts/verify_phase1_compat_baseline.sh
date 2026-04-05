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

require_cmd pytest
require_cmd node
require_cmd go

echo "==> Phase 1 compatibility baseline: Python contract suite"
pytest tests/test_registration_routes.py tests/test_accounts_routes.py tests/test_settings_routes.py tests/test_payment_routes.py tests/test_team_routes.py tests/test_team_tasks_routes.py -q

echo "==> Phase 1 compatibility baseline: frontend contract suite"
node --test tests/frontend/registration_log_buffer.test.mjs tests/frontend/accounts_state_actions.test.mjs tests/frontend/accounts_team_entry.test.mjs tests/frontend/auto_team.test.mjs

echo "==> Phase 1 compatibility baseline: Go static contract suite"
(cd backend-go && go test ./db/migrations -v)

echo "==> Phase 1 compatibility baseline: Go compatibility suite"
(cd backend-go && go test ./tests/e2e -run 'TestRecentAccountsCompatibilityEndpoint|TestRegistrationCompatibilityFlow|TestRegistrationWebSocketCompatibility|TestRegistrationBatchCompatibilityFlow' -v)

if [[ -n "${BACKEND_GO_BASE_URL:-}" ]]; then
  echo "==> Phase 1 compatibility baseline: optional live health check"
  (cd backend-go && BACKEND_GO_BASE_URL="$BACKEND_GO_BASE_URL" go test ./tests/e2e -run 'TestHealthzEndpoint' -v)
else
  echo "SKIP live checks: BACKEND_GO_BASE_URL is not set"
fi

if [[ -n "${MIGRATION_TEST_DATABASE_URL:-${DATABASE_URL:-}}" ]]; then
  echo "==> Phase 1 compatibility baseline: optional real PostgreSQL migration check"
  (
    cd backend-go
    MIGRATION_TEST_DATABASE_URL="${MIGRATION_TEST_DATABASE_URL:-$DATABASE_URL}" \
      go test ./db/migrations -run 'TestGooseMigratesLegacySchemaWithRealPostgres' -v
  )
else
  echo "SKIP live checks: MIGRATION_TEST_DATABASE_URL or DATABASE_URL is not set"
fi
