package main

import (
	"context"
	"log"
	"time"

	"github.com/dou-jiang/codex-console/backend-go/internal/config"
	"github.com/dou-jiang/codex-console/backend-go/internal/jobs"
	postgresplatform "github.com/dou-jiang/codex-console/backend-go/internal/platform/postgres"
	redisplatform "github.com/dou-jiang/codex-console/backend-go/internal/platform/redis"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	redisv9 "github.com/redis/go-redis/v9"
)

type workerDependencies struct {
	Config   config.Config
	Postgres *pgxpool.Pool
	Redis    *redisv9.Client
	Queue    *jobs.AsynqQueue
	Server   *asynq.Server
	Service  *jobs.Service
}

func main() {
	deps, err := bootstrapWorker(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer closeWorker(deps)

	worker := jobs.NewWorkerWithID(deps.Service, "worker-main")
	mux := asynq.NewServeMux()
	mux.HandleFunc(jobs.TypeGenericJob, worker.HandleTask)

	log.Print("worker bootstrap started")
	if err := deps.Server.Run(mux); err != nil {
		log.Fatal(err)
	}
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

	queue := jobs.NewAsynqQueue(asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       cfg.RedisDB,
	})
	service := jobs.NewService(jobs.NewRepository(pool), queue)
	server := asynq.NewServer(asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       cfg.RedisDB,
	}, asynq.Config{
		Concurrency: 5,
	})

	return workerDependencies{
		Config:   cfg,
		Postgres: pool,
		Redis:    redisClient,
		Queue:    queue,
		Server:   server,
		Service:  service,
	}, nil
}

func closeWorker(deps workerDependencies) {
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
