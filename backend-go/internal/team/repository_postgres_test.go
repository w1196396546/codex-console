package team

import (
	"os"
	"strings"
	"testing"
)

func TestRepositoryMigrationIncludesTeamCompatibilitySchema(t *testing.T) {
	sqlBytes, err := os.ReadFile("../../db/migrations/0008_init_team_domains.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(sqlBytes)

	requiredSnippets := []string{
		"-- +goose Up",
		"CREATE TABLE IF NOT EXISTS teams",
		"CREATE TABLE IF NOT EXISTS team_memberships",
		"CREATE TABLE IF NOT EXISTS team_tasks",
		"CREATE TABLE IF NOT EXISTS team_task_items",
		"task_uuid TEXT NOT NULL",
		"active_scope_key TEXT UNIQUE",
		"sync_status TEXT NOT NULL DEFAULT 'pending'",
		"sync_error TEXT",
		"source TEXT NOT NULL DEFAULT 'sync'",
		"removed_at TIMESTAMPTZ",
		"CREATE UNIQUE INDEX IF NOT EXISTS team_tasks_task_uuid_uidx ON team_tasks (task_uuid)",
		"CREATE UNIQUE INDEX IF NOT EXISTS team_tasks_active_scope_key_uidx ON team_tasks (active_scope_key) WHERE active_scope_key IS NOT NULL",
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(sql, snippet) {
			t.Fatalf("migration missing snippet %q", snippet)
		}
	}
}

func TestRepositoryPostgresImplementsRepository(t *testing.T) {
	t.Helper()

	var _ Repository = (*PostgresRepository)(nil)
	var _ TaskRepository = (*PostgresRepository)(nil)
}
