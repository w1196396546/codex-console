package postgres

import (
	"context"

	"github.com/dou-jiang/codex-console/backend-go/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func OpenPool(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	poolConfig.MaxConns = cfg.PostgresMaxConns
	poolConfig.MinConns = cfg.PostgresMinConns

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}
