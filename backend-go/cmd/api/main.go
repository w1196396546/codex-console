package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/config"
	internalhttp "github.com/dou-jiang/codex-console/backend-go/internal/http"
	postgresplatform "github.com/dou-jiang/codex-console/backend-go/internal/platform/postgres"
	redisplatform "github.com/dou-jiang/codex-console/backend-go/internal/platform/redis"
	"github.com/jackc/pgx/v5/pgxpool"
	redisv9 "github.com/redis/go-redis/v9"
)

type apiDependencies struct {
	Config   config.Config
	Postgres *pgxpool.Pool
	Redis    *redisv9.Client
}

func main() {
	deps, err := bootstrapAPI(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer closeAPI(deps)

	log.Printf("api listening on %s", deps.Config.HTTPAddr)

	if err := http.ListenAndServe(deps.Config.HTTPAddr, internalhttp.NewRouter(deps)); err != nil {
		log.Fatal(err)
	}
}

func bootstrapAPI(parent context.Context) (apiDependencies, error) {
	cfg, err := config.Load()
	if err != nil {
		return apiDependencies{}, err
	}

	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	pool, err := postgresplatform.OpenPool(ctx, cfg)
	if err != nil {
		return apiDependencies{}, err
	}

	redisClient, err := redisplatform.NewClient(ctx, cfg)
	if err != nil {
		pool.Close()
		return apiDependencies{}, err
	}

	return apiDependencies{
		Config:   cfg,
		Postgres: pool,
		Redis:    redisClient,
	}, nil
}

func closeAPI(deps apiDependencies) {
	if deps.Redis != nil {
		if err := deps.Redis.Close(); err != nil {
			log.Printf("close redis client: %v", err)
		}
	}

	if deps.Postgres != nil {
		deps.Postgres.Close()
	}
}
