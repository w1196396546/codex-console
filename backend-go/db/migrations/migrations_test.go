package migrations_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

const (
	jobsMigration                 = "0001_init_jobs.sql"
	accountsRegistrationMigration = "0002_init_accounts_registration.sql"
	serviceConfigsMigration       = "0003_extend_registration_service_configs.sql"
)

func TestSQLMigrationsStartWithGooseUpAnnotation(t *testing.T) {
	for _, path := range []string{
		jobsMigration,
		accountsRegistrationMigration,
		serviceConfigsMigration,
	} {
		sql := mustReadMigration(t, path)
		firstLine := firstNonEmptyLine(sql)
		if firstLine != "-- +goose Up" {
			t.Fatalf("migration %s must start with -- +goose Up, got %q", path, firstLine)
		}
	}
}

func TestMigrationVerificationEntrypointsAreDocumented(t *testing.T) {
	makefile := mustReadProjectFile(t, "../../Makefile")
	readme := mustReadProjectFile(t, "../../README.md")
	runbook := mustReadProjectFile(t, "../../docs/phase1-runbook.md")

	if !strings.Contains(makefile, "test-migrations-pg:") {
		t.Fatalf("Makefile missing test-migrations-pg target")
	}

	if !strings.Contains(readme, "make test-migrations-pg") {
		t.Fatalf("README missing make test-migrations-pg command")
	}

	if !strings.Contains(runbook, "make test-migrations-pg") {
		t.Fatalf("runbook missing make test-migrations-pg command")
	}

	if !strings.Contains(runbook, "MIGRATION_TEST_DATABASE_URL") && !strings.Contains(runbook, "DATABASE_URL") {
		t.Fatalf("runbook missing migration test database configuration")
	}

	if !strings.Contains(readme, "without a system goose binary") {
		t.Fatalf("README missing no-system-goose guidance for test-migrations-pg")
	}

	if !strings.Contains(runbook, "without a system goose binary") {
		t.Fatalf("runbook missing no-system-goose guidance for test-migrations-pg")
	}
}

func TestRealPostgresMigrationHelperDoesNotDependOnGooseCLI(t *testing.T) {
	source := mustReadProjectFile(t, "migrations_test.go")
	checkedFunctions := []string{
		"TestGooseMigratesLegacySchemaWithRealPostgres",
		"mustRunGooseUp",
	}

	disallowedSnippets := []string{
		`exec.LookPath("goose")`,
		`exec.CommandContext(ctx, "goose"`,
	}

	for _, functionName := range checkedFunctions {
		functionSource := mustReadFunctionSource(t, source, functionName)
		for _, snippet := range disallowedSnippets {
			if strings.Contains(functionSource, snippet) {
				t.Fatalf("%s still depends on goose CLI snippet %q", functionName, snippet)
			}
		}
	}
}

func TestGooseMigratesLegacySchemaWithRealPostgres(t *testing.T) {
	databaseURL := migrationTestDatabaseURL()
	if databaseURL == "" {
		t.Skip("skipping real goose migration test: set MIGRATION_TEST_DATABASE_URL or DATABASE_URL")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	adminPool := mustOpenPool(t, ctx, databaseURL)
	defer adminPool.Close()

	schemaName := fmt.Sprintf("migration_it_%d", time.Now().UnixNano())
	mustExec(t, ctx, adminPool, fmt.Sprintf("CREATE SCHEMA %s", schemaName))

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		_, _ = adminPool.Exec(cleanupCtx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName))
	})

	schemaDatabaseURL := mustSchemaDatabaseURL(t, databaseURL, schemaName)
	legacyPool := mustOpenPool(t, ctx, schemaDatabaseURL)
	defer legacyPool.Close()

	seedLegacySchema(t, ctx, legacyPool)
	mustRunGooseUp(t, ctx, schemaDatabaseURL)

	assertLatestGooseVersion(t, ctx, legacyPool, 8)
	assertColumnState(t, ctx, legacyPool, "jobs", "priority", true, "0")
	assertColumnState(t, ctx, legacyPool, "jobs", "payload", true, "'{}'::jsonb")
	assertColumnState(t, ctx, legacyPool, "job_logs", "seq", true, "nextval")
	assertColumnState(t, ctx, legacyPool, "email_services", "service_type", true, "''")
	assertColumnState(t, ctx, legacyPool, "email_services", "enabled", true, "true")
	assertColumnState(t, ctx, legacyPool, "email_services", "priority", true, "0")
	assertIndexExists(t, ctx, legacyPool, "job_runs", "job_runs_job_id_idx")
	assertIndexExists(t, ctx, legacyPool, "job_logs", "job_logs_job_id_seq_idx")
	assertIndexExists(t, ctx, legacyPool, "email_services", "email_services_enabled_priority_idx")
	assertIndexExists(t, ctx, legacyPool, "accounts", "accounts_activity_sort_idx")
	assertIndexExists(t, ctx, legacyPool, "cpa_services", "cpa_services_enabled_priority_idx")
	assertIndexExists(t, ctx, legacyPool, "sub2api_services", "sub2api_services_enabled_priority_idx")
	assertIndexExists(t, ctx, legacyPool, "tm_services", "tm_services_enabled_priority_idx")
	assertEmailServicesRepaired(t, ctx, legacyPool)
	assertAccountsBackfilled(t, ctx, legacyPool)
	assertUpgradedJobTablesAcceptWrites(t, ctx, legacyPool)
}

