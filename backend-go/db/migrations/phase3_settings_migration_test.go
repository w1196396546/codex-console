package migrations_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const phase3SettingsMigration = "0004_extend_settings_metadata_and_proxies.sql"

func TestSettingsMigrationStartsWithGooseUpAnnotation(t *testing.T) {
	sql := mustReadMigration(t, phase3SettingsMigration)
	firstLine := firstNonEmptyLine(sql)
	if firstLine != "-- +goose Up" {
		t.Fatalf("migration %s must start with -- +goose Up, got %q", phase3SettingsMigration, firstLine)
	}
}

func TestSettingsMigrationDefinesMetadataAndProxyStorage(t *testing.T) {
	sql := mustReadMigration(t, phase3SettingsMigration)

	requiredSnippets := []string{
		"ALTER TABLE settings ADD COLUMN IF NOT EXISTS description TEXT;",
		"ALTER TABLE settings ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT 'general';",
		"ALTER TABLE settings ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();",
		"CREATE TABLE IF NOT EXISTS proxies (",
		"id SERIAL PRIMARY KEY",
		"name TEXT NOT NULL",
		"type TEXT NOT NULL DEFAULT 'http'",
		"host TEXT NOT NULL",
		"port INT NOT NULL",
		"username TEXT",
		"password TEXT",
		"enabled BOOLEAN NOT NULL DEFAULT TRUE",
		"is_default BOOLEAN NOT NULL DEFAULT FALSE",
		"priority INT NOT NULL DEFAULT 0",
		"last_used TIMESTAMPTZ",
		"created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"proxy_url TEXT GENERATED ALWAYS AS",
		"CREATE INDEX IF NOT EXISTS proxies_enabled_default_last_used_idx ON proxies (enabled, is_default, last_used, id);",
	}

	for _, snippet := range requiredSnippets {
		if !containsLine(sql, snippet) {
			t.Fatalf("migration %s missing snippet %q", phase3SettingsMigration, snippet)
		}
	}
}

func TestSettingsMigrationBackfillsLegacySettingsMetadataBeforeConstraints(t *testing.T) {
	sql := mustReadMigration(t, phase3SettingsMigration)

	requiredSnippets := []string{
		"UPDATE settings SET description = '' WHERE description IS NULL;",
		"ALTER TABLE settings ALTER COLUMN description SET DEFAULT '';",
		"UPDATE settings SET category = 'general' WHERE category IS NULL OR BTRIM(category) = '';",
		"ALTER TABLE settings ALTER COLUMN category SET DEFAULT 'general';",
		"ALTER TABLE settings ALTER COLUMN category SET NOT NULL;",
		"UPDATE settings SET updated_at = NOW() WHERE updated_at IS NULL;",
		"ALTER TABLE settings ALTER COLUMN updated_at SET DEFAULT NOW();",
		"ALTER TABLE settings ALTER COLUMN updated_at SET NOT NULL;",
	}

	for _, snippet := range requiredSnippets {
		if !containsLine(sql, snippet) {
			t.Fatalf("migration %s missing snippet %q", phase3SettingsMigration, snippet)
		}
	}

	assertSnippetOrder(t, sql,
		"UPDATE settings SET category = 'general' WHERE category IS NULL OR BTRIM(category) = '';",
		"ALTER TABLE settings ALTER COLUMN category SET NOT NULL;",
	)
}

func TestSettingsMigrationAppliesLegacySchemaWithRealPostgres(t *testing.T) {
	databaseURL := migrationTestDatabaseURL()
	if databaseURL == "" {
		t.Skip("skipping real goose migration test: set MIGRATION_TEST_DATABASE_URL or DATABASE_URL")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	adminPool := mustOpenPool(t, ctx, databaseURL)
	defer adminPool.Close()

	schemaName := fmt.Sprintf("migration_it_phase3_settings_%d", time.Now().UnixNano())
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
	mustExec(t, ctx, legacyPool, `
ALTER TABLE settings ADD COLUMN IF NOT EXISTS description TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS category TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ;
INSERT INTO settings (key, value, description, category, updated_at)
VALUES ('proxy.enabled', 'true', NULL, NULL, NULL)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, description = NULL, category = NULL, updated_at = NULL;
`)
	mustRunGooseUp(t, ctx, schemaDatabaseURL)

	assertColumnState(t, ctx, legacyPool, "settings", "category", true, "'general'::text")
	assertColumnState(t, ctx, legacyPool, "settings", "updated_at", true, "now()")
	assertIndexExists(t, ctx, legacyPool, "proxies", "proxies_enabled_default_last_used_idx")
	assertSettingsMetadataRepaired(t, ctx, legacyPool)
	assertLegacyProxyPoolAcceptsWrites(t, ctx, legacyPool)
}

func assertSettingsMetadataRepaired(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	var (
		description string
		category    string
	)
	if err := pool.QueryRow(ctx, `
SELECT COALESCE(description, ''), category
FROM settings
WHERE key = 'proxy.enabled'
`).Scan(&description, &category); err != nil {
		t.Fatalf("query repaired settings row: %v", err)
	}

	if category != "general" {
		t.Fatalf("expected repaired settings category=general, got %q", category)
	}
}

func assertLegacyProxyPoolAcceptsWrites(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	var proxyURL string
	if err := pool.QueryRow(ctx, `
INSERT INTO proxies (name, type, host, port, username, password, enabled, is_default)
VALUES ('default-proxy', 'socks5', '127.0.0.1', 1080, 'user', 'pass', TRUE, TRUE)
RETURNING proxy_url
`).Scan(&proxyURL); err != nil {
		t.Fatalf("insert proxy row: %v", err)
	}

	if proxyURL != "socks5://user:pass@127.0.0.1:1080" {
		t.Fatalf("expected generated proxy_url, got %q", proxyURL)
	}
}

func containsLine(sql string, snippet string) bool {
	normalizedSQL := normalizeWhitespace(sql)
	normalizedSnippet := normalizeWhitespace(snippet)
	return normalizedSQL != "" && normalizedSnippet != "" && strings.Contains(normalizedSQL, normalizedSnippet)
}

func normalizeWhitespace(value string) string {
	out := make([]rune, 0, len(value))
	lastSpace := false
	for _, r := range value {
		space := r == ' ' || r == '\n' || r == '\t' || r == '\r'
		if space {
			if lastSpace {
				continue
			}
			out = append(out, ' ')
			lastSpace = true
			continue
		}
		out = append(out, r)
		lastSpace = false
	}
	return string(out)
}
