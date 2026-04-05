package migrations_test

import (
	"strings"
	"testing"
)

func TestEmailServicesManagementMigrationAddsLastUsedMetadata(t *testing.T) {
	sql := mustReadMigration(t, "0005_extend_email_services_management.sql")

	requiredSnippets := []string{
		"-- +goose Up",
		"ALTER TABLE email_services ADD COLUMN IF NOT EXISTS last_used TIMESTAMPTZ;",
		"CREATE INDEX IF NOT EXISTS email_services_management_sort_idx ON email_services (service_type, priority, id, last_used DESC);",
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(sql, snippet) {
			t.Fatalf("migration 0005_extend_email_services_management.sql missing snippet %q", snippet)
		}
	}
}