func TestJobsMigrationUsesIdempotentCreateStatements(t *testing.T) {
	sql := mustReadMigration(t, jobsMigration)

	requiredSnippets := []string{
		"CREATE SEQUENCE IF NOT EXISTS job_logs_seq;",
		"CREATE TABLE IF NOT EXISTS jobs (",
		"CREATE TABLE IF NOT EXISTS job_runs (",
		"CREATE TABLE IF NOT EXISTS job_logs (",
		"CREATE INDEX IF NOT EXISTS job_runs_job_id_idx ON job_runs (job_id);",
		"CREATE INDEX IF NOT EXISTS job_logs_job_id_seq_idx ON job_logs (job_id, seq);",
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(sql, snippet) {
			t.Fatalf("migration %s missing snippet %q", jobsMigration, snippet)
		}
	}
}

func TestJobsMigrationBackfillsLegacyTablesBeforeIndexes(t *testing.T) {
	sql := mustReadMigration(t, jobsMigration)

	requiredSnippets := []string{
		"ALTER TABLE jobs ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0;",
		"ALTER TABLE jobs ADD COLUMN IF NOT EXISTS payload JSONB NOT NULL DEFAULT '{}'::jsonb;",
		"ALTER TABLE jobs ADD COLUMN IF NOT EXISTS result JSONB;",
		"ALTER TABLE jobs ADD COLUMN IF NOT EXISTS error TEXT;",
		"ALTER TABLE jobs ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();",
		"ALTER TABLE jobs ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ;",
		"ALTER TABLE jobs ADD COLUMN IF NOT EXISTS finished_at TIMESTAMPTZ;",
		"ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS worker_id TEXT NOT NULL DEFAULT '';",
		"ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS attempt INT NOT NULL DEFAULT 1;",
		"ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS finished_at TIMESTAMPTZ;",
		"ALTER TABLE job_logs ADD COLUMN IF NOT EXISTS job_run_id UUID;",
		"ALTER TABLE job_logs ADD COLUMN IF NOT EXISTS seq BIGINT NOT NULL DEFAULT nextval('job_logs_seq');",
		"ALTER TABLE job_logs ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();",
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(sql, snippet) {
			t.Fatalf("migration %s missing snippet %q", jobsMigration, snippet)
		}
	}

	assertSnippetOrder(t, sql,
		"ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS finished_at TIMESTAMPTZ;",
		"CREATE INDEX IF NOT EXISTS job_runs_job_id_idx ON job_runs (job_id);",
	)
	assertSnippetOrder(t, sql,
		"ALTER TABLE job_logs ADD COLUMN IF NOT EXISTS seq BIGINT NOT NULL DEFAULT nextval('job_logs_seq');",
		"CREATE INDEX IF NOT EXISTS job_logs_job_id_seq_idx ON job_logs (job_id, seq);",
	)
}

func TestAccountsRegistrationMigrationDefinesMinimalAccountsRegistrationSchema(t *testing.T) {
	sql := mustReadMigration(t, accountsRegistrationMigration)
	requiredSnippets := []string{
		"CREATE TABLE IF NOT EXISTS accounts",
		"id SERIAL PRIMARY KEY",
		"email TEXT NOT NULL",
		"refresh_token TEXT",
		"extra_data JSONB NOT NULL DEFAULT '{}'::jsonb",
		"status TEXT NOT NULL DEFAULT 'active'",
		"source TEXT NOT NULL DEFAULT 'register'",
		"created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"CREATE UNIQUE INDEX IF NOT EXISTS accounts_email_uidx ON accounts (email)",
		"CREATE TABLE IF NOT EXISTS email_services",
		"id SERIAL PRIMARY KEY",
		"service_type TEXT NOT NULL",
		"name TEXT NOT NULL",
		"config JSONB NOT NULL DEFAULT '{}'::jsonb",
		"enabled BOOLEAN NOT NULL DEFAULT TRUE",
		"priority INT NOT NULL DEFAULT 0",
		"CREATE INDEX IF NOT EXISTS email_services_enabled_priority_idx ON email_services (enabled, priority, id)",
		"CREATE INDEX IF NOT EXISTS email_services_type_enabled_priority_idx ON email_services (service_type, enabled, priority, id)",
		"CREATE TABLE IF NOT EXISTS settings",
		"key TEXT PRIMARY KEY",
		"value TEXT",
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(sql, snippet) {
			t.Fatalf("migration %s missing snippet %q", accountsRegistrationMigration, snippet)
		}
	}
}

func TestAccountsRegistrationMigrationBackfillsLegacyEmailServiceColumnsBeforeIndexes(t *testing.T) {
	sql := mustReadMigration(t, accountsRegistrationMigration)

	requiredSnippets := []string{
		"ALTER TABLE email_services ADD COLUMN IF NOT EXISTS service_type TEXT NOT NULL DEFAULT '';",
		"ALTER TABLE email_services ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';",
		"ALTER TABLE email_services ADD COLUMN IF NOT EXISTS config JSONB NOT NULL DEFAULT '{}'::jsonb;",
		"ALTER TABLE email_services ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT TRUE;",
		"ALTER TABLE email_services ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0;",
		"ALTER TABLE email_services ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();",
		"ALTER TABLE email_services ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();",
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(sql, snippet) {
			t.Fatalf("migration %s missing snippet %q", accountsRegistrationMigration, snippet)
		}
	}

	assertSnippetOrder(t, sql,
		"ALTER TABLE email_services ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0;",
		"CREATE INDEX IF NOT EXISTS email_services_enabled_priority_idx ON email_services (enabled, priority, id);",
	)
	assertSnippetOrder(t, sql,
		"ALTER TABLE email_services ADD COLUMN IF NOT EXISTS service_type TEXT NOT NULL DEFAULT '';",
		"CREATE INDEX IF NOT EXISTS email_services_type_enabled_priority_idx ON email_services (service_type, enabled, priority, id);",
	)
}

