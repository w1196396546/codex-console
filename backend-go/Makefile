test:
	go test ./...

sqlc-generate:
	sqlc generate

migrate-up:
	goose -dir db/migrations postgres "$(DATABASE_URL)" up

run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker
