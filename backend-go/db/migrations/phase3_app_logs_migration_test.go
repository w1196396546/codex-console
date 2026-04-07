package migrations_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

const appLogsMigration = "0006_add_app_logs_management.sql"

func TestLogsMigrationStartsWithGooseUpAnnotation(t *testing.T) {
	sql := mustReadMigration(t, appLogsMigration)
	firstLine := firstNonEmptyLine(sql)
	if firstLine != "-- +goose Up" {
		t.Fatalf("migration %s must start with -- +goose Up, got %q", appLogsMigration, firstLine)
	}
}

func TestLogsMigrationDefinesAppLogsCompatibilitySchema(t *testing.T) {
	sql := mustReadMigration(t, appLogsMigration)

	requiredSnippets := []string{
		"CREATE TABLE IF NOT EXISTS app_logs",
		"level TEXT NOT NULL",
		"logger TEXT NOT NULL",
		"module TEXT",
		"pathname TEXT",
		"lineno INT",
		"message TEXT NOT NULL",
		"exception TEXT",
		"created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"CREATE INDEX IF NOT EXISTS app_logs_created_sort_idx ON app_logs (created_at DESC, id DESC);",
		"CREATE INDEX IF NOT EXISTS app_logs_level_created_sort_idx ON app_logs (level, created_at DESC, id DESC);",
		"CREATE INDEX IF NOT EXISTS app_logs_logger_created_sort_idx ON app_logs (logger, created_at DESC, id DESC);",
	}

	for _, snippet := range requiredSnippets {
		if !containsMigrationSnippet(sql, snippet) {
			t.Fatalf("migration %s missing snippet %q", appLogsMigration, snippet)
		}
	}
}

func TestLogsMigrationCreatesWritableAppLogsTableOnRealPostgres(t *testing.T) {
	databaseURL := migrationTestDatabaseURL()
	if databaseURL == "" {
		t.Skip("skipping real goose migration test: set MIGRATION_TEST_DATABASE_URL or DATABASE_URL")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	adminPool := mustOpenPool(t, ctx, databaseURL)
	defer adminPool.Close()

	schemaName := fmt.Sprintf("app_logs_migration_it_%d", time.Now().UnixNano())
	mustExec(t, ctx, adminPool, fmt.Sprintf("CREATE SCHEMA %s", schemaName))

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		_, _ = adminPool.Exec(cleanupCtx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName))
	})

	schemaDatabaseURL := mustSchemaDatabaseURL(t, databaseURL, schemaName)
	mustRunGooseUp(t, ctx, schemaDatabaseURL)

	pool := mustOpenPool(t, ctx, schemaDatabaseURL)
	defer pool.Close()

	assertLatestGooseVersion(t, ctx, pool, 8)
	assertIndexExists(t, ctx, pool, "app_logs", "app_logs_created_sort_idx")
	assertIndexExists(t, ctx, pool, "app_logs", "app_logs_level_created_sort_idx")
	assertIndexExists(t, ctx, pool, "app_logs", "app_logs_logger_created_sort_idx")

	var (
		id        int64
		level     string
		logger    string
		module    *string
		pathname  *string
		lineno    *int
		message   string
		exception *string
		created   time.Time
	)
	if err := pool.QueryRow(ctx, `
		INSERT INTO app_logs (level, logger, module, pathname, lineno, message, exception)
		VALUES ('ERROR', 'phase3.logs', 'worker', '/tmp/task.go', 18, 'boom', 'traceback')
		RETURNING id, level, logger, module, pathname, lineno, message, exception, created_at
	`).Scan(&id, &level, &logger, &module, &pathname, &lineno, &message, &exception, &created); err != nil {
		t.Fatalf("insert app_logs row: %v", err)
	}

	if id <= 0 || level != "ERROR" || logger != "phase3.logs" || message != "boom" {
		t.Fatalf("unexpected inserted app_logs row: id=%d level=%q logger=%q message=%q", id, level, logger, message)
	}
	if module == nil || *module != "worker" || pathname == nil || *pathname != "/tmp/task.go" {
		t.Fatalf("unexpected module/pathname values: module=%v pathname=%v", module, pathname)
	}
	if lineno == nil || *lineno != 18 || exception == nil || *exception != "traceback" {
		t.Fatalf("unexpected lineno/exception values: lineno=%v exception=%v", lineno, exception)
	}
	if created.IsZero() {
		t.Fatal("expected created_at to be populated")
	}
}

func containsMigrationSnippet(sql string, snippet string) bool {
	return len(sql) > 0 && len(snippet) > 0 && strings.Contains(sql, snippet)
}