func TestAccountsRegistrationMigrationRepairsLegacyEmailServiceConstraintsAndDefaults(t *testing.T) {
	sql := mustReadMigration(t, accountsRegistrationMigration)

	requiredSnippets := []string{
		"UPDATE email_services SET service_type = '' WHERE service_type IS NULL;",
		"ALTER TABLE email_services ALTER COLUMN service_type SET DEFAULT '';",
		"ALTER TABLE email_services ALTER COLUMN service_type SET NOT NULL;",
		"UPDATE email_services SET name = '' WHERE name IS NULL;",
		"ALTER TABLE email_services ALTER COLUMN name SET DEFAULT '';",
		"ALTER TABLE email_services ALTER COLUMN name SET NOT NULL;",
		"UPDATE email_services SET config = '{}'::jsonb WHERE config IS NULL;",
		"ALTER TABLE email_services ALTER COLUMN config SET DEFAULT '{}'::jsonb;",
		"ALTER TABLE email_services ALTER COLUMN config SET NOT NULL;",
		"UPDATE email_services SET enabled = TRUE WHERE enabled IS NULL;",
		"ALTER TABLE email_services ALTER COLUMN enabled SET DEFAULT TRUE;",
		"ALTER TABLE email_services ALTER COLUMN enabled SET NOT NULL;",
		"UPDATE email_services SET priority = 0 WHERE priority IS NULL;",
		"ALTER TABLE email_services ALTER COLUMN priority SET DEFAULT 0;",
		"ALTER TABLE email_services ALTER COLUMN priority SET NOT NULL;",
		"UPDATE email_services SET created_at = NOW() WHERE created_at IS NULL;",
		"ALTER TABLE email_services ALTER COLUMN created_at SET DEFAULT NOW();",
		"ALTER TABLE email_services ALTER COLUMN created_at SET NOT NULL;",
		"UPDATE email_services SET updated_at = NOW() WHERE updated_at IS NULL;",
		"ALTER TABLE email_services ALTER COLUMN updated_at SET DEFAULT NOW();",
		"ALTER TABLE email_services ALTER COLUMN updated_at SET NOT NULL;",
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(sql, snippet) {
			t.Fatalf("migration %s missing snippet %q", accountsRegistrationMigration, snippet)
		}
	}

	assertSnippetOrder(t, sql,
		"UPDATE email_services SET service_type = '' WHERE service_type IS NULL;",
		"ALTER TABLE email_services ALTER COLUMN service_type SET NOT NULL;",
	)
	assertSnippetOrder(t, sql,
		"ALTER TABLE email_services ALTER COLUMN updated_at SET NOT NULL;",
		"CREATE INDEX IF NOT EXISTS email_services_enabled_priority_idx ON email_services (enabled, priority, id);",
	)
}

func TestServiceConfigsMigrationDefinesUploaderServiceTables(t *testing.T) {
	sql := mustReadMigration(t, serviceConfigsMigration)

	requiredSnippets := []string{
		"CREATE TABLE IF NOT EXISTS cpa_services",
		"id SERIAL PRIMARY KEY",
		"name TEXT NOT NULL",
		"api_url TEXT NOT NULL",
		"api_token TEXT NOT NULL",
		"proxy_url TEXT",
		"enabled BOOLEAN NOT NULL DEFAULT TRUE",
		"priority INT NOT NULL DEFAULT 0",
		"created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"CREATE INDEX IF NOT EXISTS cpa_services_enabled_priority_idx ON cpa_services (enabled, priority, id)",
		"CREATE TABLE IF NOT EXISTS sub2api_services",
		"api_key TEXT NOT NULL",
		"target_type TEXT NOT NULL DEFAULT 'sub2api'",
		"CREATE INDEX IF NOT EXISTS sub2api_services_enabled_priority_idx ON sub2api_services (enabled, priority, id)",
		"CREATE TABLE IF NOT EXISTS tm_services",
		"api_key TEXT NOT NULL",
		"CREATE INDEX IF NOT EXISTS tm_services_enabled_priority_idx ON tm_services (enabled, priority, id)",
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(sql, snippet) {
			t.Fatalf("migration %s missing snippet %q", serviceConfigsMigration, snippet)
		}
	}
}

func TestServiceConfigsMigrationAddsMissingColumnsForLegacyTables(t *testing.T) {
	sql := mustReadMigration(t, serviceConfigsMigration)

	requiredSnippets := []string{
		"ALTER TABLE accounts ADD COLUMN IF NOT EXISTS password TEXT;",
		"ALTER TABLE accounts ADD COLUMN IF NOT EXISTS client_id TEXT;",
		"ALTER TABLE accounts ADD COLUMN IF NOT EXISTS session_token TEXT;",
		"ALTER TABLE accounts ADD COLUMN IF NOT EXISTS email_service TEXT NOT NULL DEFAULT '';",
		"ALTER TABLE accounts ADD COLUMN IF NOT EXISTS cpa_uploaded BOOLEAN NOT NULL DEFAULT FALSE;",
		"ALTER TABLE accounts ADD COLUMN IF NOT EXISTS cpa_uploaded_at TIMESTAMPTZ;",
		"ALTER TABLE accounts ADD COLUMN IF NOT EXISTS sub2api_uploaded BOOLEAN NOT NULL DEFAULT FALSE;",
		"ALTER TABLE accounts ADD COLUMN IF NOT EXISTS sub2api_uploaded_at TIMESTAMPTZ;",
		"ALTER TABLE accounts ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();",
		"UPDATE email_services SET service_type = 'moe_mail' WHERE service_type = 'custom_domain';",
		"UPDATE accounts SET email_service = 'moe_mail' WHERE email_service = 'custom_domain';",
		"CREATE INDEX IF NOT EXISTS accounts_activity_sort_idx ON accounts ((COALESCE(registered_at, created_at, updated_at)) DESC, id DESC);",
		"ALTER TABLE cpa_services ADD COLUMN IF NOT EXISTS api_url TEXT;",
		"ALTER TABLE cpa_services ADD COLUMN IF NOT EXISTS api_token TEXT;",
		"ALTER TABLE cpa_services ADD COLUMN IF NOT EXISTS proxy_url TEXT;",
		"ALTER TABLE cpa_services ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT TRUE;",
		"ALTER TABLE cpa_services ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0;",
		"ALTER TABLE sub2api_services ADD COLUMN IF NOT EXISTS api_url TEXT;",
		"ALTER TABLE sub2api_services ADD COLUMN IF NOT EXISTS api_key TEXT;",
		"ALTER TABLE sub2api_services ADD COLUMN IF NOT EXISTS target_type TEXT NOT NULL DEFAULT 'sub2api';",
		"ALTER TABLE sub2api_services ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT TRUE;",
		"ALTER TABLE sub2api_services ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0;",
		"ALTER TABLE tm_services ADD COLUMN IF NOT EXISTS api_url TEXT;",
		"ALTER TABLE tm_services ADD COLUMN IF NOT EXISTS api_key TEXT;",
		"ALTER TABLE tm_services ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT TRUE;",
		"ALTER TABLE tm_services ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0;",
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(sql, snippet) {
			t.Fatalf("migration %s missing snippet %q", serviceConfigsMigration, snippet)
		}
	}

	assertSnippetOrder(t, sql,
		"ALTER TABLE accounts ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();",
		"CREATE INDEX IF NOT EXISTS accounts_activity_sort_idx ON accounts ((COALESCE(registered_at, created_at, updated_at)) DESC, id DESC);",
	)
	assertSnippetOrder(t, sql,
		"ALTER TABLE cpa_services ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0;",
		"CREATE INDEX IF NOT EXISTS cpa_services_enabled_priority_idx ON cpa_services (enabled, priority, id);",
	)
	assertSnippetOrder(t, sql,
		"ALTER TABLE sub2api_services ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0;",
		"CREATE INDEX IF NOT EXISTS sub2api_services_enabled_priority_idx ON sub2api_services (enabled, priority, id);",
	)
	assertSnippetOrder(t, sql,
		"ALTER TABLE tm_services ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0;",
		"CREATE INDEX IF NOT EXISTS tm_services_enabled_priority_idx ON tm_services (enabled, priority, id);",
	)
}

func assertSnippetOrder(t *testing.T, sql string, first string, second string) {
	t.Helper()

	firstIndex := strings.Index(sql, first)
	if firstIndex == -1 {
		t.Fatalf("sql missing first snippet %q", first)
	}

	secondIndex := strings.Index(sql, second)
	if secondIndex == -1 {
		t.Fatalf("sql missing second snippet %q", second)
	}

	if firstIndex >= secondIndex {
		t.Fatalf("expected snippet %q to appear before %q", first, second)
	}
}

func assertLatestGooseVersion(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int64) {
	t.Helper()

	var got int64
	err := pool.QueryRow(ctx, "SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied").Scan(&got)
	if err != nil {
		t.Fatalf("query goose version: %v", err)
	}

	if got != want {
		t.Fatalf("latest goose version = %d, want %d", got, want)
	}
}

func assertColumnState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string, column string, wantNotNull bool, wantDefaultContains string) {
	t.Helper()

	var isNullable string
	var columnDefault string
	err := pool.QueryRow(ctx, `
SELECT is_nullable, COALESCE(column_default, '')
FROM information_schema.columns
WHERE table_schema = current_schema()
  AND table_name = $1
  AND column_name = $2
`, table, column).Scan(&isNullable, &columnDefault)
	if err != nil {
		t.Fatalf("query column state for %s.%s: %v", table, column, err)
	}

	if wantNotNull && isNullable != "NO" {
		t.Fatalf("column %s.%s should be NOT NULL, got is_nullable=%s", table, column, isNullable)
	}

	if wantDefaultContains != "" && !strings.Contains(columnDefault, wantDefaultContains) {
		t.Fatalf("column %s.%s default = %q, want substring %q", table, column, columnDefault, wantDefaultContains)
	}
}

func assertIndexExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string, index string) {
	t.Helper()

	var exists bool
	err := pool.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM pg_indexes
    WHERE schemaname = current_schema()
      AND tablename = $1
      AND indexname = $2
)
`, table, index).Scan(&exists)
	if err != nil {
		t.Fatalf("query index %s on %s: %v", index, table, err)
	}

	if !exists {
		t.Fatalf("expected index %s on table %s", index, table)
	}
}

func assertEmailServicesRepaired(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	var (
		serviceType string
		name        string
		configText  string
		enabled     bool
		priority    int
		hasCreated  bool
		hasUpdated  bool
	)

	err := pool.QueryRow(ctx, `
SELECT service_type, name, config::text, enabled, priority, created_at IS NOT NULL, updated_at IS NOT NULL
FROM email_services
LIMIT 1
`).Scan(&serviceType, &name, &configText, &enabled, &priority, &hasCreated, &hasUpdated)
	if err != nil {
		t.Fatalf("query repaired email_services row: %v", err)
	}

	if serviceType != "" || name != "" || configText != "{}" || !enabled || priority != 0 || !hasCreated || !hasUpdated {
		t.Fatalf("unexpected repaired email_services row: service_type=%q name=%q config=%q enabled=%v priority=%d created=%v updated=%v",
			serviceType, name, configText, enabled, priority, hasCreated, hasUpdated)
	}
}

func assertAccountsBackfilled(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	var emailService string
	err := pool.QueryRow(ctx, "SELECT email_service FROM accounts WHERE email = 'legacy@example.com'").Scan(&emailService)
	if err != nil {
		t.Fatalf("query migrated accounts row: %v", err)
	}

	if emailService != "moe_mail" {
		t.Fatalf("accounts.email_service = %q, want %q", emailService, "moe_mail")
	}
}

func assertUpgradedJobTablesAcceptWrites(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	const (
		jobID    = "11111111-1111-1111-1111-111111111111"
		jobRunID = "22222222-2222-2222-2222-222222222222"
	)

	mustExec(t, ctx, pool, `
INSERT INTO jobs (job_id, job_type, scope_type, scope_id, status)
VALUES ('`+jobID+`', 'refresh', 'workspace', 'alpha', 'queued')
`)
	mustExec(t, ctx, pool, `
INSERT INTO job_runs (job_run_id, job_id, worker_id, attempt, status)
VALUES ('`+jobRunID+`', '`+jobID+`', 'worker-a', 1, 'running')
`)
	mustExec(t, ctx, pool, `
INSERT INTO job_logs (job_id, level, message)
VALUES ('`+jobID+`', 'info', 'migration integration')
`)

	var (
		priority   int
		payload    string
		hasCreated bool
	)
	err := pool.QueryRow(ctx, `
SELECT priority, payload::text, created_at IS NOT NULL
FROM jobs
WHERE job_id = '`+jobID+`'
`).Scan(&priority, &payload, &hasCreated)
	if err != nil {
		t.Fatalf("query upgraded jobs row: %v", err)
	}

	if priority != 0 || payload != "{}" || !hasCreated {
		t.Fatalf("unexpected upgraded jobs row: priority=%d payload=%q created=%v", priority, payload, hasCreated)
	}

	var (
		seq           int64
		logHasCreated bool
	)
	err = pool.QueryRow(ctx, `
SELECT seq, created_at IS NOT NULL
FROM job_logs
WHERE job_id = '`+jobID+`'
ORDER BY id DESC
LIMIT 1
`).Scan(&seq, &logHasCreated)
	if err != nil {
		t.Fatalf("query upgraded job_logs row: %v", err)
	}

	if seq <= 0 || !logHasCreated {
		t.Fatalf("unexpected upgraded job_logs row: seq=%d created=%v", seq, logHasCreated)
	}
}

func mustReadMigration(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration %s: %v", path, err)
	}

	return string(content)
}

func firstNonEmptyLine(sql string) string {
	for _, line := range strings.Split(sql, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}

	return ""
}

func mustReadProjectFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read project file %s: %v", path, err)
	}

	return string(content)
}

func mustReadFunctionSource(t *testing.T, source string, functionName string) string {
	t.Helper()

	startMarker := "func " + functionName + "("
	start := strings.Index(source, startMarker)
	if start == -1 {
		t.Fatalf("function %s not found", functionName)
	}

	rest := source[start+len(startMarker):]
	next := strings.Index(rest, "\nfunc ")
	if next == -1 {
		return source[start:]
	}

	return source[start : start+len(startMarker)+next]
}

func migrationTestDatabaseURL() string {
	if value := os.Getenv("MIGRATION_TEST_DATABASE_URL"); value != "" {
		return value
	}

	return os.Getenv("DATABASE_URL")
}

func mustSchemaDatabaseURL(t *testing.T, databaseURL string, schemaName string) string {
	t.Helper()

	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse database url: %v", err)
	}

	query := parsed.Query()
	query.Set("search_path", schemaName)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func mustOpenPool(t *testing.T, ctx context.Context, databaseURL string) *pgxpool.Pool {
	t.Helper()

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse database url: %v", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("open postgres pool: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping postgres: %v", err)
	}

	return pool
}

func seedLegacySchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	statements := []string{
		`CREATE TABLE jobs (
			job_id UUID PRIMARY KEY,
			job_type TEXT NOT NULL,
			scope_type TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			status TEXT NOT NULL
		)`,
		`CREATE TABLE job_runs (
			job_run_id UUID PRIMARY KEY,
			job_id UUID NOT NULL REFERENCES jobs(job_id) ON DELETE CASCADE,
			status TEXT NOT NULL,
			started_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE job_logs (
			id BIGSERIAL PRIMARY KEY,
			job_id UUID NOT NULL REFERENCES jobs(job_id) ON DELETE CASCADE,
			level TEXT NOT NULL,
			message TEXT NOT NULL
		)`,
		`CREATE TABLE accounts (
			id SERIAL PRIMARY KEY,
			email TEXT NOT NULL,
			email_service TEXT
		)`,
		`INSERT INTO accounts (email, email_service) VALUES ('legacy@example.com', 'custom_domain')`,
		`CREATE TABLE email_services (
			id SERIAL PRIMARY KEY,
			service_type TEXT,
			name TEXT,
			config JSONB,
			enabled BOOLEAN,
			priority INT,
			created_at TIMESTAMPTZ,
			updated_at TIMESTAMPTZ
		)`,
		`INSERT INTO email_services (service_type, name, config, enabled, priority, created_at, updated_at)
		VALUES (NULL, NULL, NULL, NULL, NULL, NULL, NULL)`,
		`CREATE TABLE cpa_services (
			id SERIAL PRIMARY KEY
		)`,
		`INSERT INTO cpa_services DEFAULT VALUES`,
		`CREATE TABLE sub2api_services (
			id SERIAL PRIMARY KEY
		)`,
		`INSERT INTO sub2api_services DEFAULT VALUES`,
		`CREATE TABLE tm_services (
			id SERIAL PRIMARY KEY
		)`,
		`INSERT INTO tm_services DEFAULT VALUES`,
	}

	for _, statement := range statements {
		mustExec(t, ctx, pool, statement)
	}
}

func mustRunGooseUp(t *testing.T, ctx context.Context, databaseURL string) {
	t.Helper()

	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	db, err := goose.OpenDBWithDriver("postgres", databaseURL)
	if err != nil {
		t.Fatalf("open goose database handle: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping goose database handle: %v", err)
	}

	if err := goose.UpContext(ctx, db, workingDir); err != nil {
		t.Fatalf("run goose up: %v", err)
	}
}

func mustExec(t *testing.T, ctx context.Context, pool *pgxpool.Pool, statement string) {
	t.Helper()

	if _, err := pool.Exec(ctx, statement); err != nil {
		t.Fatalf("exec statement %q: %v", statement, err)
	}
}
