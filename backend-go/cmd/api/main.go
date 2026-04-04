package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/config"
	internalhttp "github.com/dou-jiang/codex-console/backend-go/internal/http"
	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	postgresplatform "github.com/dou-jiang/codex-console/backend-go/internal/platform/postgres"
	redisplatform "github.com/dou-jiang/codex-console/backend-go/internal/platform/redis"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	redisv9 "github.com/redis/go-redis/v9"
)

type apiDependencies struct {
	Config   config.Config
	Postgres *pgxpool.Pool
	Redis    *redisv9.Client
	Queue    *jobs.AsynqQueue
}

func main() {
	deps, err := bootstrapAPI(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer closeAPI(deps)

	log.Printf("api listening on %s", deps.Config.HTTPAddr)

	jobService := jobs.NewService(jobs.NewRepository(deps.Postgres), deps.Queue)

	if err := http.ListenAndServe(deps.Config.HTTPAddr, internalhttp.NewRouter(jobService)); err != nil {
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

	queue := jobs.NewAsynqQueue(asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       cfg.RedisDB,
	})

	return apiDependencies{
		Config:   cfg,
		Postgres: pool,
		Redis:    redisClient,
		Queue:    queue,
	}, nil
}

func closeAPI(deps apiDependencies) {
	if deps.Queue != nil {
		if err := deps.Queue.Close(); err != nil {
			log.Printf("close asynq queue: %v", err)
		}
	}

	if deps.Redis != nil {
		if err := deps.Redis.Close(); err != nil {
			log.Printf("close redis client: %v", err)
		}
	}

	if deps.Postgres != nil {
		deps.Postgres.Close()
	}
}
