package redis

import (
	"context"

	redisv9 "github.com/redis/go-redis/v9"

	"github.com/dou-jiang/codex-console/backend-go/internal/config"
)

func NewClient(ctx context.Context, cfg config.Config) (*redisv9.Client, error) {
	client := redisv9.NewClient(&redisv9.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPass,
		DB:           cfg.RedisDB,
		DialTimeout:  cfg.RedisDialTimeout,
		ReadTimeout:  cfg.RedisReadTimeout,
		WriteTimeout: cfg.RedisWriteTimeout,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}

	return client, nil
}
