guard-sqlc:
	@command -v sqlc >/dev/null 2>&1 || (echo "sqlc is required in PATH"; exit 1)

guard-goose:
	@command -v goose >/dev/null 2>&1 || (echo "goose is required in PATH"; exit 1)

guard-database-url:
	@test -n "$(DATABASE_URL)" || (echo "DATABASE_URL is required"; exit 1)

guard-backend-go-base-url:
	@test -n "$(BACKEND_GO_BASE_URL)" || (echo "BACKEND_GO_BASE_URL is required for e2e"; exit 1)

test:
	go test ./...

test-migrations:
	go test ./db/migrations -v

test-migrations-pg:
	go test ./db/migrations -run TestGooseMigratesLegacySchemaWithRealPostgres -v

test-e2e: guard-backend-go-base-url
	go test ./tests/e2e -v

sqlc-generate: guard-sqlc
	sqlc generate

migrate-up: guard-goose guard-database-url
	goose -dir db/migrations postgres "$(DATABASE_URL)" up

verify-phase1: guard-backend-go-base-url
	go test ./...
	BACKEND_GO_BASE_URL="$(BACKEND_GO_BASE_URL)" go test ./tests/e2e -v

verify-phase5:
	bash ../scripts/verify_phase5_cutover.sh

run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker
