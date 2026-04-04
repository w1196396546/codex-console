package main

import (
	"context"
	"log"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/config"
	postgresplatform "github.com/dou-jiang/codex-console/backend-go/internal/platform/postgres"
	redisplatform "github.com/dou-jiang/codex-console/backend-go/internal/platform/redis"
	"github.com/jackc/pgx/v5/pgxpool"
	redisv9 "github.com/redis/go-redis/v9"
)

type workerDependencies struct {
	Config   config.Config
	Postgres *pgxpool.Pool
	Redis    *redisv9.Client
}

func main() {
	deps, err := bootstrapWorker(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer closeWorker(deps)

	log.Print("worker bootstrap started")
}

func bootstrapWorker(parent context.Context) (workerDependencies, error) {
	cfg, err := config.Load()
	if err != nil {
		return workerDependencies{}, err
	}

	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	pool, err := postgresplatform.OpenPool(ctx, cfg)
	if err != nil {
		return workerDependencies{}, err
	}

	redisClient, err := redisplatform.NewClient(ctx, cfg)
	if err != nil {
		pool.Close()
		return workerDependencies{}, err
	}

	return workerDependencies{
		Config:   cfg,
		Postgres: pool,
		Redis:    redisClient,
	}, nil
}

func closeWorker(deps workerDependencies) {
	if deps.Redis != nil {
		if err := deps.Redis.Close(); err != nil {
			log.Printf("close redis client: %v", err)
		}
	}

	if deps.Postgres != nil {
		deps.Postgres.Close()
	}
}
