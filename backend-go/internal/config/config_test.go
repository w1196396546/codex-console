package config_test

import (
	"testing"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/config"
)

func TestLoadConfigRequiresDatabaseAndRedis(t *testing.T) {
	_, err := config.LoadFromEnv(map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing required env")
	}
}

func TestLoadConfigAppliesDefaultsAndTrimsWhitespace(t *testing.T) {
	cfg, err := config.LoadFromEnv(map[string]string{
		"DATABASE_URL":        "  postgres://codex:pass@localhost:5432/codex  ",
		"REDIS_ADDR":          "  127.0.0.1:6379  ",
		"REDIS_PASSWORD":      "  secret  ",
		"POSTGRES_MIN_CONNS":  " 3 ",
		"POSTGRES_MAX_CONNS":  " 15 ",
		"REDIS_DIAL_TIMEOUT":  " 4s ",
		"REDIS_READ_TIMEOUT":  " 5s ",
		"REDIS_WRITE_TIMEOUT": " 6s ",
	})
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.AppEnv != "development" {
		t.Fatalf("expected default AppEnv, got %q", cfg.AppEnv)
	}
	if cfg.HTTPAddr != ":18080" {
		t.Fatalf("expected default HTTPAddr, got %q", cfg.HTTPAddr)
	}
	if cfg.DatabaseURL != "postgres://codex:pass@localhost:5432/codex" {
		t.Fatalf("expected trimmed DatabaseURL, got %q", cfg.DatabaseURL)
	}
	if cfg.RedisAddr != "127.0.0.1:6379" {
		t.Fatalf("expected trimmed RedisAddr, got %q", cfg.RedisAddr)
	}
	if cfg.RedisPass != "secret" {
		t.Fatalf("expected trimmed RedisPass, got %q", cfg.RedisPass)
	}
	if cfg.PostgresMinConns != 3 {
		t.Fatalf("expected PostgresMinConns=3, got %d", cfg.PostgresMinConns)
	}
	if cfg.PostgresMaxConns != 15 {
		t.Fatalf("expected PostgresMaxConns=15, got %d", cfg.PostgresMaxConns)
	}
	if cfg.RedisDialTimeout != 4*time.Second {
		t.Fatalf("expected RedisDialTimeout=4s, got %s", cfg.RedisDialTimeout)
	}
	if cfg.RedisReadTimeout != 5*time.Second {
		t.Fatalf("expected RedisReadTimeout=5s, got %s", cfg.RedisReadTimeout)
	}
	if cfg.RedisWriteTimeout != 6*time.Second {
		t.Fatalf("expected RedisWriteTimeout=6s, got %s", cfg.RedisWriteTimeout)
	}
}

func TestLoadConfigUsesConnectionDefaults(t *testing.T) {
	cfg, err := config.LoadFromEnv(map[string]string{
		"DATABASE_URL": "postgres://codex:pass@localhost:5432/codex",
		"REDIS_ADDR":   "127.0.0.1:6379",
	})
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.PostgresMinConns != 2 {
		t.Fatalf("expected default PostgresMinConns=2, got %d", cfg.PostgresMinConns)
	}
	if cfg.PostgresMaxConns != 20 {
		t.Fatalf("expected default PostgresMaxConns=20, got %d", cfg.PostgresMaxConns)
	}
	if cfg.RedisDialTimeout != 5*time.Second {
		t.Fatalf("expected default RedisDialTimeout=5s, got %s", cfg.RedisDialTimeout)
	}
	if cfg.RedisReadTimeout != 3*time.Second {
		t.Fatalf("expected default RedisReadTimeout=3s, got %s", cfg.RedisReadTimeout)
	}
	if cfg.RedisWriteTimeout != 3*time.Second {
		t.Fatalf("expected default RedisWriteTimeout=3s, got %s", cfg.RedisWriteTimeout)
	}
}

func TestLoadConfigRejectsInvalidRedisDB(t *testing.T) {
	_, err := config.LoadFromEnv(map[string]string{
		"DATABASE_URL": "postgres://codex:pass@localhost:5432/codex",
		"REDIS_ADDR":   "127.0.0.1:6379",
		"REDIS_DB":     "abc",
	})
	if err == nil {
		t.Fatal("expected error for invalid REDIS_DB")
	}
}

func TestLoadConfigRejectsNegativeRedisDB(t *testing.T) {
	_, err := config.LoadFromEnv(map[string]string{
		"DATABASE_URL": "postgres://codex:pass@localhost:5432/codex",
		"REDIS_ADDR":   "127.0.0.1:6379",
		"REDIS_DB":     "-1",
	})
	if err == nil {
		t.Fatal("expected error for negative REDIS_DB")
	}
}
