package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv            string
	HTTPAddr          string
	DatabaseURL       string
	PostgresMinConns  int32
	PostgresMaxConns  int32
	RedisAddr         string
	RedisDB           int
	RedisPass         string
	RedisDialTimeout  time.Duration
	RedisReadTimeout  time.Duration
	RedisWriteTimeout time.Duration
}

func Load() (Config, error) {
	return LoadFromEnv(currentEnv())
}

func LoadFromEnv(env map[string]string) (Config, error) {
	cfg := Config{
		AppEnv:      get(env, "APP_ENV", "development"),
		HTTPAddr:    get(env, "HTTP_ADDR", ":18080"),
		DatabaseURL: strings.TrimSpace(env["DATABASE_URL"]),
		RedisAddr:   strings.TrimSpace(env["REDIS_ADDR"]),
		RedisPass:   strings.TrimSpace(env["REDIS_PASSWORD"]),
	}

	if cfg.DatabaseURL == "" || cfg.RedisAddr == "" {
		return Config{}, errors.New("DATABASE_URL and REDIS_ADDR are required")
	}

	redisDB, err := parseRedisDB(env["REDIS_DB"])
	if err != nil {
		return Config{}, err
	}
	cfg.RedisDB = redisDB

	postgresMinConns, err := parseInt32(env["POSTGRES_MIN_CONNS"], 2, "POSTGRES_MIN_CONNS", 1)
	if err != nil {
		return Config{}, err
	}
	cfg.PostgresMinConns = postgresMinConns

	postgresMaxConns, err := parseInt32(env["POSTGRES_MAX_CONNS"], 20, "POSTGRES_MAX_CONNS", 1)
	if err != nil {
		return Config{}, err
	}
	if postgresMaxConns < postgresMinConns {
		return Config{}, errors.New("POSTGRES_MAX_CONNS must be greater than or equal to POSTGRES_MIN_CONNS")
	}
	cfg.PostgresMaxConns = postgresMaxConns

	redisDialTimeout, err := parseDuration(env["REDIS_DIAL_TIMEOUT"], 5*time.Second, "REDIS_DIAL_TIMEOUT")
	if err != nil {
		return Config{}, err
	}
	cfg.RedisDialTimeout = redisDialTimeout

	redisReadTimeout, err := parseDuration(env["REDIS_READ_TIMEOUT"], 3*time.Second, "REDIS_READ_TIMEOUT")
	if err != nil {
		return Config{}, err
	}
	cfg.RedisReadTimeout = redisReadTimeout

	redisWriteTimeout, err := parseDuration(env["REDIS_WRITE_TIMEOUT"], 3*time.Second, "REDIS_WRITE_TIMEOUT")
	if err != nil {
		return Config{}, err
	}
	cfg.RedisWriteTimeout = redisWriteTimeout

	return cfg, nil
}

func currentEnv() map[string]string {
	keys := []string{
		"APP_ENV",
		"HTTP_ADDR",
		"DATABASE_URL",
		"POSTGRES_MIN_CONNS",
		"POSTGRES_MAX_CONNS",
		"REDIS_ADDR",
		"REDIS_PASSWORD",
		"REDIS_DB",
		"REDIS_DIAL_TIMEOUT",
		"REDIS_READ_TIMEOUT",
		"REDIS_WRITE_TIMEOUT",
	}

	env := make(map[string]string, len(keys))
	for _, key := range keys {
		env[key] = os.Getenv(key)
	}

	return env
}

func get(env map[string]string, key string, fallback string) string {
	value := strings.TrimSpace(env[key])
	if value == "" {
		return fallback
	}
	return value
}

func parseRedisDB(raw string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, nil
	}

	redisDB, err := strconv.Atoi(value)
	if err != nil {
		return 0, errors.New("REDIS_DB must be a valid integer")
	}
	if redisDB < 0 {
		return 0, errors.New("REDIS_DB must be greater than or equal to 0")
	}

	return redisDB, nil
}

func parseInt32(raw string, fallback int32, field string, minValue int32) (int32, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid integer", field)
	}
	if parsed < int(minValue) {
		return 0, fmt.Errorf("%s must be greater than or equal to %d", field, minValue)
	}

	return int32(parsed), nil
}

func parseDuration(raw string, fallback time.Duration, field string) (time.Duration, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration", field)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be greater than 0", field)
	}

	return parsed, nil
}
